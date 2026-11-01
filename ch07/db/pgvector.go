package db

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"babyagent/ch07/rag"
)

// PGVectorStore 使用 pgvector 存储和检索向量
type PGVectorStore struct {
	db        *gorm.DB
	dimension int
}

// DocumentChunk 文档块模型
type DocumentChunk struct {
	ID         uint   `gorm:"primaryKey"`
	Content    string `gorm:"type:text;not null"`
	DocumentID string `gorm:"type:text;not null;index"`
	StartPos   int    `gorm:"not null"`
	EndPos     int    `gorm:"not null"`
	// The actual dimension is set by initTable's CREATE TABLE. AutoMigrate is
	// not used because the dimension is part of the vector type.
	Embedding string    `gorm:"type:vector"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// TableName 指定表名
func (DocumentChunk) TableName() string {
	return "document_chunks"
}

// ToChunk 转换为 rag.Chunk
func (d *DocumentChunk) ToChunk() rag.Chunk {
	return rag.Chunk{
		Content: d.Content,
		Meta: rag.Meta{
			DocumentID: d.DocumentID,
			StartPos:   d.StartPos,
			EndPos:     d.EndPos,
		},
	}
}

// Config pgvector 配置
type Config struct {
	// DSN takes precedence over the individual fields when non-empty, e.g.
	// "host=localhost port=5432 user=postgres password=... dbname=babyagent sslmode=disable".
	DSN       string
	Host      string
	Port      int
	User      string
	Password  string
	Database  string
	Dimension int
}

// NewPGVectorStore 创建一个新的 PGVectorStore
func NewPGVectorStore(config Config) (*PGVectorStore, error) {
	if config.Dimension == 0 {
		config.Dimension = rag.DefaultDimensions
	}

	dsn := config.DSN
	if dsn == "" {
		dsn = fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			config.Host, config.Port, config.User, config.Password, config.Database,
		)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	store := &PGVectorStore{
		db:        db,
		dimension: config.Dimension,
	}

	// 初始化表
	if err := store.initTable(); err != nil {
		return nil, fmt.Errorf("failed to initialize table: %w", err)
	}

	return store, nil
}

// initTable 创建必要的表和索引。
//
// 这里不使用 AutoMigrate：vector 的维度是列类型的一部分，必须在建表时
// 按配置生成 vector(dim)。如果表已经存在但维度不同，直接报错，避免插入
// 时才出现难以理解的类型错误。
func (s *PGVectorStore) initTable() error {
	if err := s.db.Exec("CREATE EXTENSION IF NOT EXISTS vector").Error; err != nil {
		return fmt.Errorf("failed to create vector extension: %w", err)
	}

	createTableSQL := fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS document_chunks (
			id BIGSERIAL PRIMARY KEY,
			content TEXT NOT NULL,
			document_id TEXT NOT NULL,
			start_pos INTEGER NOT NULL,
			end_pos INTEGER NOT NULL,
			embedding vector(%d) NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`, s.dimension)
	if err := s.db.Exec(createTableSQL).Error; err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}

	var columnType string
	if err := s.db.Raw(`
		SELECT format_type(atttypid, atttypmod)
		FROM pg_attribute
		WHERE attrelid = 'document_chunks'::regclass AND attname = 'embedding'
	`).Scan(&columnType).Error; err != nil {
		return fmt.Errorf("failed to inspect embedding column: %w", err)
	}
	expectedType := fmt.Sprintf("vector(%d)", s.dimension)
	if columnType != expectedType {
		return fmt.Errorf(
			"document_chunks.embedding is %s, but configured dimension is %d; drop the table or use the matching dimension",
			columnType, s.dimension,
		)
	}

	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_document_chunks_document_id ON document_chunks (document_id)`,
		`CREATE INDEX IF NOT EXISTS idx_document_chunks_embedding
		 ON document_chunks USING ivfflat (embedding vector_cosine_ops) WITH (lists = 100)`,
	}
	for _, indexSQL := range indexes {
		if err := s.db.Exec(indexSQL).Error; err != nil {
			return fmt.Errorf("failed to create index: %w", err)
		}
	}

	return nil
}

// Insert 插入一个向量点
func (s *PGVectorStore) Insert(ctx context.Context, vp rag.VectorPoint) error {
	if len(vp.Vector) != s.dimension {
		return fmt.Errorf("vector dimension mismatch: expected %d, got %d", s.dimension, len(vp.Vector))
	}

	doc := &DocumentChunk{
		Content:    vp.Chunk.Content,
		DocumentID: vp.Chunk.Meta.DocumentID,
		StartPos:   vp.Chunk.Meta.StartPos,
		EndPos:     vp.Chunk.Meta.EndPos,
		Embedding:  vectorToPGVector(vp.Vector),
	}

	return s.db.WithContext(ctx).Create(doc).Error
}

// InsertBatch 批量插入向量点
func (s *PGVectorStore) InsertBatch(ctx context.Context, vps []rag.VectorPoint) error {
	docs := make([]*DocumentChunk, len(vps))
	for i, vp := range vps {
		if len(vp.Vector) != s.dimension {
			return fmt.Errorf("vector dimension mismatch at index %d: expected %d, got %d", i, s.dimension, len(vp.Vector))
		}

		docs[i] = &DocumentChunk{
			Content:    vp.Chunk.Content,
			DocumentID: vp.Chunk.Meta.DocumentID,
			StartPos:   vp.Chunk.Meta.StartPos,
			EndPos:     vp.Chunk.Meta.EndPos,
			Embedding:  vectorToPGVector(vp.Vector),
		}
	}

	return s.db.WithContext(ctx).CreateInBatches(docs, 100).Error
}

// ReplaceDocument 在一个事务里替换文档的所有块。
// 调用方应先把所有 chunk 嵌入成功，再调用本方法；这样嵌入失败不会把
// 文档已有的索引删空。
func (s *PGVectorStore) ReplaceDocument(ctx context.Context, documentID string, vps []rag.VectorPoint) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("document_id = ?", documentID).Delete(&DocumentChunk{}).Error; err != nil {
			return err
		}
		if len(vps) == 0 {
			return nil
		}

		docs := make([]*DocumentChunk, len(vps))
		for i, vp := range vps {
			if len(vp.Vector) != s.dimension {
				return fmt.Errorf("vector dimension mismatch at index %d: expected %d, got %d", i, s.dimension, len(vp.Vector))
			}
			docs[i] = &DocumentChunk{
				Content:    vp.Chunk.Content,
				DocumentID: vp.Chunk.Meta.DocumentID,
				StartPos:   vp.Chunk.Meta.StartPos,
				EndPos:     vp.Chunk.Meta.EndPos,
				Embedding:  vectorToPGVector(vp.Vector),
			}
		}

		return tx.CreateInBatches(docs, 100).Error
	})
}

// Search 执行向量相似度搜索
func (s *PGVectorStore) Search(ctx context.Context, queryVector rag.Vector, limit int) ([]rag.VectorPointResult, error) {
	if len(queryVector) != s.dimension {
		return nil, fmt.Errorf("query vector dimension mismatch: expected %d, got %d", s.dimension, len(queryVector))
	}

	vectorStr := vectorToPGVector(queryVector)

	var results []struct {
		ID         int
		Content    string
		DocumentID string
		StartPos   int
		EndPos     int
		Score      float32
	}

	query := `
		SELECT id, content, document_id, start_pos, end_pos,
		       1 - (embedding <=> ?) as score
		FROM document_chunks
		ORDER BY embedding <=> ?
		LIMIT ?
	`

	err := s.db.WithContext(ctx).Raw(query, vectorStr, vectorStr, limit).Scan(&results).Error
	if err != nil {
		return nil, err
	}

	vectorPointResults := make([]rag.VectorPointResult, len(results))
	for i, r := range results {
		vectorPointResults[i] = rag.VectorPointResult{
			VectorPoint: rag.VectorPoint{
				Vector: nil,
				Chunk: rag.Chunk{
					Content: r.Content,
					Meta: rag.Meta{
						DocumentID: r.DocumentID,
						StartPos:   r.StartPos,
						EndPos:     r.EndPos,
					},
				},
			},
			Score: r.Score,
		}
	}

	return vectorPointResults, nil
}

// DeleteByDocument 删除指定文档的所有向量
func (s *PGVectorStore) DeleteByDocument(ctx context.Context, documentID string) error {
	return s.db.WithContext(ctx).Where("document_id = ?", documentID).Delete(&DocumentChunk{}).Error
}

// Clear 清空表
func (s *PGVectorStore) Clear(ctx context.Context) error {
	return s.db.WithContext(ctx).Exec("TRUNCATE TABLE document_chunks").Error
}

// Count 返回文档块总数
func (s *PGVectorStore) Count(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&DocumentChunk{}).Count(&count).Error
	return count, err
}

// GetByDocumentID 获取指定文档的所有文档块
func (s *PGVectorStore) GetByDocumentID(ctx context.Context, documentID string) ([]rag.Chunk, error) {
	var docs []DocumentChunk
	err := s.db.WithContext(ctx).Where("document_id = ?", documentID).Find(&docs).Error
	if err != nil {
		return nil, err
	}

	chunks := make([]rag.Chunk, len(docs))
	for i, d := range docs {
		chunks[i] = d.ToChunk()
	}

	return chunks, nil
}

// Close 关闭数据库连接
func (s *PGVectorStore) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// vectorToPGVector 将 rag.Vector 转换为 pgvector 格式字符串
func vectorToPGVector(v rag.Vector) string {
	if len(v) == 0 {
		return "[]"
	}

	strValues := make([]string, len(v))
	for i, val := range v {
		// %f only keeps 6 decimals, which is too lossy for embeddings.
		strValues[i] = strconv.FormatFloat(float64(val), 'f', -1, 32)
	}

	return "[" + strings.Join(strValues, ",") + "]"
}

// GetDocumentIndexedTime 获取文档的索引时间（返回最早的索引时间）
func (s *PGVectorStore) GetDocumentIndexedTime(ctx context.Context, documentID string) (time.Time, error) {
	var doc DocumentChunk
	err := s.db.WithContext(ctx).Where("document_id = ?", documentID).Order("created_at ASC").First(&doc).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return time.Time{}, nil // 文档不存在，返回零值时间
		}
		return time.Time{}, err
	}
	return doc.CreatedAt, nil
}

// GetDocumentChunkCount 获取文档的文档块数量
func (s *PGVectorStore) GetDocumentChunkCount(ctx context.Context, documentID string) (int64, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&DocumentChunk{}).Where("document_id = ?", documentID).Count(&count).Error
	return count, err
}

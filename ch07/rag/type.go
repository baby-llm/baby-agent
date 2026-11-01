package rag

import (
	"context"
	"time"
)

type Vector = []float32

const (
	// DefaultEmbeddingModel and DefaultDimensions match OpenAI's standard
	// embedding model. Override them for local or vendor-specific services.
	DefaultEmbeddingModel = "text-embedding-3-small"
	DefaultDimensions     = 1536
)

type VectorPoint struct {
	Vector Vector `json:"vector"`
	Chunk  Chunk  `json:"chunk"`
}

type Chunk struct {
	Content string `json:"content"`
	Meta    Meta   `json:"meta"`
}

type Meta struct {
	StartPos   int    `json:"start_pos"`   // 起始位置（可以是行号、字符偏移、段落索引等）
	EndPos     int    `json:"end_pos"`     // 结束位置
	DocumentID string `json:"document_id"` // 文档唯一标识（文件路径、URL、ID等）
}

type VectorPointResult struct {
	VectorPoint
	Score float32 `json:"score"`
}

type RerankService interface {
	Rerank(ctx context.Context, query string, candidates []Chunk) ([]Chunk, error)
}

type EmbeddingService interface {
	Embed(ctx context.Context, chunk string) (Vector, error)
}

type ChunkerService interface {
	Chunk(documentID, content string) []Chunk
}

// VectorStore 向量存储接口，抽象向量数据库操作
type VectorStore interface {
	// InsertBatch 批量插入向量点
	InsertBatch(ctx context.Context, vps []VectorPoint) error

	// ReplaceDocument 在一个事务里替换指定文档的所有向量点。
	// 调用方应先完成嵌入，避免嵌入失败时删掉已有索引。
	ReplaceDocument(ctx context.Context, documentID string, vps []VectorPoint) error

	// Search 执行向量相似度搜索
	Search(ctx context.Context, queryVector Vector, limit int) ([]VectorPointResult, error)

	// DeleteByDocument 删除指定文档的所有向量
	DeleteByDocument(ctx context.Context, documentID string) error

	// GetDocumentIndexedTime 获取文档的索引时间，用于去重判断
	// 返回零值时间表示文档不存在
	GetDocumentIndexedTime(ctx context.Context, documentID string) (time.Time, error)

	// Clear 清空所有向量数据
	Clear(ctx context.Context) error

	// Close 关闭连接
	Close() error
}

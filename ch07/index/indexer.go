package index

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"babyagent/ch07/rag"
)

// Indexer 索引器，负责将代码仓库索引到向量数据库
type Indexer struct {
	rootPath     string
	fileWalker   *FileWalker
	chunker      rag.ChunkerService
	vectorStore  rag.VectorStore
	embedService rag.EmbeddingService
}

// IndexerConfig 索引器配置
type IndexerConfig struct {
	RootPath    string
	ChunkerType rag.ChunkerType
	MaxLines    int
	MaxChars    int
}

// NewIndexer 创建新的索引器
func NewIndexer(config IndexerConfig, vectorStore rag.VectorStore, embedService rag.EmbeddingService) *Indexer {
	return &Indexer{
		rootPath:     config.RootPath,
		fileWalker:   NewFileWalker(),
		chunker:      rag.NewChunker(config.ChunkerType, config.MaxLines, config.MaxChars),
		vectorStore:  vectorStore,
		embedService: embedService,
	}
}

// Index 执行索引操作
func (idx *Indexer) Index(ctx context.Context) (*IndexResult, error) {
	startTime := time.Now()

	// 1. 遍历文件
	files, err := idx.fileWalker.Walk(idx.rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to walk files: %w", err)
	}

	result := &IndexResult{
		TotalFiles: len(files),
	}

	// 2. 处理每个文件
	for _, filePath := range files {
		fileResult, err := idx.indexFile(ctx, filePath)
		if err != nil {
			result.FailedFiles++
			result.Errors = append(result.Errors, fmt.Errorf("%s: %w", filePath, err))
			continue
		}

		result.TotalChunks += fileResult.Chunks

		// 根据操作类型更新统计
		switch fileResult.Action {
		case IndexActionSkip:
			result.SkippedFiles++
		case IndexActionReindex:
			result.ReindexedFiles++
			result.SuccessFiles++
		default:
			result.SuccessFiles++
		}
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// IndexConcurrent 并发执行索引操作
func (idx *Indexer) IndexConcurrent(ctx context.Context, concurrency int) (*IndexResult, error) {
	startTime := time.Now()

	files, err := idx.fileWalker.Walk(idx.rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to walk files: %w", err)
	}

	result := &IndexResult{
		TotalFiles: len(files),
	}

	// 创建任务队列
	fileChan := make(chan string, len(files))
	for _, file := range files {
		fileChan <- file
	}
	close(fileChan)

	// 启动 worker
	var wg sync.WaitGroup
	resultMu := sync.Mutex{}

	if concurrency <= 0 {
		concurrency = 10
	}

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for filePath := range fileChan {
				fileResult, err := idx.indexFile(ctx, filePath)
				if err != nil {
					resultMu.Lock()
					result.FailedFiles++
					result.Errors = append(result.Errors, fmt.Errorf("%s: %w", filePath, err))
					resultMu.Unlock()
					continue
				}

				resultMu.Lock()
				result.TotalChunks += fileResult.Chunks

				// 根据操作类型更新统计
				switch fileResult.Action {
				case IndexActionSkip:
					result.SkippedFiles++
				case IndexActionReindex:
					result.ReindexedFiles++
					result.SuccessFiles++
				default:
					result.SuccessFiles++
				}
				resultMu.Unlock()
			}
		}()
	}

	wg.Wait()
	result.Duration = time.Since(startTime)
	return result, nil
}

// indexFile 索引单个文件（带去重）
func (idx *Indexer) indexFile(ctx context.Context, filePath string) (*FileIndexResult, error) {
	// 转换为相对路径
	relPath, err := filepath.Rel(idx.rootPath, filePath)
	if err != nil {
		relPath = filePath
	}
	// Use forward slashes so document IDs are stable across Windows/Linux.
	relPath = filepath.ToSlash(relPath)

	// 获取文件信息（用于检查修改时间）
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	// 检查文件是否已索引
	indexedTime, err := idx.vectorStore.GetDocumentIndexedTime(ctx, relPath)
	if err != nil {
		return nil, fmt.Errorf("failed to check file index status: %w", err)
	}

	// 如果文件已索引
	if !indexedTime.IsZero() {
		// 文件未修改，跳过
		if fileInfo.ModTime().Before(indexedTime) || fileInfo.ModTime().Equal(indexedTime) {
			return &FileIndexResult{
				FilePath: filePath,
				Chunks:   0,
				Action:   IndexActionSkip,
			}, nil
		}

	}

	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// 切分文本
	chunks := idx.chunker.Chunk(relPath, string(content))
	if len(chunks) == 0 {
		// 文件被清空时也要清掉旧索引。
		if err := idx.vectorStore.ReplaceDocument(ctx, relPath, nil); err != nil {
			return nil, fmt.Errorf("failed to clear old index: %w", err)
		}
		return &FileIndexResult{Chunks: 0, Action: IndexActionSkip}, nil
	}

	// 先完成全部嵌入，再在一个事务里替换旧索引；嵌入失败时旧索引保持可用。
	vectorPoints, err := idx.embedChunks(ctx, chunks)
	if err != nil {
		return nil, fmt.Errorf("failed to embed chunks: %w", err)
	}

	if err := idx.vectorStore.ReplaceDocument(ctx, relPath, vectorPoints); err != nil {
		return nil, fmt.Errorf("failed to replace document index: %w", err)
	}

	// 确定操作类型
	action := IndexActionNew
	if !indexedTime.IsZero() {
		action = IndexActionReindex
	}

	return &FileIndexResult{
		FilePath: filePath,
		Chunks:   len(chunks),
		Action:   action,
	}, nil
}

// embedChunks 批量获取向量嵌入。
//
// 使用固定大小的 worker 池，避免为每个 chunk 起一个 goroutine：
// 索引一个仓库时 chunk 数量可能上万，无限制并发会同时打爆 embedding
// 服务的限流和本地内存。
func (idx *Indexer) embedChunks(ctx context.Context, chunks []rag.Chunk) ([]rag.VectorPoint, error) {
	vectorPoints := make([]rag.VectorPoint, len(chunks))

	const maxWorkers = 8
	workers := maxWorkers
	if len(chunks) < workers {
		workers = len(chunks)
	}

	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error

	setErr := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if err := ctx.Err(); err != nil {
					setErr(err)
					continue
				}
				vector, err := idx.embedService.Embed(ctx, chunks[i].Content)
				if err != nil {
					setErr(err)
					continue
				}
				vectorPoints[i] = rag.VectorPoint{
					Vector: vector,
					Chunk:  chunks[i],
				}
			}
		}()
	}

	for i := range chunks {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	return vectorPoints, nil
}

// Search 在索引中搜索相似内容
func (idx *Indexer) Search(ctx context.Context, query string, limit int) ([]rag.VectorPointResult, error) {
	queryVector, err := idx.embedService.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to embed query: %w", err)
	}

	return idx.vectorStore.Search(ctx, queryVector, limit)
}

// Clear 清空索引
func (idx *Indexer) Clear(ctx context.Context) error {
	return idx.vectorStore.Clear(ctx)
}

// Close 关闭索引器
func (idx *Indexer) Close() error {
	return idx.vectorStore.Close()
}

// IndexResult 索引结果
type IndexResult struct {
	TotalFiles     int
	SuccessFiles   int
	FailedFiles    int
	SkippedFiles   int // 跳过的文件（已索引且未修改）
	ReindexedFiles int // 重新索引的文件（已索引但已修改）
	TotalChunks    int
	Duration       time.Duration
	Errors         []error
}

// FileIndexResult 单文件索引结果
type FileIndexResult struct {
	FilePath string
	Chunks   int
	Action   IndexAction // 执行的操作
}

// IndexAction 索引操作类型
type IndexAction string

const (
	IndexActionNew     IndexAction = "new"     // 新索引
	IndexActionSkip    IndexAction = "skip"    // 跳过（已索引且未修改）
	IndexActionReindex IndexAction = "reindex" // 重新索引（已修改）
)

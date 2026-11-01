package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/openai/openai-go/v3"

	"babyagent/ch07/rag"
)

// SemanticSearchParams 语义搜索参数
type SemanticSearchParams struct {
	Query string `json:"query" description:"搜索查询文本"`
	TopK  int    `json:"top_k" description:"返回结果数量，默认为5"`
}

// SemanticSearchTool 语义搜索工具
type SemanticSearchTool struct {
	embedService  rag.EmbeddingService
	vectorStore   rag.VectorStore
	rerankService rag.RerankService
}

// NewSemanticSearchTool 创建语义搜索工具
func NewSemanticSearchTool(
	embedService rag.EmbeddingService,
	vectorStore rag.VectorStore,
	rerankService rag.RerankService,
) *SemanticSearchTool {
	return &SemanticSearchTool{
		embedService:  embedService,
		vectorStore:   vectorStore,
		rerankService: rerankService,
	}
}

// ToolName 返回工具名称
func (s *SemanticSearchTool) ToolName() AgentTool {
	return AgentToolSemanticSearch
}

// Info 返回工具的 OpenAI 函数定义
func (s *SemanticSearchTool) Info() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(openai.FunctionDefinitionParam{
		Name:        string(AgentToolSemanticSearch),
		Description: openai.String("在向量库中进行语义搜索，查找与查询相关的文档片段"),
		Parameters: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "搜索查询文本，例如：\"如何在Go中处理错误\"",
				},
				"top_k": map[string]any{
					"type":        "integer",
					"description": "返回结果数量，默认为5",
				},
			},
			"required": []string{"query"},
		},
	})
}

// Execute 执行语义搜索
func (s *SemanticSearchTool) Execute(ctx context.Context, argumentsInJSON string) (string, error) {
	// 解析参数
	var params SemanticSearchParams
	if err := json.Unmarshal([]byte(argumentsInJSON), &params); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// 设置默认值
	if params.TopK <= 0 {
		params.TopK = 5
	}

	// 1. 将 query 转换为向量
	queryVector, err := s.embedService.Embed(ctx, params.Query)
	if err != nil {
		return "", fmt.Errorf("failed to embed query: %w", err)
	}

	// 2. 在向量存储中搜索
	vectorResults, err := s.vectorStore.Search(ctx, queryVector, params.TopK*2) // 召回更多候选用于重排序
	if err != nil {
		return "", fmt.Errorf("failed to search vectors: %w", err)
	}

	if len(vectorResults) == 0 {
		return "未找到相关结果", nil
	}

	// 3. 提取候选文档块用于重排序
	candidates := make([]rag.Chunk, len(vectorResults))
	for i, result := range vectorResults {
		candidates[i] = result.Chunk
	}

	// 4. 重排序
	var rerankedChunks []rag.Chunk
	if s.rerankService != nil {
		rerankedChunks, err = s.rerankService.Rerank(ctx, params.Query, candidates)
		if err != nil {
			// 如果重排序失败，使用原始结果
			rerankedChunks = candidates
		}
	} else {
		rerankedChunks = candidates
	}

	// 5. 重排序候选通常比 top_k 多，最终只返回请求的数量。
	if params.TopK > 0 && len(rerankedChunks) > params.TopK {
		rerankedChunks = rerankedChunks[:params.TopK]
	}

	// 6. 格式化结果
	return s.formatResults(params.Query, rerankedChunks, len(vectorResults)), nil
}

// formatResults 格式化搜索结果为可读字符串
func (s *SemanticSearchTool) formatResults(query string, chunks []rag.Chunk, totalCandidates int) string {
	result := fmt.Sprintf("语义搜索结果 (查询: %s)\n", query)
	result += fmt.Sprintf("从 %d 个候选中重排序，返回前 %d 个结果：\n\n", totalCandidates, len(chunks))

	for i, chunk := range chunks {
		result += fmt.Sprintf("--- 结果 %d ---\n", i+1)
		result += fmt.Sprintf("文档: %s\n", chunk.Meta.DocumentID)
		result += fmt.Sprintf("位置: 行 %d-%d\n", chunk.Meta.StartPos, chunk.Meta.EndPos)
		result += fmt.Sprintf("内容:\n%s\n\n", chunk.Content)
	}

	return result
}

package rag

import (
	"context"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

// HTTPRerankConfig HTTP Rerank服务配置
type HTTPRerankConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

// DefaultHTTPRerankConfig 默认配置
func DefaultHTTPRerankConfig(apiKey string) HTTPRerankConfig {
	return HTTPRerankConfig{
		APIKey:  apiKey,
		BaseURL: "",
		Model:   "rerank",
	}
}

// HTTPRerankService HTTP Rerank服务实现
type HTTPRerankService struct {
	client *resty.Client
	config HTTPRerankConfig
}

// NewHTTPRerankService 创建HTTP Rerank服务
func NewHTTPRerankService(config HTTPRerankConfig) *HTTPRerankService {
	client := resty.New().
		SetBaseURL(config.BaseURL).
		SetTimeout(60*time.Second).
		SetHeader("Authorization", "Bearer "+config.APIKey).
		SetHeader("Content-Type", "application/json")

	return &HTTPRerankService{
		client: client,
		config: config,
	}
}

// rerankRequest Rerank请求
type rerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n,omitempty"`
}

// rerankResponse Rerank响应
type rerankResponse struct {
	ID      string `json:"id"`
	Results []struct {
		Document       string  `json:"document"`
		Index          int     `json:"index"`
		RelevanceScore float32 `json:"relevance_score"`
	} `json:"results"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
	Created   string `json:"created"`
	RequestID string `json:"request_id"`
}

// Rerank 对候选文档进行重排序
func (s *HTTPRerankService) Rerank(ctx context.Context, query string, candidates []Chunk) ([]Chunk, error) {
	if len(candidates) == 0 {
		return candidates, nil
	}
	if s.config.BaseURL == "" {
		return nil, fmt.Errorf("rerank base URL is not configured")
	}

	// 构建请求
	documents := make([]string, len(candidates))
	for i, chunk := range candidates {
		documents[i] = chunk.Content
	}

	req := rerankRequest{
		Model:     s.config.Model,
		Query:     query,
		Documents: documents,
		TopN:      len(candidates),
	}

	var resp rerankResponse
	r := s.client.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&resp)

	httpResp, err := r.Post("/rerank")
	if err != nil {
		return nil, fmt.Errorf("failed to call rerank API: %w", err)
	}
	if httpResp.IsError() {
		return nil, fmt.Errorf("rerank API returned %s: %s", httpResp.Status(), httpResp.String())
	}

	// 根据重排序结果重新组织候选文档
	result := make([]Chunk, len(resp.Results))
	for i, item := range resp.Results {
		if item.Index < 0 || item.Index >= len(candidates) {
			return nil, fmt.Errorf("rerank returned invalid index %d (candidates: %d)", item.Index, len(candidates))
		}
		result[i] = candidates[item.Index]
	}

	return result, nil
}

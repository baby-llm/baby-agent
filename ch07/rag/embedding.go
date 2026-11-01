package rag

import (
	"context"
	"fmt"
	"time"

	"github.com/go-resty/resty/v2"
)

// HTTPEmbeddingConfig HTTP Embedding服务配置
type HTTPEmbeddingConfig struct {
	APIKey     string
	BaseURL    string
	Model      string
	Dimensions int
}

// DefaultHTTPEmbeddingConfig 默认配置
func DefaultHTTPEmbeddingConfig(apiKey string) HTTPEmbeddingConfig {
	return HTTPEmbeddingConfig{
		APIKey:     apiKey,
		BaseURL:    "",
		Model:      DefaultEmbeddingModel,
		Dimensions: DefaultDimensions,
	}
}

// HTTPEmbeddingService HTTP Embedding服务实现
type HTTPEmbeddingService struct {
	client *resty.Client
	config HTTPEmbeddingConfig
}

// NewHTTPEmbeddingService 创建HTTP Embedding服务
func NewHTTPEmbeddingService(config HTTPEmbeddingConfig) *HTTPEmbeddingService {
	client := resty.New().
		SetBaseURL(config.BaseURL).
		SetTimeout(60*time.Second).
		SetHeader("Authorization", "Bearer "+config.APIKey).
		SetHeader("Content-Type", "application/json")

	return &HTTPEmbeddingService{
		client: client,
		config: config,
	}
}

// embeddingRequest Embedding请求
type embeddingRequest struct {
	Model      string `json:"model"`
	Input      string `json:"input"`
	Dimensions int    `json:"dimensions,omitempty"`
}

// embeddingResponse Embedding响应
type embeddingResponse struct {
	Object string `json:"object"`
	Data   []struct {
		Index     int       `json:"index"`
		Object    string    `json:"object"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Model string `json:"model"`
}

// Embed 将文本转换为向量
func (s *HTTPEmbeddingService) Embed(ctx context.Context, text string) (Vector, error) {
	if s.config.BaseURL == "" {
		return nil, fmt.Errorf("embedding base URL is not configured")
	}

	var resp embeddingResponse

	req := embeddingRequest{
		Model: s.config.Model,
		Input: text,
	}

	// 只有非零维度才设置
	if s.config.Dimensions > 0 {
		req.Dimensions = s.config.Dimensions
	}

	r := s.client.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&resp)

	httpResp, err := r.Post("/embeddings")
	if err != nil {
		return nil, fmt.Errorf("failed to call embedding API: %w", err)
	}
	if httpResp.IsError() {
		return nil, fmt.Errorf("embedding API returned %s: %s", httpResp.Status(), httpResp.String())
	}

	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}

	return Vector(resp.Data[0].Embedding), nil
}

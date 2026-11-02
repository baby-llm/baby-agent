package ch01

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"babyagent/shared"
)

type RequestMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ResponseMessage is used for both a full message and a stream delta. Fields
// that only appear on one of them are simply empty in the other case.
type ResponseMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`

	ReasoningContent *string `json:"reasoning_content"` // vary by different model provider
	Reasoning        *string `json:"reasoning"`         // vary by different model provider
}

type Choice struct {
	Message      ResponseMessage `json:"message"`
	Delta        ResponseMessage `json:"delta"`
	FinishReason string          `json:"finish_reason"`
}

type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type OpenAIChatCompletionResponse struct {
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
}

type OpenAIChatCompletionStreamChunk struct {
	Choices []Choice `json:"choices"`
	Usage   *Usage   `json:"usage,omitempty"`
}

type OpenAIChatCompletionRequest struct {
	Model    string           `json:"model"`
	Messages []RequestMessage `json:"messages"`
	Stream   bool             `json:"stream"`
}

func NonStreamingRequestRawHTTP(ctx context.Context, modelConf shared.ModelConfig, query string) error {
	client := http.Client{Timeout: 2 * time.Minute}

	requestBody := OpenAIChatCompletionRequest{
		Messages: []RequestMessage{
			{Role: "user", Content: query},
		},
		Model:  modelConf.Model,
		Stream: false,
	}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/chat/completions", modelConf.BaseURL), bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+modelConf.ApiKey)

	httpResp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(httpResp.Body, 4096))
		return fmt.Errorf("unexpected status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(body)))
	}

	respBodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	resp := OpenAIChatCompletionResponse{}
	if err := json.Unmarshal(respBodyBytes, &resp); err != nil {
		return fmt.Errorf("unmarshal response: %w", err)
	}

	if len(resp.Choices) == 0 {
		log.Printf("no choices returned, resp: %v", resp)
		return nil
	}
	log.Printf("resp content: %s", resp.Choices[0].Message.Content)
	log.Printf("token usage: %+v", resp.Usage)
	return nil
}

func StreamingRequestRawHTTP(ctx context.Context, modelConf shared.ModelConfig, query string) error {
	client := http.Client{Timeout: 2 * time.Minute}

	requestBody := OpenAIChatCompletionRequest{
		Messages: []RequestMessage{
			{Role: "user", Content: query},
		},
		Model:  modelConf.Model,
		Stream: true,
	}
	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("%s/chat/completions", modelConf.BaseURL), bytes.NewReader(bodyBytes))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+modelConf.ApiKey)

	httpResp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(httpResp.Body, 4096))
		return fmt.Errorf("unexpected status %d: %s", httpResp.StatusCode, strings.TrimSpace(string(body)))
	}

	scanner := bufio.NewScanner(httpResp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		if line == "" {
			continue
		}

		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}

		chunk := OpenAIChatCompletionStreamChunk{}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return fmt.Errorf("unmarshal chunk: %w", err)
		}
		if len(chunk.Choices) > 0 {
			// SSE 返回的是增量，逐段打印就能看到“打字机”效果。
			fmt.Print(chunk.Choices[0].Delta.Content)
		}
		if chunk.Usage != nil {
			log.Printf("token usage: %+v", chunk.Usage)
		}
	}
	fmt.Println()
	return scanner.Err()
}

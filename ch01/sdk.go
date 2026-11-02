package ch01

import (
	"context"
	"fmt"
	"log"

	"github.com/openai/openai-go/v3"

	"babyagent/shared"
)

func NonStreamingRequestSDK(ctx context.Context, modelConf shared.ModelConfig, query string) error {
	client := shared.NewLLMClient(modelConf)

	req := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(query),
		},
		Model: modelConf.Model,
	}

	resp, err := client.Chat.Completions.New(ctx, req)
	if err != nil {
		return fmt.Errorf("send completion request: %w", err)
	}

	if len(resp.Choices) == 0 {
		log.Printf("no choices returned, resp: %s", resp.RawJSON())
		return nil
	}

	log.Printf("resp content: %s", resp.Choices[0].Message.Content)
	log.Printf("token usage: %s", resp.Usage.RawJSON())
	return nil
}

func StreamingRequestSDK(ctx context.Context, modelConf shared.ModelConfig, query string) error {
	client := shared.NewLLMClient(modelConf)

	req := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(query),
		},
		Model:         modelConf.Model,
		StreamOptions: openai.ChatCompletionStreamOptionsParam{IncludeUsage: openai.Bool(true)},
	}

	stream := client.Chat.Completions.NewStreaming(ctx, req)
	defer stream.Close()

	for stream.Next() {
		chunk := stream.Current()
		if len(chunk.Choices) > 0 {
			// SDK 已经把 SSE 解析成结构体，直接打印增量即可。
			fmt.Print(chunk.Choices[0].Delta.Content)
		}
		if chunk.Usage.TotalTokens != 0 {
			log.Printf("token usage: %s", chunk.Usage.RawJSON())
		}
	}
	fmt.Println()

	if err := stream.Err(); err != nil {
		return fmt.Errorf("stream error: %w", err)
	}
	return nil
}

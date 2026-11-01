package ch01

import (
	"context"
	"log"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"babyagent/shared"
)

func NonStreamingRequestSDK(ctx context.Context, modelConf shared.ModelConfig, query string) {
	client := openai.NewClient(option.WithBaseURL(modelConf.BaseURL), option.WithAPIKey(modelConf.ApiKey))

	req := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(query),
		},
		Model: modelConf.Model,
	}

	resp, err := client.Chat.Completions.New(ctx, req)
	if err != nil {
		log.Fatalf("failed to send a new completion request: %v", err)
		return
	}

	if len(resp.Choices) == 0 {
		log.Printf("no choices returned, resp: %s", resp.RawJSON())
		return
	}

	log.Printf("resp content: %s", resp.Choices[0].Message.Content)
	log.Printf("token usage: %s", resp.Usage.RawJSON())
}

func StreamingRequestSDK(ctx context.Context, modelConf shared.ModelConfig, query string) {
	client := openai.NewClient(option.WithBaseURL(modelConf.BaseURL), option.WithAPIKey(modelConf.ApiKey))

	req := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(query),
		},
		Model: modelConf.Model,
	}

	stream := client.Chat.Completions.NewStreaming(ctx, req)
	defer stream.Close()

	for stream.Next() {
		chunk := stream.Current()
		log.Printf("stream chunk: %s", chunk.RawJSON())
		if chunk.Usage.TotalTokens != 0 {
			log.Printf("token usage: %s", chunk.Usage.RawJSON())
		}
	}

	if stream.Err() != nil {
		log.Fatalf("stream error: %v", stream.Err())
	}
}

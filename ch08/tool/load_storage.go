package tool

import (
	"context"
	"encoding/json"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"

	"babyagent/ch08/storage"
)

type LoadStorageTool struct {
	storage storage.Storage
}

func NewLoadStorageTool(storage storage.Storage) *LoadStorageTool {
	return &LoadStorageTool{
		storage: storage,
	}
}

type LoadStorageParam struct {
	Key string `json:"key"`
}

func (t *LoadStorageTool) ToolName() AgentTool {
	return AgentToolLoadStorage
}

func (t *LoadStorageTool) Info() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:        string(AgentToolLoadStorage),
		Description: openai.String("load data from storage"),
		Parameters: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"key": map[string]any{
					"type":        "string",
					"description": "the key to load data from storage",
				},
			},
			"required": []string{"key"},
		},
	})
}

func (t *LoadStorageTool) Execute(ctx context.Context, argumentsInJSON string) (string, error) {
	p := LoadStorageParam{}
	err := json.Unmarshal([]byte(argumentsInJSON), &p)
	if err != nil {
		return "", err
	}
	output, err := t.storage.Load(ctx, p.Key)
	if err != nil {
		return "", err
	}
	return output, nil
}

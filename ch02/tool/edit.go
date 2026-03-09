package tool

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/shared"
)

type EditTool struct{}

func NewEditTool() *EditTool {
	return &EditTool{}
}

type EditToolParam struct {
	Path   string `json:"path"`
	Before string `json:"before"`
	After  string `json:"after"`
}

func (t *EditTool) ToolName() AgentTool {
	return AgentToolEdit
}

func (t *EditTool) Info() openai.ChatCompletionToolUnionParam {
	return openai.ChatCompletionFunctionTool(shared.FunctionDefinitionParam{
		Name:        string(AgentToolEdit),
		Description: openai.String("edit content in file"),
		Parameters: openai.FunctionParameters{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "the file path to edit",
				},
				"before": map[string]any{
					"type":        "string",
					"description": "the content to search for",
				},
				"after": map[string]any{
					"type":        "string",
					"description": "the content to replace with",
				},
			},
			"required": []string{"path", "before", "after"},
		},
	})
}

func (t *EditTool) Execute(ctx context.Context, argumentsInJSON string) (string, error) {
	p := EditToolParam{}
	err := json.Unmarshal([]byte(argumentsInJSON), &p)
	if err != nil {
		return "", err
	}

	raw, err := os.ReadFile(p.Path)
	if err != nil {
		return "", err
	}

	backupPath := p.Path + ".bak"
	err = os.WriteFile(backupPath, raw, 0644)
	if err != nil {
		return "", err
	}

	replaced := strings.ReplaceAll(string(raw), p.Before, p.After)

	err = os.WriteFile(p.Path, []byte(replaced), 0644)
	if err != nil {
		os.Rename(backupPath, p.Path)
		return "", err
	}

	os.Remove(backupPath)
	return "", nil
}

package tool

import (
	"context"

	"github.com/openai/openai-go/v3"
)

type AgentTool = string

const (
	AgentToolBash AgentTool = "bash"
)

type Tool interface {
	ToolName() AgentTool
	Info() openai.ChatCompletionToolUnionParam
	Execute(ctx context.Context, argumentsInJSON string) (string, error)
}

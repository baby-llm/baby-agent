package tool

import (
	"context"

	"github.com/openai/openai-go/v3"
)

type AgentTool = string

const (
	AgentToolBash      AgentTool = "bash"
	AgentToolTodoWrite AgentTool = "todo_write"
)

type Tool interface {
	ToolName() AgentTool
	Info() openai.ChatCompletionToolUnionParam
	Execute(ctx context.Context, argumentsInJSON string) (string, error)
}

package tool

import (
	"context"
	"github.com/openai/openai-go/v3"
)

type AgentTool = string

const AgentToolBash AgentTool = "bash"

type Tool interface {
	ToolName() AgentTool
	Info() openai.ChatCompletionToolUnionParam
	Execute(context.Context, string) (string, error)
}

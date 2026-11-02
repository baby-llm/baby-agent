package tool

import (
	"context"

	"github.com/openai/openai-go/v3"
)

type AgentTool string

const (
	AgentToolRead        AgentTool = "read"
	AgentToolWrite       AgentTool = "write"
	AgentToolEdit        AgentTool = "edit"
	AgentToolBash        AgentTool = "bash"
	AgentToolLoadStorage AgentTool = "load_storage"
	AgentToolLoadSkill   AgentTool = "load_skill"
)

type Tool interface {
	ToolName() AgentTool
	Info() openai.ChatCompletionToolUnionParam
	Execute(ctx context.Context, argumentsInJSON string) (string, error)
}

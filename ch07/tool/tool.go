package tool

import (
	"context"

	"github.com/openai/openai-go/v3"
)

type AgentTool string

const (
	AgentToolSemanticSearch AgentTool = "semantic_search"
	AgentToolFullTextSearch AgentTool = "full_text_search"
	AgentToolHybridSearch   AgentTool = "hybrid_search"
)

type Tool interface {
	ToolName() AgentTool
	Info() openai.ChatCompletionToolUnionParam
	Execute(ctx context.Context, argumentsInJSON string) (string, error)
}

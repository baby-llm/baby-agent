package agent

import "context"

// Message is a provider-independent message used by the agent loop and its tests.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Usage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
}
type Completion struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Usage     Usage      `json:"usage,omitempty"`
}

// Client is deliberately small so a real SDK and a deterministic mock share one boundary.
type Client interface {
	Complete(context.Context, []Message) (Completion, error)
}
type Tool interface {
	Name() string
	Execute(context.Context, string) (string, error)
}

type Result struct {
	Response  string
	Messages  []Message
	Usage     Usage
	LoopDepth int
	ToolCalls []string
}

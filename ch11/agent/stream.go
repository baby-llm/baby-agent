package agent

const (
	EventError      = "error"
	EventReasoning  = "reasoning"
	EventContent    = "content"
	EventToolCall   = "tool_call"
	EventToolResult = "tool_result"
)

type StreamEvent struct{ Event, Content, ReasoningContent, ToolCall, ToolArguments, ToolResult string }

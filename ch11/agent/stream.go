package agent

const (
	EventError      = "error"
	EventReasoning  = "reasoning"
	EventContent    = "content"
	EventToolCall   = "tool_call"
	EventToolResult = "tool_result"
)

// StreamEvent 是 agent 内部流式输出的事件类型，与传输层无关。
type StreamEvent struct {
	Event            string
	Content          string
	ReasoningContent string
	ToolCall         string
	ToolArguments    string
	ToolResult       string
}

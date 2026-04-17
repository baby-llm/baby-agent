package agent

import "babyagent/ch12/agent/plan"

const (
	EventError      = "error"
	EventReasoning  = "reasoning"
	EventContent    = "content"
	EventToolCall   = "tool_call"
	EventToolResult = "tool_result"
	EventTodoSnap   = "todo_snapshot"
)

// StreamEvent 是 agent 内部流式输出的事件类型，与传输层无关
type StreamEvent struct {
	Event            string
	Content          string
	ReasoningContent string
	ToolCall         string
	ToolArguments    string
	ToolResult       string
	PlanState        *plan.PlanningState
}

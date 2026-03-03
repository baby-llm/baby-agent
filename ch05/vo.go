package ch05

const (
	MessageTypeReasoning = "reasoning"
	MessageTypeContent   = "content"
	MessageTypeToolCall  = "tool_call"
	MessageTypeError     = "error"
	MessageTypePolicy    = "policy"
)

// MessageVO 用于流式展示当前模型流式输出或者状态
type MessageVO struct {
	Type string `json:"type"`

	ReasoningContent *string     `json:"reasoning_content,omitempty"`
	Content          *string     `json:"content,omitempty"`
	ToolCall         *ToolCallVO `json:"tool,omitempty"`
	Policy           *PolicyVO   `json:"policy,omitempty"`
}

// PolicyVO 策略执行状态
type PolicyVO struct {
	Name    string `json:"name"`    // 策略名称
	Running bool   `json:"running"` // 是否正在执行
	Error   error  `json:"error"`
}

type ToolCallVO struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

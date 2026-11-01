package vo

type SSEMessageVO struct {
	MessageID        string  `json:"message_id"`
	Event            string  `json:"event"`
	Content          *string `json:"content,omitempty"`
	ReasoningContent *string `json:"reasoning_content,omitempty"`
	ToolCall         *string `json:"tool_call,omitempty"`
	ToolArguments    *string `json:"tool_arguments,omitempty"`
	ToolResult       *string `json:"tool_result,omitempty"`
}

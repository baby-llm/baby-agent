package observe

import (
	"sync"
	"time"

	"github.com/google/uuid"
)

type AgentTrace struct {
	TraceID, ConversationID, MessageID, Query, Status string
	StartTime, EndTime                                time.Time
	Events                                            []TraceEvent
	Summary                                           TraceSummary
	mu                                                sync.Mutex
}
type TraceEvent struct {
	EventID, Type string
	Timestamp     time.Time
	Duration      time.Duration
	Attrs         map[string]any
}
type TraceSummary struct {
	TotalDuration                          time.Duration
	LoopDepth, LLMCallCount, ToolCallCount int
	TotalPromptTokens, TotalCompletTokens  int64
}

func NewAgentTrace(traceID, conversationID, messageID, query string) *AgentTrace {
	t := &AgentTrace{TraceID: traceID, ConversationID: conversationID, MessageID: messageID, Query: query, StartTime: time.Now(), Status: "running"}
	t.Add("query_start", 0, map[string]any{"query_length": len(query)})
	return t
}
func (t *AgentTrace) Add(kind string, duration time.Duration, attrs map[string]any) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.Events = append(t.Events, TraceEvent{EventID: uuid.NewString(), Type: kind, Timestamp: time.Now(), Duration: duration, Attrs: attrs})
}
func (t *AgentTrace) LLM(duration time.Duration, prompt, completion int64, firstToken time.Duration, toolCalls int) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.Summary.LLMCallCount++
	t.Summary.LoopDepth++
	t.Summary.TotalPromptTokens += prompt
	t.Summary.TotalCompletTokens += completion
	t.mu.Unlock()
	t.Add("llm_thinking", duration, map[string]any{"prompt_tokens": prompt, "completion_tokens": completion, "first_token_ms": firstToken.Milliseconds(), "tool_calls": toolCalls})
}
func (t *AgentTrace) Tool(name string, duration time.Duration, err error) {
	if t == nil {
		return
	}
	status := "ok"
	if err != nil {
		status = "error"
	}
	t.mu.Lock()
	t.Summary.ToolCallCount++
	t.mu.Unlock()
	t.Add("tool_call", duration, map[string]any{"tool": name, "status": status})
}
func (t *AgentTrace) End(status string) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.EndTime = time.Now()
	t.Status = status
	t.Summary.TotalDuration = t.EndTime.Sub(t.StartTime)
	t.mu.Unlock()
	t.Add("query_end", 0, map[string]any{"status": status})
}

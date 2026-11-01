package agent

import (
	"context"
	"fmt"
	"sync"
)

// MockClient returns queued completions in order and records every request.
// It makes agent-loop tests independent of model availability, latency, and cost.
type MockClient struct {
	mu        sync.Mutex
	Responses []Completion
	Err       error
	Requests  [][]Message
}

func (m *MockClient) Complete(_ context.Context, messages []Message) (Completion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Requests = append(m.Requests, append([]Message(nil), messages...))
	if m.Err != nil {
		return Completion{}, m.Err
	}
	if len(m.Responses) == 0 {
		return Completion{}, fmt.Errorf("mock has no response for request %d", len(m.Requests))
	}
	r := m.Responses[0]
	m.Responses = m.Responses[1:]
	return r, nil
}

type MockTool struct {
	ToolName, Output string
	Err              error
	Calls            []string
}

func (t *MockTool) Name() string { return t.ToolName }
func (t *MockTool) Execute(_ context.Context, arguments string) (string, error) {
	t.Calls = append(t.Calls, arguments)
	return t.Output, t.Err
}

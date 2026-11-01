package observe

import (
	"errors"
	"testing"
	"time"
)

func TestAgentTrace_AggregatesLLMAndToolEvents(t *testing.T) {
	tr := NewAgentTrace("trace-1", "conversation-1", "message-1", "hello")
	tr.LLM(time.Second, 12, 8, 100*time.Millisecond, 1)
	tr.Tool("bash", 50*time.Millisecond, nil)
	tr.Tool("bash", 20*time.Millisecond, errors.New("exit status 1"))
	tr.End("error")

	if tr.Summary.LLMCallCount != 1 || tr.Summary.LoopDepth != 1 {
		t.Fatalf("LLM summary = %+v, want one loop", tr.Summary)
	}
	if tr.Summary.TotalPromptTokens != 12 || tr.Summary.TotalCompletTokens != 8 {
		t.Fatalf("token summary = %+v", tr.Summary)
	}
	if tr.Summary.ToolCallCount != 2 || tr.Status != "error" || tr.EndTime.IsZero() {
		t.Fatalf("trace final state = %+v", tr)
	}
	if len(tr.Events) != 5 { // start + llm + two tools + end
		t.Fatalf("event count = %d, want 5", len(tr.Events))
	}
}

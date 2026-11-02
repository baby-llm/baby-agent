package server

import "testing"

func TestBuildHistory_IgnoresUnknownParent(t *testing.T) {
	msgs := []ChatMessage{{MessageID: "m1", Rounds: `[{"role":"user","content":"hi"}]`}}
	if got := buildHistory(msgs, "missing"); len(got) != 0 {
		t.Fatalf("buildHistory() len = %d, want 0", len(got))
	}
}

func TestBuildHistory_HandlesCycle(t *testing.T) {
	msgs := []ChatMessage{
		{MessageID: "m1", ParentMessageID: "m2", Rounds: `[{"role":"user","content":"one"}]`},
		{MessageID: "m2", ParentMessageID: "m1", Rounds: `[{"role":"user","content":"two"}]`},
	}
	got := buildHistory(msgs, "m1")
	if len(got) != 2 {
		t.Fatalf("buildHistory() len = %d, want 2 (cycle must not hang or duplicate)", len(got))
	}
}

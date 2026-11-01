package server

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"babyagent/ch11/observe"
)

func TestGetTrace_DecodesPersistedSummaryAndEvents(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(db, nil)
	summary, _ := json.Marshal(observe.TraceSummary{LoopDepth: 2})
	events, _ := json.Marshal([]observe.TraceEvent{{Type: "query_start"}})
	if err := db.Create(&AgentTrace{TraceID: "trace-1", ConversationID: "conv-1", MessageID: "msg-1", Status: "ok", StartTime: 1, EndTime: 2, Summary: string(summary), Events: string(events)}).Error; err != nil {
		t.Fatal(err)
	}

	got, err := s.GetTrace("trace-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.TraceID != "trace-1" || got.Status != "ok" || got.Summary == nil || got.Events == nil {
		t.Fatalf("trace = %+v", got)
	}
}

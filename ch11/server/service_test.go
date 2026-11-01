package server

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"babyagent/ch11/observe"
	"babyagent/ch11/vo"
)

func TestRenameConversation_UpdatesTitle(t *testing.T) {
	s := newTestServer(t)

	created, err := s.CreateConversation(vo.CreateConversationReq{
		UserID: "user_001",
		Title:  "Old Title",
	})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}

	updated, err := s.RenameConversation(created.ConversationID, "New Title")
	if err != nil {
		t.Fatalf("RenameConversation() error = %v", err)
	}
	if updated.Title != "New Title" {
		t.Fatalf("updated title = %q, want %q", updated.Title, "New Title")
	}

	var stored Conversation
	if err := s.db.First(&stored, "conversation_id = ?", created.ConversationID).Error; err != nil {
		t.Fatalf("load stored conversation: %v", err)
	}
	if stored.Title != "New Title" {
		t.Fatalf("stored title = %q, want %q", stored.Title, "New Title")
	}
}

func TestDeleteConversation_RemovesConversationMessagesAndTraces(t *testing.T) {
	s := newTestServer(t)

	created, err := s.CreateConversation(vo.CreateConversationReq{
		UserID: "user_001",
		Title:  "Delete Me",
	})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}

	if err := s.db.Create(&ChatMessage{
		MessageID:      "msg-1",
		UserID:         "user_001",
		ConversationID: created.ConversationID,
		Query:          "hello",
		Response:       "world",
		Model:          "test-model",
		CreatedAt:      time.Now().Unix(),
	}).Error; err != nil {
		t.Fatalf("seed chat message: %v", err)
	}
	if err := s.db.Create(&AgentTrace{
		TraceID:        "trace-1",
		ConversationID: created.ConversationID,
		MessageID:      "msg-1",
		Status:         "ok",
	}).Error; err != nil {
		t.Fatalf("seed agent trace: %v", err)
	}

	if err := s.DeleteConversation(created.ConversationID); err != nil {
		t.Fatalf("DeleteConversation() error = %v", err)
	}

	for name, model := range map[string]any{
		"conversations": &Conversation{},
		"messages":      &ChatMessage{},
		"traces":        &AgentTrace{},
	} {
		var count int64
		if err := s.db.Model(model).
			Where("conversation_id = ?", created.ConversationID).
			Count(&count).Error; err != nil {
			t.Fatalf("count %s: %v", name, err)
		}
		if count != 0 {
			t.Fatalf("%s count = %d, want 0", name, count)
		}
	}
}

func TestGetTrace_DecodesPersistedSummaryAndEvents(t *testing.T) {
	db, err := InitDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := CloseDB(db); err != nil {
			t.Errorf("CloseDB() error = %v", err)
		}
	})
	s := NewServer(db, nil)

	summary, _ := json.Marshal(observe.TraceSummary{LoopDepth: 2})
	events, _ := json.Marshal([]observe.TraceEvent{{Type: "query_start"}})
	if err := db.Create(&AgentTrace{
		TraceID:        "trace-1",
		ConversationID: "conv-1",
		MessageID:      "msg-1",
		Status:         "ok",
		StartTime:      1,
		EndTime:        2,
		Summary:        string(summary),
		Events:         string(events),
	}).Error; err != nil {
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

func newTestServer(t *testing.T) *Server {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}
	t.Cleanup(func() {
		if err := CloseDB(db); err != nil {
			t.Errorf("CloseDB() error = %v", err)
		}
	})

	return NewServer(db, nil)
}

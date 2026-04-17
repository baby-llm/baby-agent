package server

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"babyagent/ch12/agent"
	"babyagent/ch12/agent/plan"
	"babyagent/ch12/vo"
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

func TestDeleteConversation_RemovesConversationAndMessages(t *testing.T) {
	s := newTestServer(t)

	created, err := s.CreateConversation(vo.CreateConversationReq{
		UserID: "user_001",
		Title:  "Delete Me",
	})
	if err != nil {
		t.Fatalf("CreateConversation() error = %v", err)
	}

	if err := s.db.Create(&ChatMessage{
		MessageID:       "msg-1",
		UserID:          "user_001",
		ConversationID:  created.ConversationID,
		ParentMessageID: "",
		Query:           "hello",
		Response:        "world",
		Model:           "test-model",
		CreatedAt:       time.Now().Unix(),
	}).Error; err != nil {
		t.Fatalf("seed chat message: %v", err)
	}

	if err := s.DeleteConversation(created.ConversationID); err != nil {
		t.Fatalf("DeleteConversation() error = %v", err)
	}

	var conversationCount int64
	if err := s.db.Model(&Conversation{}).
		Where("conversation_id = ?", created.ConversationID).
		Count(&conversationCount).Error; err != nil {
		t.Fatalf("count conversations: %v", err)
	}
	if conversationCount != 0 {
		t.Fatalf("conversation count = %d, want 0", conversationCount)
	}

	var messageCount int64
	if err := s.db.Model(&ChatMessage{}).
		Where("conversation_id = ?", created.ConversationID).
		Count(&messageCount).Error; err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if messageCount != 0 {
		t.Fatalf("message count = %d, want 0", messageCount)
	}
}

func TestListMessages_ParsesPlanState(t *testing.T) {
	s := newTestServer(t)

	planState, err := json.Marshal(vo.PlanningStateVO{
		Items: []vo.PlanItemVO{
			{Content: "Inspect login flow", Status: "completed"},
			{Content: "Patch handler", Status: "in_progress"},
		},
		Revision:        2,
		LastUpdatedLoop: 3,
	})
	if err != nil {
		t.Fatalf("marshal plan state: %v", err)
	}

	if err := s.db.Create(&ChatMessage{
		MessageID:       "msg-1",
		UserID:          "user_001",
		ConversationID:  "conv-1",
		ParentMessageID: "",
		Query:           "hello",
		Response:        "world",
		Model:           "test-model",
		PlanState:       string(planState),
		CreatedAt:       time.Now().Unix(),
	}).Error; err != nil {
		t.Fatalf("seed chat message: %v", err)
	}

	result, err := s.ListMessages("conv-1")
	if err != nil {
		t.Fatalf("ListMessages() error = %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("len(result) = %d, want 1", len(result))
	}
	if result[0].PlanState == nil {
		t.Fatal("result[0].PlanState = nil, want parsed plan state")
	}
	if got, want := result[0].PlanState.Revision, 2; got != want {
		t.Fatalf("result[0].PlanState.Revision = %d, want %d", got, want)
	}
}

func TestToSSEMessage_MapsTodoSnapshot(t *testing.T) {
	event := agent.StreamEvent{
		Event: agent.EventTodoSnap,
		PlanState: &plan.PlanningState{
			Items: []plan.PlanItem{
				{Content: "Inspect login flow", Status: plan.PlanStatusPending},
			},
			Revision:        1,
			LastUpdatedLoop: 2,
		},
	}

	got := toSSEMessage("msg-1", event)
	if got.PlanState == nil {
		t.Fatal("got.PlanState = nil, want plan state")
	}
	if got.Event != agent.EventTodoSnap {
		t.Fatalf("got.Event = %q, want %q", got.Event, agent.EventTodoSnap)
	}
}

func newTestServer(t *testing.T) *Server {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}

	return NewServer(db, nil)
}

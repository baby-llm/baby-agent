package server

import (
	"path/filepath"
	"testing"
	"time"

	"babyagent/ch10/vo"
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

func newTestServer(t *testing.T) *Server {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := InitDB(dbPath)
	if err != nil {
		t.Fatalf("InitDB() error = %v", err)
	}

	return NewServer(db, nil)
}

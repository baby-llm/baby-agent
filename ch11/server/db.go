package server

import (
	"github.com/libtnb/sqlite"
	"gorm.io/gorm"
)

type Conversation struct {
	ConversationID string `gorm:"primaryKey"`
	UserID         string `gorm:"index"`
	Title          string
	CreatedAt      int64
}
type ChatMessage struct {
	MessageID       string `gorm:"primaryKey"`
	UserID          string `gorm:"index"`
	ConversationID  string `gorm:"index"`
	ParentMessageID string
	Query           string
	Response        string
	Rounds          string
	Model           string
	Usage           string
	CreatedAt       int64
}

// AgentTrace stores the semantic execution trace separately from transport spans.
type AgentTrace struct {
	TraceID        string `gorm:"primaryKey"`
	ConversationID string `gorm:"index"`
	MessageID      string `gorm:"index"`
	Query          string
	Status         string `gorm:"index"`
	StartTime      int64
	EndTime        int64
	Summary        string
	Events         string
}

func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	return db, db.AutoMigrate(&Conversation{}, &ChatMessage{}, &AgentTrace{})
}

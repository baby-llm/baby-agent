package server

import (
	"strings"

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
	// busy_timeout + WAL keep concurrent SSE completions from failing with
	// SQLITE_BUSY when two requests finish at the same time.
	if !strings.Contains(dsn, "_pragma=") {
		sep := "?"
		if strings.Contains(dsn, "?") {
			sep = "&"
		}
		dsn += sep + "_txlock=immediate&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	}

	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	if sqlDB, err := db.DB(); err == nil {
		// SQLite has a single writer; serializing connections avoids lock
		// contention in this teaching server.
		sqlDB.SetMaxOpenConns(1)
	}
	return db, db.AutoMigrate(&Conversation{}, &ChatMessage{}, &AgentTrace{})
}

// CloseDB closes the underlying SQL connection. Tests must call it before
// t.TempDir() cleanup, otherwise Windows cannot remove the open SQLite file.
func CloseDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

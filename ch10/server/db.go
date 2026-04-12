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

	Query    string // 用户的原始提问
	Response string // 模型的最终输出
	Rounds   string // 用户提问到模型结束 tool loop 之间所有的 llm 请求，以 json 存储

	Model string // 使用的模型
	Usage string

	CreatedAt int64
}

func InitDB(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	err = db.AutoMigrate(&Conversation{}, &ChatMessage{})
	if err != nil {
		return nil, err
	}
	return db, nil
}

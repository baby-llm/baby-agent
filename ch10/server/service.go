package server

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"babyagent/ch10/agent"
	"babyagent/ch10/vo"
	"babyagent/shared"
	"babyagent/shared/log"
)

type Server struct {
	db    *gorm.DB
	agent *agent.Agent
}

func NewServer(db *gorm.DB, agent *agent.Agent) *Server {
	return &Server{db: db, agent: agent}
}

func (s *Server) CreateConversation(req vo.CreateConversationReq) (vo.ConversationVO, error) {
	conv := Conversation{
		ConversationID: uuid.New().String(),
		UserID:         req.UserID,
		Title:          req.Title,
		CreatedAt:      time.Now().Unix(),
	}
	if err := s.db.Create(&conv).Error; err != nil {
		return vo.ConversationVO{}, err
	}
	return vo.ConversationVO{
		ConversationID: conv.ConversationID,
		UserID:         conv.UserID,
		Title:          conv.Title,
		CreatedAt:      conv.CreatedAt,
	}, nil
}

func (s *Server) ListConversations(userID string) ([]vo.ConversationVO, error) {
	var convs []Conversation
	query := s.db.Order("created_at desc")
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if err := query.Find(&convs).Error; err != nil {
		return nil, err
	}

	result := make([]vo.ConversationVO, 0, len(convs))
	for _, conv := range convs {
		result = append(result, vo.ConversationVO{
			ConversationID: conv.ConversationID,
			UserID:         conv.UserID,
			Title:          conv.Title,
			CreatedAt:      conv.CreatedAt,
		})
	}
	return result, nil
}

func (s *Server) ListMessages(conversationID string) ([]vo.ChatMessageVO, error) {
	var msgs []ChatMessage
	if err := s.db.Where("conversation_id = ?", conversationID).
		Order("created_at asc").Find(&msgs).Error; err != nil {
		return nil, err
	}

	result := make([]vo.ChatMessageVO, 0, len(msgs))
	for _, msg := range msgs {
		result = append(result, vo.ChatMessageVO{
			MessageID:       msg.MessageID,
			ConversationID:  msg.ConversationID,
			ParentMessageID: msg.ParentMessageID,
			Query:           msg.Query,
			Response:        msg.Response,
			Model:           msg.Model,
			CreatedAt:       msg.CreatedAt,
			Rounds:          parseRounds(msg.Rounds),
		})
	}
	return result, nil
}

// CreateMessage 验证会话、构建历史、保存消息记录，并启动 agent 流式执行。
func (s *Server) CreateMessage(ctx context.Context, conversationID string, req vo.CreateMessageReq, voCh chan<- vo.SSEMessageVO) error {
	// 验证会话存在
	var conv Conversation
	if err := s.db.Where("conversation_id = ?", conversationID).First(&conv).Error; err != nil {
		return err
	}

	// 从历史消息构建 history
	var historyMsgs []ChatMessage
	if err := s.db.Where("conversation_id = ?", conversationID).
		Order("created_at asc").Find(&historyMsgs).Error; err != nil {
		return err
	}
	history := buildHistory(historyMsgs, req.ParentMessageID)

	msgID := uuid.New().String()
	createdAt := time.Now().Unix()

	eventCh := make(chan agent.StreamEvent, 64)
	defer func() {
		close(eventCh)
	}()

	go func() {
		for e := range eventCh {
			voCh <- toSSEMessage(msgID, e)
		}
	}()

	result, runErr := s.agent.RunStreaming(ctx, history, req.Query, eventCh)
	if runErr != nil {
		log.Warnf("run streaming error: %v", runErr)
	}

	roundsJSON, _ := json.Marshal(result.Rounds)
	usageJSON, _ := json.Marshal(result.Usage)
	s.db.Create(&ChatMessage{
		MessageID:       msgID,
		UserID:          req.UserID,
		ConversationID:  conversationID,
		ParentMessageID: req.ParentMessageID,
		Query:           req.Query,
		Response:        result.Response,
		Rounds:          string(roundsJSON),
		Usage:           string(usageJSON),
		Model:           s.agent.Model(),
		CreatedAt:       createdAt,
	})

	return nil
}

func toSSEMessage(msgID string, e agent.StreamEvent) vo.SSEMessageVO {
	msg := vo.SSEMessageVO{MessageID: msgID, Event: e.Event}
	switch e.Event {
	case agent.EventReasoning:
		msg.ReasoningContent = &e.ReasoningContent
	case agent.EventContent, agent.EventError:
		msg.Content = &e.Content
	case agent.EventToolCall:
		msg.ToolCall = &e.ToolCall
		msg.ToolArguments = &e.ToolArguments
	case agent.EventToolResult:
		msg.ToolCall = &e.ToolCall
		msg.ToolResult = &e.ToolResult
	}
	return msg
}

// parseRounds 将存储的 rounds JSON 转换为前端友好的 RoundMessageVO 列表。
func parseRounds(roundsJSON string) []vo.RoundMessageVO {
	if roundsJSON == "" {
		return nil
	}
	var msgs []shared.OpenAIMessage
	if err := json.Unmarshal([]byte(roundsJSON), &msgs); err != nil {
		return nil
	}

	result := make([]vo.RoundMessageVO, 0, len(msgs))
	for _, m := range msgs {
		switch {
		case m.OfUser != nil:
			// user 消息不需要展示
			continue

		case m.OfAssistant != nil:
			a := m.OfAssistant
			rv := vo.RoundMessageVO{Role: "assistant"}
			if len(a.ToolCalls) > 0 {
				for _, tc := range a.ToolCalls {
					if tc.OfFunction != nil {
						rv.ToolCalls = append(rv.ToolCalls, vo.ToolCallVO{
							ID:        tc.OfFunction.ID,
							Name:      tc.OfFunction.Function.Name,
							Arguments: tc.OfFunction.Function.Arguments,
						})
					}
				}
				result = append(result, rv)
			}

		case m.OfTool != nil:
			t := m.OfTool
			result = append(result, vo.RoundMessageVO{
				Role:    "tool",
				ToolID:  t.ToolCallID,
				Content: t.Content.OfString.Value,
			})
		}
	}
	return result
}

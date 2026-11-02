package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"babyagent/ch11/agent"
	"babyagent/ch11/observe"
	"babyagent/ch11/vo"
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
	query := s.db.Order("created_at desc, rowid desc")
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

func (s *Server) RenameConversation(conversationID string, title string) (vo.ConversationVO, error) {
	if err := s.db.Model(&Conversation{}).
		Where("conversation_id = ?", conversationID).
		Update("title", title).Error; err != nil {
		return vo.ConversationVO{}, err
	}

	var conv Conversation
	if err := s.db.First(&conv, "conversation_id = ?", conversationID).Error; err != nil {
		return vo.ConversationVO{}, err
	}

	return vo.ConversationVO{
		ConversationID: conv.ConversationID,
		UserID:         conv.UserID,
		Title:          conv.Title,
		CreatedAt:      conv.CreatedAt,
	}, nil
}

func (s *Server) DeleteConversation(conversationID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("conversation_id = ?", conversationID).
			Delete(&ChatMessage{}).Error; err != nil {
			return err
		}
		if err := tx.Where("conversation_id = ?", conversationID).
			Delete(&AgentTrace{}).Error; err != nil {
			return err
		}

		return tx.Where("conversation_id = ?", conversationID).
			Delete(&Conversation{}).Error
	})
}

func (s *Server) ListMessages(conversationID string) ([]vo.ChatMessageVO, error) {
	var msgs []ChatMessage
	if err := s.db.Where("conversation_id = ?", conversationID).
		Order("created_at asc, rowid asc").Find(&msgs).Error; err != nil {
		return nil, err
	}

	// 一条消息可能有多条 trace 记录（重试等），取最近的一条即可。
	var traces []AgentTrace
	if err := s.db.Where("conversation_id = ?", conversationID).
		Order("start_time asc").Find(&traces).Error; err != nil {
		return nil, err
	}
	traceIDs := make(map[string]string, len(traces))
	for _, trace := range traces {
		traceIDs[trace.MessageID] = trace.TraceID
	}

	result := make([]vo.ChatMessageVO, 0, len(msgs))
	for _, msg := range msgs {
		result = append(result, vo.ChatMessageVO{
			MessageID:       msg.MessageID,
			TraceID:         traceIDs[msg.MessageID],
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
// 同时创建一条语义 trace（一次 Query = 一条 Trace）。
func (s *Server) CreateMessage(ctx context.Context, conversationID string, req vo.CreateMessageReq, voCh chan<- vo.SSEMessageVO) error {
	// 验证会话存在
	var conv Conversation
	if err := s.db.Where("conversation_id = ?", conversationID).First(&conv).Error; err != nil {
		return err
	}

	// 从历史消息构建 history
	var historyMsgs []ChatMessage
	if err := s.db.Where("conversation_id = ?", conversationID).
		Order("created_at asc, rowid asc").Find(&historyMsgs).Error; err != nil {
		return err
	}
	history := buildHistory(historyMsgs, req.ParentMessageID)

	msgID := uuid.New().String()
	createdAt := time.Now().Unix()

	ctx, span := observe.StartAgentSpan(ctx, conversationID, msgID)
	defer span.End()

	trace := observe.NewAgentTrace(span.SpanContext().TraceID().String(), conversationID, msgID, req.Query)

	eventCh := make(chan agent.StreamEvent, 64)
	// The pump is the only goroutine that writes to voCh. CreateMessage waits
	// for it before returning, so the HTTP layer can safely close voCh after
	// this function returns without racing with a pending send.
	pumpDone := make(chan struct{})
	go func() {
		defer close(pumpDone)
		for e := range eventCh {
			select {
			case voCh <- toSSEMessage(msgID, trace.TraceID, e):
			case <-ctx.Done():
				return
			}
		}
	}()

	result, runErr := s.agent.RunStreaming(ctx, history, req.Query, eventCh, trace)
	close(eventCh)
	<-pumpDone

	status := "ok"
	if runErr != nil {
		status = "error"
		span.RecordError(runErr)
	}
	trace.End(status)
	s.saveTrace(trace)

	if runErr != nil {
		// The agent already emitted an EventError before returning; logging here
		// avoids a duplicate error event on the wire.
		log.Warnf("run streaming error: %v", runErr)
		return nil
	}

	roundsJSON, err := json.Marshal(result.Rounds)
	if err != nil {
		return fmt.Errorf("marshal rounds: %w", err)
	}
	usageJSON, err := json.Marshal(result.Usage)
	if err != nil {
		return fmt.Errorf("marshal usage: %w", err)
	}
	if err := s.db.Create(&ChatMessage{
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
	}).Error; err != nil {
		return fmt.Errorf("save message: %w", err)
	}

	return nil
}

// saveTrace 把语义轨迹写入 SQLite。trace 失败不应影响对话本身，因此只记日志。
func (s *Server) saveTrace(trace *observe.AgentTrace) {
	summary, err := json.Marshal(trace.Summary)
	if err != nil {
		log.Warnf("marshal trace summary: %v", err)
	}
	events, err := json.Marshal(trace.Events)
	if err != nil {
		log.Warnf("marshal trace events: %v", err)
	}
	if err := s.db.Create(&AgentTrace{
		TraceID:        trace.TraceID,
		ConversationID: trace.ConversationID,
		MessageID:      trace.MessageID,
		Query:          trace.Query,
		Status:         trace.Status,
		StartTime:      trace.StartTime.UnixMilli(),
		EndTime:        trace.EndTime.UnixMilli(),
		Summary:        string(summary),
		Events:         string(events),
	}).Error; err != nil {
		log.Warnf("save agent trace: %v", err)
	}
}

func toSSEMessage(msgID, traceID string, e agent.StreamEvent) vo.SSEMessageVO {
	msg := vo.SSEMessageVO{MessageID: msgID, TraceID: traceID, Event: e.Event}
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

func (s *Server) GetTrace(id string) (vo.TraceVO, error) {
	var trace AgentTrace
	if err := s.db.First(&trace, "trace_id = ?", id).Error; err != nil {
		return vo.TraceVO{}, err
	}

	var summary, events any
	_ = json.Unmarshal([]byte(trace.Summary), &summary)
	_ = json.Unmarshal([]byte(trace.Events), &events)
	return vo.TraceVO{
		TraceID:        trace.TraceID,
		ConversationID: trace.ConversationID,
		MessageID:      trace.MessageID,
		Status:         trace.Status,
		StartTime:      trace.StartTime,
		EndTime:        trace.EndTime,
		Summary:        summary,
		Events:         events,
	}, nil
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

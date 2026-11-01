package server

import (
	"babyagent/ch11/agent"
	"babyagent/ch11/observe"
	"babyagent/ch11/vo"
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"time"
)

type Server struct {
	db    *gorm.DB
	agent *agent.Agent
}

func NewServer(db *gorm.DB, a *agent.Agent) *Server { return &Server{db: db, agent: a} }
func (s *Server) CreateConversation(req vo.CreateConversationReq) (vo.ConversationVO, error) {
	c := Conversation{ConversationID: uuid.NewString(), UserID: req.UserID, Title: req.Title, CreatedAt: time.Now().Unix()}
	if err := s.db.Create(&c).Error; err != nil {
		return vo.ConversationVO{}, err
	}
	return vo.ConversationVO{ConversationID: c.ConversationID, UserID: c.UserID, Title: c.Title, CreatedAt: c.CreatedAt}, nil
}
func (s *Server) ListConversations(userID string) ([]vo.ConversationVO, error) {
	var cs []Conversation
	q := s.db.Order("created_at desc")
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	}
	if err := q.Find(&cs).Error; err != nil {
		return nil, err
	}
	r := make([]vo.ConversationVO, 0, len(cs))
	for _, c := range cs {
		r = append(r, vo.ConversationVO{ConversationID: c.ConversationID, UserID: c.UserID, Title: c.Title, CreatedAt: c.CreatedAt})
	}
	return r, nil
}
func (s *Server) ListMessages(id string) ([]vo.ChatMessageVO, error) {
	var ms []ChatMessage
	if err := s.db.Where("conversation_id = ?", id).Order("created_at asc").Find(&ms).Error; err != nil {
		return nil, err
	}
	var traces []AgentTrace
	if err := s.db.Where("conversation_id = ?", id).Find(&traces).Error; err != nil {
		return nil, err
	}
	traceIDs := make(map[string]string, len(traces))
	for _, tr := range traces {
		traceIDs[tr.MessageID] = tr.TraceID
	}
	r := make([]vo.ChatMessageVO, 0, len(ms))
	for _, m := range ms {
		r = append(r, vo.ChatMessageVO{MessageID: m.MessageID, TraceID: traceIDs[m.MessageID], ConversationID: m.ConversationID, ParentMessageID: m.ParentMessageID, Query: m.Query, Response: m.Response, Model: m.Model, CreatedAt: m.CreatedAt})
	}
	return r, nil
}
func (s *Server) CreateMessage(ctx context.Context, conversationID string, req vo.CreateMessageReq, output chan<- vo.SSEMessageVO) error {
	var conv Conversation
	if err := s.db.Where("conversation_id = ?", conversationID).First(&conv).Error; err != nil {
		return err
	}
	var past []ChatMessage
	if err := s.db.Where("conversation_id = ?", conversationID).Order("created_at asc").Find(&past).Error; err != nil {
		return err
	}
	id := uuid.NewString()
	ctx, span := observe.StartAgentSpan(ctx, conversationID, id)
	defer span.End()
	tr := observe.NewAgentTrace(span.SpanContext().TraceID().String(), conversationID, id, req.Query)
	events := make(chan agent.StreamEvent, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for e := range events {
			select {
			case output <- toSSE(id, e):
			case <-ctx.Done():
				return
			}
		}
	}()
	result, runErr := s.agent.RunStreaming(ctx, buildHistory(past, req.ParentMessageID), req.Query, events, tr)
	close(events)
	<-done
	status := "ok"
	if runErr != nil {
		status = "error"
		span.RecordError(runErr)
	}
	tr.End(status)
	summary, _ := json.Marshal(tr.Summary)
	traceEvents, _ := json.Marshal(tr.Events)
	_ = s.db.Create(&AgentTrace{TraceID: tr.TraceID, ConversationID: conversationID, MessageID: id, Query: req.Query, Status: tr.Status, StartTime: tr.StartTime.UnixMilli(), EndTime: tr.EndTime.UnixMilli(), Summary: string(summary), Events: string(traceEvents)}).Error
	rounds, _ := json.Marshal(result.Rounds)
	usage, _ := json.Marshal(result.Usage)
	if err := s.db.Create(&ChatMessage{MessageID: id, UserID: req.UserID, ConversationID: conversationID, ParentMessageID: req.ParentMessageID, Query: req.Query, Response: result.Response, Rounds: string(rounds), Usage: string(usage), Model: s.agent.Model(), CreatedAt: time.Now().Unix()}).Error; err != nil {
		return err
	}
	return runErr
}
func toSSE(id string, e agent.StreamEvent) vo.SSEMessageVO {
	m := vo.SSEMessageVO{MessageID: id, Event: e.Event}
	switch e.Event {
	case agent.EventReasoning:
		m.ReasoningContent = &e.ReasoningContent
	case agent.EventContent, agent.EventError:
		m.Content = &e.Content
	case agent.EventToolCall:
		m.ToolCall = &e.ToolCall
		m.ToolArguments = &e.ToolArguments
	case agent.EventToolResult:
		m.ToolCall = &e.ToolCall
		m.ToolResult = &e.ToolResult
	}
	return m
}
func (s *Server) GetTrace(id string) (vo.TraceVO, error) {
	var t AgentTrace
	if err := s.db.First(&t, "trace_id = ?", id).Error; err != nil {
		return vo.TraceVO{}, err
	}
	var summary, events any
	_ = json.Unmarshal([]byte(t.Summary), &summary)
	_ = json.Unmarshal([]byte(t.Events), &events)
	return vo.TraceVO{TraceID: t.TraceID, ConversationID: t.ConversationID, MessageID: t.MessageID, Status: t.Status, StartTime: t.StartTime, EndTime: t.EndTime, Summary: summary, Events: events}, nil
}

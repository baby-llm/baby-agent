package vo

type R struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data,omitempty"`
}

func OK(data any) R              { return R{Code: 0, Msg: "ok", Data: data} }
func Err(code int, msg string) R { return R{Code: code, Msg: msg} }

type CreateConversationReq struct {
	UserID string `json:"user_id" binding:"required"`
	Title  string `json:"title"`
}
type UpdateConversationReq struct {
	Title string `json:"title" binding:"required"`
}
type CreateMessageReq struct {
	UserID          string `json:"user_id" binding:"required"`
	Query           string `json:"query" binding:"required"`
	ParentMessageID string `json:"parent_message_id"`
}
type ConversationVO struct {
	ConversationID string `json:"conversation_id"`
	UserID         string `json:"user_id"`
	Title          string `json:"title"`
	CreatedAt      int64  `json:"created_at"`
}
type ChatMessageVO struct {
	MessageID       string `json:"message_id"`
	TraceID         string `json:"trace_id,omitempty"`
	ConversationID  string `json:"conversation_id"`
	ParentMessageID string `json:"parent_message_id"`
	Query           string `json:"query"`
	Response        string `json:"response"`
	Model           string `json:"model"`
	CreatedAt       int64  `json:"created_at"`
}
type TraceVO struct {
	TraceID        string `json:"trace_id"`
	ConversationID string `json:"conversation_id"`
	MessageID      string `json:"message_id"`
	Status         string `json:"status"`
	StartTime      int64  `json:"start_time"`
	EndTime        int64  `json:"end_time"`
	Summary        any    `json:"summary"`
	Events         any    `json:"events,omitempty"`
}

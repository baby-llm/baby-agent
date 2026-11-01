package server

import (
	"babyagent/ch11/agent"
	"babyagent/ch11/observe"
	"babyagent/ch11/vo"
	"github.com/gin-gonic/gin"
	"net/http"
)

func NewRouter(s *Server, metrics *observe.Metrics) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), observe.Middleware(metrics))
	r.GET("/metrics", gin.WrapH(prometheusHandler()))
	api := r.Group("/api")
	api.POST("/conversation", s.createConversation)
	api.GET("/conversation", s.listConversations)
	api.GET("/conversation/:conversation_id/message", s.listMessages)
	api.POST("/conversation/:conversation_id/message", s.createMessage)
	api.GET("/trace/:trace_id", s.getTrace)
	return r
}
func (s *Server) createConversation(c *gin.Context) {
	var req vo.CreateConversationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, vo.Err(400, err.Error()))
		return
	}
	r, err := s.CreateConversation(req)
	if err != nil {
		c.JSON(500, vo.Err(500, err.Error()))
		return
	}
	c.JSON(200, vo.OK(r))
}
func (s *Server) listConversations(c *gin.Context) {
	r, err := s.ListConversations(c.Query("user_id"))
	if err != nil {
		c.JSON(500, vo.Err(500, err.Error()))
		return
	}
	c.JSON(200, vo.OK(r))
}
func (s *Server) listMessages(c *gin.Context) {
	r, err := s.ListMessages(c.Param("conversation_id"))
	if err != nil {
		c.JSON(500, vo.Err(500, err.Error()))
		return
	}
	c.JSON(200, vo.OK(r))
}
func (s *Server) getTrace(c *gin.Context) {
	r, err := s.GetTrace(c.Param("trace_id"))
	if err != nil {
		c.JSON(404, vo.Err(404, err.Error()))
		return
	}
	c.JSON(200, vo.OK(r))
}
func (s *Server) createMessage(c *gin.Context) {
	var req vo.CreateMessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, vo.Err(400, err.Error()))
		return
	}
	ch := make(chan vo.SSEMessageVO, 64)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	go func() {
		defer close(ch)
		if err := s.CreateMessage(c.Request.Context(), c.Param("conversation_id"), req, ch); err != nil {
			message := err.Error()
			ch <- vo.SSEMessageVO{Event: agent.EventError, Content: &message}
		}
	}()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			c.SSEvent("message", e)
			c.Writer.Flush()
		}
	}
}

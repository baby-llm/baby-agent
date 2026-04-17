package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"babyagent/ch12/agent"
	"babyagent/ch12/vo"
	"babyagent/shared/log"
)

func NewRouter(s *Server) *gin.Engine {
	g := gin.Default()

	api := g.Group("/api")
	api.POST("/conversation", s.createConversation)
	api.GET("/conversation", s.listConversations)
	api.PATCH("/conversation/:conversation_id", s.renameConversation)
	api.DELETE("/conversation/:conversation_id", s.deleteConversation)
	api.POST("/conversation/:conversation_id/message", s.createMessage)
	api.GET("/conversation/:conversation_id/message", s.listMessages)

	return g
}

// POST /conversation
func (s *Server) createConversation(c *gin.Context) {
	var req vo.CreateConversationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, vo.Err(400, err.Error()))
		return
	}

	result, err := s.CreateConversation(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, vo.Err(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, vo.OK(result))
}

// GET /conversation
func (s *Server) listConversations(c *gin.Context) {
	userID := c.Query("user_id")

	result, err := s.ListConversations(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, vo.Err(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, vo.OK(result))
}

// PATCH /conversation/:conversation_id
func (s *Server) renameConversation(c *gin.Context) {
	conversationID := c.Param("conversation_id")

	var req vo.UpdateConversationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, vo.Err(400, err.Error()))
		return
	}

	result, err := s.RenameConversation(conversationID, req.Title)
	if err != nil {
		c.JSON(http.StatusInternalServerError, vo.Err(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, vo.OK(result))
}

// DELETE /conversation/:conversation_id
func (s *Server) deleteConversation(c *gin.Context) {
	conversationID := c.Param("conversation_id")

	if err := s.DeleteConversation(conversationID); err != nil {
		c.JSON(http.StatusInternalServerError, vo.Err(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, vo.OK(map[string]any{"conversation_id": conversationID}))
}

// GET /conversation/:conversation_id/message
func (s *Server) listMessages(c *gin.Context) {
	conversationID := c.Param("conversation_id")

	result, err := s.ListMessages(conversationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, vo.Err(500, err.Error()))
		return
	}

	c.JSON(http.StatusOK, vo.OK(result))
}

// POST /conversation/:conversation_id/message
// 创建新消息并 SSE 流式输出 agent 响应
func (s *Server) createMessage(c *gin.Context) {
	conversationID := c.Param("conversation_id")

	var req vo.CreateMessageReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, vo.Err(400, err.Error()))
		return
	}

	eventCh := make(chan vo.SSEMessageVO, 64)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	go func() {
		defer close(eventCh)
		if err := s.CreateMessage(c.Request.Context(), conversationID, req, eventCh); err != nil {
			errMsg := err.Error()
			eventCh <- vo.SSEMessageVO{Event: agent.EventError, Content: &errMsg}
			return
		}
	}()

	for {
		select {
		case <-c.Request.Context().Done():
			log.Warn("Server is shutting down. Exiting...")
			return
		case e, ok := <-eventCh:
			if !ok {
				return
			}
			c.SSEvent("message", e)
			c.Writer.Flush()
		}
	}
}

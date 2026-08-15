package admin

import (
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
)

type quickReplyRequest struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type quickReplyReorderRequest struct {
	IDs []int64 `json:"ids"`
}

type quickReplyImportRequest struct {
	Items []quickReplyRequest `json:"items"`
}

func (h *ChatHandler) ListQuickReplies(c *gin.Context) {
	adminID, ok := chatAdminID(c)
	if !ok {
		return
	}
	items, err := h.chatService.ListQuickReplies(c.Request.Context(), adminID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *ChatHandler) CreateQuickReply(c *gin.Context) {
	adminID, ok := chatAdminID(c)
	if !ok {
		return
	}
	var req quickReplyRequest
	if !bindChatJSON(c, &req) {
		return
	}
	item, err := h.chatService.CreateQuickReply(c.Request.Context(), adminID, req.Title, req.Content)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *ChatHandler) UpdateQuickReply(c *gin.Context) {
	adminID, ok := chatAdminID(c)
	if !ok {
		return
	}
	id, ok := parsePositiveChatID(c, "id", "Invalid quick reply ID")
	if !ok {
		return
	}
	var req quickReplyRequest
	if !bindChatJSON(c, &req) {
		return
	}
	item, err := h.chatService.UpdateQuickReply(c.Request.Context(), adminID, id, req.Title, req.Content)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

func (h *ChatHandler) DeleteQuickReply(c *gin.Context) {
	adminID, ok := chatAdminID(c)
	if !ok {
		return
	}
	id, ok := parsePositiveChatID(c, "id", "Invalid quick reply ID")
	if !ok {
		return
	}
	if err := h.chatService.DeleteQuickReply(c.Request.Context(), adminID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

func (h *ChatHandler) ReorderQuickReplies(c *gin.Context) {
	adminID, ok := chatAdminID(c)
	if !ok {
		return
	}
	var req quickReplyReorderRequest
	if !bindChatJSON(c, &req) {
		return
	}
	if err := h.chatService.ReorderQuickReplies(c.Request.Context(), adminID, req.IDs); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

func (h *ChatHandler) ImportQuickReplies(c *gin.Context) {
	adminID, ok := chatAdminID(c)
	if !ok {
		return
	}
	var req quickReplyImportRequest
	if !bindChatJSON(c, &req) {
		return
	}
	items := make([]chat.QuickReply, 0, len(req.Items))
	for _, item := range req.Items {
		items = append(items, chat.QuickReply{Title: item.Title, Content: item.Content})
	}
	result, err := h.chatService.ImportQuickReplies(c.Request.Context(), adminID, items)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

func chatAdminID(c *gin.Context) (int64, bool) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return 0, false
	}
	return subject.UserID, true
}

func bindChatJSON(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxChatRequestBodyBytes)
	if err := c.ShouldBindJSON(target); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			response.RequestEntityTooLarge(c, "Request body too large")
		} else {
			response.BadRequest(c, "Invalid request")
		}
		return false
	}
	return true
}

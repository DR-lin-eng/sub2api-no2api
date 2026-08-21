package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler/supportchatasset"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
)

// UploadAsset stores a pending image for the authenticated user's conversation.
func (h *ChatHandler) UploadAsset(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	input, ok := supportchatasset.ParseUpload(c)
	if !ok {
		return
	}
	asset, err := h.chatService.UploadAssetForUser(c.Request.Context(), subject.UserID, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, asset)
}

// GetAsset serves an image only when it is linked to the caller's conversation.
func (h *ChatHandler) GetAsset(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	if name := c.Param("id"); supportchatasset.IsLegacyAssetName(name) {
		supportchatasset.WriteLegacyAsset(c, name)
		return
	}
	id, ok := supportchatasset.ParseID(c)
	if !ok {
		return
	}
	asset, err := h.chatService.GetAssetForUser(c.Request.Context(), id, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	supportchatasset.WriteAsset(c, asset)
}

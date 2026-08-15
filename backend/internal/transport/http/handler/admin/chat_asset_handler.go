package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler/supportchatasset"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
)

func (h *ChatHandler) UploadAsset(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	conversationID, ok := parsePositiveChatID(c, "id", "Invalid conversation ID")
	if !ok {
		return
	}
	input, ok := supportchatasset.ParseUpload(c)
	if !ok {
		return
	}
	asset, err := h.chatService.UploadAssetForAdmin(c.Request.Context(), conversationID, subject.UserID, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, asset)
}

func (h *ChatHandler) GetAsset(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	id, ok := supportchatasset.ParseID(c)
	if !ok {
		return
	}
	asset, err := h.chatService.GetAssetForAdmin(c.Request.Context(), id, subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	supportchatasset.WriteAsset(c, asset)
}

func (h *ChatHandler) ListLibrary(c *gin.Context) {
	h.listCatalog(c, chat.AssetScopeLibrary)
}

func (h *ChatHandler) CreateLibraryItem(c *gin.Context) {
	h.createCatalog(c, chat.AssetScopeLibrary, "category")
}

func (h *ChatHandler) DeleteLibraryItem(c *gin.Context) {
	h.hideCatalog(c, chat.AssetScopeLibrary)
}

func (h *ChatHandler) ListStickers(c *gin.Context) {
	h.listCatalog(c, chat.AssetScopeSticker)
}

func (h *ChatHandler) CreateSticker(c *gin.Context) {
	h.createCatalog(c, chat.AssetScopeSticker, "group")
}

func (h *ChatHandler) DeleteSticker(c *gin.Context) {
	h.hideCatalog(c, chat.AssetScopeSticker)
}

func (h *ChatHandler) listCatalog(c *gin.Context, scope chat.AssetScope) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	items, err := h.chatService.ListCatalogAssets(c.Request.Context(), scope, limit)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

func (h *ChatHandler) createCatalog(c *gin.Context, scope chat.AssetScope, collectionField string) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}
	input, ok := supportchatasset.ParseUpload(c)
	if !ok {
		return
	}
	if value := strings.TrimSpace(c.PostForm(collectionField)); value != "" {
		input.Collection = value
	}
	asset, err := h.chatService.CreateCatalogAsset(c.Request.Context(), subject.UserID, scope, input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, asset)
}

func (h *ChatHandler) hideCatalog(c *gin.Context, scope chat.AssetScope) {
	id, ok := supportchatasset.ParseID(c)
	if !ok {
		return
	}
	if err := h.chatService.HideCatalogAsset(c.Request.Context(), scope, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

func parsePositiveChatID(c *gin.Context, name, message string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, message)
		return 0, false
	}
	return id, true
}

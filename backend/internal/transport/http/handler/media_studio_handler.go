package handler

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"

	"github.com/gin-gonic/gin"
)

type MediaStudioHandler struct {
	mediaStudioService *service.MediaStudioService
}

func NewMediaStudioHandler(mediaStudioService *service.MediaStudioService) *MediaStudioHandler {
	return &MediaStudioHandler{mediaStudioService: mediaStudioService}
}

func (h *MediaStudioHandler) GetAdminGroupRoutes(c *gin.Context) {
	routes, err := h.mediaStudioService.GetGroupRoutes(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, routes)
}

func (h *MediaStudioHandler) UpdateAdminGroupRoutes(c *gin.Context) {
	var routes service.MediaStudioGroupRoutes
	if err := c.ShouldBindJSON(&routes); err != nil {
		response.BadRequest(c, "Invalid media studio group routes: "+err.Error())
		return
	}
	updated, err := h.mediaStudioService.SaveGroupRoutes(c.Request.Context(), routes)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, updated)
}

func (h *MediaStudioHandler) GetConfig(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	config, err := h.mediaStudioService.GetAvailableGroups(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, config)
}

func (h *MediaStudioHandler) ListModels(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	groupID, err := strconv.ParseInt(c.Query("group_id"), 10, 64)
	if err != nil || groupID <= 0 {
		response.BadRequest(c, "Invalid media studio group ID")
		return
	}
	models, err := h.mediaStudioService.ListModels(
		c.Request.Context(),
		subject.UserID,
		groupID,
		strings.TrimSpace(c.Query("media_type")),
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	data := make([]gin.H, 0, len(models))
	for _, model := range models {
		data = append(data, gin.H{
			"id":       model,
			"object":   "model",
			"owned_by": "media-studio",
		})
	}
	response.Success(c, gin.H{"object": "list", "data": data})
}

type mediaStudioSessionRequest struct {
	MediaType string `json:"media_type" binding:"required"`
	GroupID   int64  `json:"group_id" binding:"required"`
}

func (h *MediaStudioHandler) CreateSession(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req mediaStudioSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid media studio session request: "+err.Error())
		return
	}
	key, err := h.mediaStudioService.EnsureAPIKey(
		c.Request.Context(),
		subject.UserID,
		strings.TrimSpace(req.MediaType),
		req.GroupID,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{
		"api_key":    key.Key,
		"group_id":   req.GroupID,
		"media_type": strings.TrimSpace(req.MediaType),
	})
}

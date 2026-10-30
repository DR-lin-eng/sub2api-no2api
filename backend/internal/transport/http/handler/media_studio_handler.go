package handler

import (
	"errors"
	"io"
	"net/http"
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

const maxMediaStudioRequestBodyBytes = 256 << 10

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
	if !bindMediaStudioJSON(c, &routes, "Invalid media studio group routes") {
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
		entry := gin.H{
			"id":       model.ID,
			"object":   "model",
			"owned_by": "media-studio",
		}
		if len(model.Attributes) > 0 {
			entry["attributes"] = model.Attributes
		}
		if len(model.Limits) > 0 {
			entry["limits"] = model.Limits
		}
		data = append(data, entry)
	}
	response.Success(c, gin.H{"object": "list", "data": data})
}

type mediaStudioSessionRequest struct {
	MediaType string `json:"media_type" binding:"required"`
	GroupID   int64  `json:"group_id" binding:"required"`
}

func (h *MediaStudioHandler) CreateSession(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req mediaStudioSessionRequest
	if !bindMediaStudioJSON(c, &req, "Invalid media studio session request") {
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

// EstimateCost 返回媒体工坊提交前的预估消耗，金额来自 sub2api 本地计价链路。
func (h *MediaStudioHandler) EstimateCost(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.Header("Pragma", "no-cache")
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	var req service.MediaStudioCostEstimateRequest
	if !bindMediaStudioJSON(c, &req, "Invalid media studio estimate request") {
		return
	}
	estimate, err := h.mediaStudioService.EstimateCost(c.Request.Context(), subject.UserID, req)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, estimate)
}

func (h *MediaStudioHandler) UploadFile(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	c.Request.Body = http.MaxBytesReader(
		c.Writer,
		c.Request.Body,
		int64(service.MaxMediaStudioUploadBytes)+(1<<20),
	)
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			response.RequestEntityTooLarge(c, "Request body too large")
			return
		}
		response.BadRequest(c, "Invalid media studio upload request")
		return
	}
	if c.Request.MultipartForm != nil {
		defer func() { _ = c.Request.MultipartForm.RemoveAll() }()
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.BadRequest(c, "Missing upload file")
		return
	}
	if fileHeader.Size > int64(service.MaxMediaStudioUploadBytes) {
		response.RequestEntityTooLarge(c, "Request body too large")
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		response.BadRequest(c, "Invalid upload file")
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, int64(service.MaxMediaStudioUploadBytes)+1))
	if err != nil {
		response.BadRequest(c, "Invalid upload file")
		return
	}
	if len(data) > service.MaxMediaStudioUploadBytes {
		response.RequestEntityTooLarge(c, "Request body too large")
		return
	}

	upload, err := h.mediaStudioService.UploadFile(
		c.Request.Context(),
		subject.UserID,
		c.PostForm("model"),
		c.PostForm("kind"),
		fileHeader.Filename,
		fileHeader.Header.Get("Content-Type"),
		data,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, upload)
}

func bindMediaStudioJSON(c *gin.Context, target any, invalidMessage string) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxMediaStudioRequestBodyBytes)
	if err := c.ShouldBindJSON(target); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			response.RequestEntityTooLarge(c, "Request body too large")
		} else {
			response.BadRequest(c, invalidMessage)
		}
		return false
	}
	return true
}

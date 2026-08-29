package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler/dto"
	"github.com/gin-gonic/gin"
)

const maxCustomModelConfigBodyBytes = 256 << 10

type CustomModelConfigHandler struct {
	service *service.CustomModelConfigService
}

func NewCustomModelConfigHandler(configService *service.CustomModelConfigService) *CustomModelConfigHandler {
	return &CustomModelConfigHandler{service: configService}
}

func (h *CustomModelConfigHandler) List(c *gin.Context) {
	var (
		items []service.CustomModelConfig
		err   error
	)
	if c.Query("runtime") == "1" {
		items, err = h.service.ListEnabled(c.Request.Context())
	} else {
		items, err = h.service.List(c.Request.Context())
	}
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	result := make([]*dto.CustomModelConfig, 0, len(items))
	for index := range items {
		result = append(result, dto.CustomModelConfigFromService(&items[index]))
	}
	response.Success(c, result)
}

func (h *CustomModelConfigHandler) Create(c *gin.Context) {
	var req dto.CreateCustomModelConfigRequest
	if !bindCustomModelJSON(c, &req) {
		return
	}
	item, err := h.service.Create(c.Request.Context(), service.CreateCustomModelConfigInput{
		ModelName:    req.ModelName,
		PrefixMatch:  req.PrefixMatch,
		Capabilities: req.Capabilities,
		VideoAPIType: req.VideoAPIType,
		TemplateID:   req.TemplateID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.CustomModelConfigFromService(item))
}

func (h *CustomModelConfigHandler) Update(c *gin.Context) {
	id, ok := parseCustomModelID(c, "id", "Invalid config ID")
	if !ok {
		return
	}
	var req dto.UpdateCustomModelConfigRequest
	if !bindCustomModelJSON(c, &req) {
		return
	}
	templateID, templateIDSet, err := parseOptionalCustomModelID(req.TemplateID)
	if err != nil {
		response.BadRequest(c, "template_id must be a positive integer or null")
		return
	}
	videoAPIType, _, err := parseOptionalString(req.VideoAPIType)
	if err != nil {
		response.BadRequest(c, "video_api_type must be a string or null")
		return
	}
	item, err := h.service.Update(c.Request.Context(), id, service.UpdateCustomModelConfigInput{
		PrefixMatch:   req.PrefixMatch,
		Capabilities:  req.Capabilities,
		VideoAPIType:  videoAPIType,
		TemplateID:    templateID,
		TemplateIDSet: templateIDSet,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.CustomModelConfigFromService(item))
}

func (h *CustomModelConfigHandler) Delete(c *gin.Context) {
	id, ok := parseCustomModelID(c, "id", "Invalid config ID")
	if !ok {
		return
	}
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

func (h *CustomModelConfigHandler) Get(c *gin.Context) {
	id, ok := parseCustomModelID(c, "id", "Invalid config ID")
	if !ok {
		return
	}
	item, err := h.service.Get(c.Request.Context(), id)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.CustomModelConfigFromService(item))
}

type customModelRequestTemplateRequest struct {
	Name           string         `json:"name" binding:"required,max=100"`
	Description    string         `json:"description" binding:"max=500"`
	RequestAdapter map[string]any `json:"request_adapter"`
}

type customModelRequestTemplateUpdateRequest struct {
	Name           *string         `json:"name" binding:"omitempty,max=100"`
	Description    *string         `json:"description" binding:"omitempty,max=500"`
	RequestAdapter *map[string]any `json:"request_adapter"`
}

func (h *CustomModelConfigHandler) ListTemplates(c *gin.Context) {
	items, err := h.service.ListTemplates(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	result := make([]*dto.CustomModelRequestTemplate, 0, len(items))
	for index := range items {
		result = append(result, dto.CustomModelRequestTemplateFromService(&items[index]))
	}
	response.Success(c, result)
}

func (h *CustomModelConfigHandler) CreateTemplate(c *gin.Context) {
	var req customModelRequestTemplateRequest
	if !bindCustomModelJSON(c, &req) {
		return
	}
	item, err := h.service.CreateTemplate(c.Request.Context(), service.CreateCustomModelRequestTemplateInput{
		Name:           req.Name,
		Description:    req.Description,
		RequestAdapter: req.RequestAdapter,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.CustomModelRequestTemplateFromService(item))
}

func (h *CustomModelConfigHandler) UpdateTemplate(c *gin.Context) {
	id, ok := parseCustomModelID(c, "templateId", "Invalid template ID")
	if !ok {
		return
	}
	var req customModelRequestTemplateUpdateRequest
	if !bindCustomModelJSON(c, &req) {
		return
	}
	item, err := h.service.UpdateTemplate(c.Request.Context(), id, service.UpdateCustomModelRequestTemplateInput{
		Name:           req.Name,
		Description:    req.Description,
		RequestAdapter: req.RequestAdapter,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.CustomModelRequestTemplateFromService(item))
}

func (h *CustomModelConfigHandler) DeleteTemplate(c *gin.Context) {
	id, ok := parseCustomModelID(c, "templateId", "Invalid template ID")
	if !ok {
		return
	}
	if err := h.service.DeleteTemplate(c.Request.Context(), id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

func bindCustomModelJSON(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxCustomModelConfigBodyBytes)
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

func parseCustomModelID(c *gin.Context, param, message string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(param), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, message)
		return 0, false
	}
	return id, true
}

func parseOptionalCustomModelID(raw json.RawMessage) (*int64, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, false, nil
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return nil, true, nil
	}
	var value int64
	if err := json.Unmarshal(trimmed, &value); err != nil || value <= 0 {
		return nil, true, errors.New("invalid positive ID")
	}
	return &value, true, nil
}

func parseOptionalString(raw json.RawMessage) (*string, bool, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, false, nil
	}
	if bytes.Equal(trimmed, []byte("null")) {
		value := ""
		return &value, true, nil
	}
	var value string
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return nil, true, err
	}
	return &value, true, nil
}

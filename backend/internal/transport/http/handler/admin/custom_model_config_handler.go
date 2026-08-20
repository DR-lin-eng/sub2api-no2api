package admin

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/custommodelconfig"
	"github.com/Wei-Shaw/sub2api/ent/custommodelrequesttemplate"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler/dto"

	"github.com/gin-gonic/gin"
)

// CustomModelConfigHandler handles custom model configuration operations
type CustomModelConfigHandler struct {
	client *ent.Client
}

// NewCustomModelConfigHandler creates a new custom model config handler
func NewCustomModelConfigHandler(client *ent.Client) *CustomModelConfigHandler {
	return &CustomModelConfigHandler{
		client: client,
	}
}

// List handles listing all custom model configs
// GET /api/v1/admin/custom-model-configs
func (h *CustomModelConfigHandler) List(c *gin.Context) {
	configs, err := h.client.CustomModelConfig.Query().All(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	templateNames := make(map[int64]string)
	templates, templateErr := h.client.CustomModelRequestTemplate.Query().All(c.Request.Context())
	if templateErr != nil {
		response.ErrorFrom(c, templateErr)
		return
	}
	for _, template := range templates {
		templateNames[int64(template.ID)] = template.Name
	}

	out := make([]*dto.CustomModelConfig, 0, len(configs))
	for _, config := range configs {
		templateName := ""
		if config.TemplateID != nil {
			templateName = templateNames[*config.TemplateID]
		}
		out = append(out, dto.CustomModelConfigFromEnt(config, templateName))
	}
	response.Success(c, out)
}

// Create handles creating a new custom model config
// POST /api/v1/admin/custom-model-configs
func (h *CustomModelConfigHandler) Create(c *gin.Context) {
	var req dto.CreateCustomModelConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	// Validate model name is not empty
	modelName := strings.TrimSpace(req.ModelName)
	if modelName == "" {
		response.BadRequest(c, "model_name is required")
		return
	}

	// Check if model already exists
	exists, err := h.client.CustomModelConfig.Query().
		Where(custommodelconfig.ModelNameEqualFold(modelName)).
		Exist(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if exists {
		response.BadRequest(c, "Model config already exists")
		return
	}

	capabilities := req.Capabilities
	if capabilities == nil {
		capabilities = []string{}
	}

	if req.TemplateID != nil {
		if _, err := h.client.CustomModelRequestTemplate.Get(c.Request.Context(), *req.TemplateID); err != nil {
			if ent.IsNotFound(err) {
				response.BadRequest(c, "template_id not found")
			} else {
				response.ErrorFrom(c, err)
			}
			return
		}
	}

	create := h.client.CustomModelConfig.Create().
		SetModelName(modelName).
		SetPrefixMatch(req.PrefixMatch).
		SetCapabilities(capabilities)
	if req.TemplateID != nil {
		create.SetTemplateID(*req.TemplateID)
	}
	if req.VideoAPIType != "" {
		create.SetVideoAPIType(req.VideoAPIType)
	}
	config, err := create.Save(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.CustomModelConfigFromEnt(config, templateNameForConfig(c, h.client, config)))
}

// Update handles updating an existing custom model config
// PUT /api/v1/admin/custom-model-configs/:id
func (h *CustomModelConfigHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid config ID")
		return
	}

	var req dto.UpdateCustomModelConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	capabilities := req.Capabilities
	if capabilities == nil {
		capabilities = []string{}
	}

	update := h.client.CustomModelConfig.UpdateOneID(id).
		SetCapabilities(capabilities)
	if req.PrefixMatch != nil {
		update.SetPrefixMatch(*req.PrefixMatch)
	}
	if len(req.TemplateID) > 0 {
		if bytes.Equal(bytes.TrimSpace(req.TemplateID), []byte("null")) {
			update.ClearTemplateID()
		} else {
			var templateID int64
			if err := json.Unmarshal(req.TemplateID, &templateID); err != nil || templateID <= 0 {
				response.BadRequest(c, "template_id must be a positive integer or null")
				return
			}
			if _, err := h.client.CustomModelRequestTemplate.Get(c.Request.Context(), templateID); err != nil {
				if ent.IsNotFound(err) {
					response.BadRequest(c, "template_id not found")
				} else {
					response.ErrorFrom(c, err)
				}
				return
			}
			update.SetTemplateID(templateID)
		}
	}

	if req.VideoAPIType != "" {
		update.SetVideoAPIType(req.VideoAPIType)
	}

	config, err := update.Save(c.Request.Context())
	if err != nil {
		if ent.IsNotFound(err) {
			response.NotFound(c, "Config not found")
		} else {
			response.ErrorFrom(c, err)
		}
		return
	}

	response.Success(c, dto.CustomModelConfigFromEnt(config, templateNameForConfig(c, h.client, config)))
}

// Delete handles deleting a custom model config
// DELETE /api/v1/admin/custom-model-configs/:id
func (h *CustomModelConfigHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid config ID")
		return
	}

	err = h.client.CustomModelConfig.DeleteOneID(id).Exec(c.Request.Context())
	if err != nil {
		if ent.IsNotFound(err) {
			response.NotFound(c, "Config not found")
		} else {
			response.ErrorFrom(c, err)
		}
		return
	}

	response.Success(c, gin.H{"message": "ok"})
}

// Get handles getting a custom model config by ID
// GET /api/v1/admin/custom-model-configs/:id
func (h *CustomModelConfigHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid config ID")
		return
	}

	config, err := h.client.CustomModelConfig.Get(c.Request.Context(), id)
	if err != nil {
		if ent.IsNotFound(err) {
			response.NotFound(c, "Config not found")
		} else {
			response.ErrorFrom(c, err)
		}
		return
	}

	response.Success(c, dto.CustomModelConfigFromEnt(config, templateNameForConfig(c, h.client, config)))
}

func templateNameForConfig(c *gin.Context, client *ent.Client, config *ent.CustomModelConfig) string {
	if config == nil || config.TemplateID == nil {
		return ""
	}
	template, err := client.CustomModelRequestTemplate.Get(c.Request.Context(), *config.TemplateID)
	if err != nil {
		return ""
	}
	return template.Name
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
	items, err := h.client.CustomModelRequestTemplate.Query().Order(ent.Asc(custommodelrequesttemplate.FieldName)).All(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	out := make([]*dto.CustomModelRequestTemplate, 0, len(items))
	for _, item := range items {
		out = append(out, dto.CustomModelRequestTemplateFromEnt(item))
	}
	response.Success(c, out)
}

func (h *CustomModelConfigHandler) CreateTemplate(c *gin.Context) {
	var req customModelRequestTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		response.BadRequest(c, "name is required")
		return
	}
	adapter := req.RequestAdapter
	if adapter == nil {
		adapter = map[string]any{}
	}
	item, err := h.client.CustomModelRequestTemplate.Create().
		SetName(name).
		SetDescription(strings.TrimSpace(req.Description)).
		SetRequestAdapter(adapter).
		Save(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, dto.CustomModelRequestTemplateFromEnt(item))
}

func (h *CustomModelConfigHandler) UpdateTemplate(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("templateId"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid template ID")
		return
	}
	var req customModelRequestTemplateUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	update := h.client.CustomModelRequestTemplate.UpdateOneID(id)
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			response.BadRequest(c, "name is required")
			return
		}
		update.SetName(name)
	}
	if req.Description != nil {
		update.SetDescription(strings.TrimSpace(*req.Description))
	}
	if req.RequestAdapter != nil {
		update.SetRequestAdapter(*req.RequestAdapter)
	}
	item, err := update.Save(c.Request.Context())
	if err != nil {
		if ent.IsNotFound(err) {
			response.NotFound(c, "Template not found")
		} else {
			response.ErrorFrom(c, err)
		}
		return
	}
	response.Success(c, dto.CustomModelRequestTemplateFromEnt(item))
}

func (h *CustomModelConfigHandler) DeleteTemplate(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("templateId"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid template ID")
		return
	}
	if err := h.client.CustomModelRequestTemplate.DeleteOneID(id).Exec(c.Request.Context()); err != nil {
		if ent.IsNotFound(err) {
			response.NotFound(c, "Template not found")
		} else {
			response.ErrorFrom(c, err)
		}
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

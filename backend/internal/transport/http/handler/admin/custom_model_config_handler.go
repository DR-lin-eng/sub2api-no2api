package admin

import (
	"strconv"

	"github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/custommodelconfig"
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

	out := make([]*dto.CustomModelConfig, 0, len(configs))
	for _, config := range configs {
		out = append(out, dto.CustomModelConfigFromEnt(config))
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
	if req.ModelName == "" {
		response.BadRequest(c, "model_name is required")
		return
	}

	// Check if model already exists
	exists, err := h.client.CustomModelConfig.Query().
		Where(custommodelconfig.ModelNameEQ(req.ModelName)).
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

	config, err := h.client.CustomModelConfig.Create().
		SetModelName(req.ModelName).
		SetCapabilities(capabilities).
		Save(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	response.Success(c, dto.CustomModelConfigFromEnt(config))
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

	config, err := h.client.CustomModelConfig.UpdateOneID(id).
		SetCapabilities(capabilities).
		Save(c.Request.Context())
	if err != nil {
		if ent.IsNotFound(err) {
			response.NotFound(c, "Config not found")
		} else {
			response.ErrorFrom(c, err)
		}
		return
	}

	response.Success(c, dto.CustomModelConfigFromEnt(config))
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

	response.Success(c, dto.CustomModelConfigFromEnt(config))
}

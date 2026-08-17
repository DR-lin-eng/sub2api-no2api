package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/gin-gonic/gin"
)

// GetCloudflareIngressSettings returns the persisted edge integration without
// exposing the API token or its encrypted representation.
func (h *OpsHandler) GetCloudflareIngressSettings(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	settings, err := h.opsService.GetCloudflareIngressSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

// UpdateCloudflareIngressSettings persists and hot-applies edge integration settings.
func (h *OpsHandler) UpdateCloudflareIngressSettings(c *gin.Context) {
	if h.opsService == nil {
		response.Error(c, http.StatusServiceUnavailable, "Ops service not available")
		return
	}
	var input service.UpdateCloudflareIngressSettingsInput
	if err := c.ShouldBindJSON(&input); err != nil {
		response.BadRequest(c, "Invalid request body")
		return
	}
	settings, err := h.opsService.UpdateCloudflareIngressSettings(c.Request.Context(), input)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, settings)
}

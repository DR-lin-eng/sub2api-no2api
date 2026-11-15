package admin

import (
	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/response"
	"github.com/DR-lin-eng/sub2api-no2api/internal/transport/http/handler/dto"

	"github.com/gin-gonic/gin"
)

// GetPanelRateLimitSettings returns the persisted panel API protection policy.
// GET /api/v1/admin/settings/panel-rate-limit
func (h *SettingHandler) GetPanelRateLimitSettings(c *gin.Context) {
	settings, err := h.settingService.GetPanelRateLimitSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, panelRateLimitSettingsDTO(settings))
}

type UpdatePanelRateLimitSettingsRequest struct {
	Enabled     *bool `json:"enabled"`
	UserRPM     *int  `json:"user_rpm"`
	HeavyRPM    *int  `json:"heavy_rpm"`
	ExemptAdmin *bool `json:"exempt_admin"`
	PublicIPRPM *int  `json:"public_ip_rpm"`
}

// UpdatePanelRateLimitSettings updates the panel API protection policy.
// PUT /api/v1/admin/settings/panel-rate-limit
func (h *SettingHandler) UpdatePanelRateLimitSettings(c *gin.Context) {
	var req UpdatePanelRateLimitSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	if req.Enabled == nil || req.UserRPM == nil || req.HeavyRPM == nil || req.ExemptAdmin == nil || req.PublicIPRPM == nil {
		response.BadRequest(c, "enabled, user_rpm, heavy_rpm, exempt_admin, and public_ip_rpm are required")
		return
	}
	settings := &service.PanelRateLimitSettings{
		Enabled:     *req.Enabled,
		UserRPM:     *req.UserRPM,
		HeavyRPM:    *req.HeavyRPM,
		ExemptAdmin: *req.ExemptAdmin,
		PublicIPRPM: *req.PublicIPRPM,
	}
	if err := h.settingService.SetPanelRateLimitSettings(c.Request.Context(), settings); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, panelRateLimitSettingsDTO(settings))
}

func panelRateLimitSettingsDTO(settings *service.PanelRateLimitSettings) dto.PanelRateLimitSettings {
	return dto.PanelRateLimitSettings{
		Enabled:     settings.Enabled,
		UserRPM:     settings.UserRPM,
		HeavyRPM:    settings.HeavyRPM,
		ExemptAdmin: settings.ExemptAdmin,
		PublicIPRPM: settings.PublicIPRPM,
	}
}

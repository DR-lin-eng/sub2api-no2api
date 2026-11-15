package admin

import (
	"encoding/json"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/response"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

// UpdateSettings updates the complete persisted settings document while
// preserving fields omitted by older or partial clients.
// PUT /api/v1/admin/settings
func (h *SettingHandler) UpdateSettings(c *gin.Context) {
	var sentFields map[string]json.RawMessage
	if err := c.ShouldBindBodyWith(&sentFields, binding.JSON); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	var req UpdateSettingsRequest
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}
	submittedReq := req
	omitted := omittedSettingKeys(sentFields)

	previousSettings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	mergeOmittedUpdateSettingsRequest(&req, previousSettings, sentFields)

	previousAuthSourceDefaults, err := h.settingService.GetAuthSourceDefaultSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	prepared, ok := h.prepareSettingsUpdate(c, &req, previousSettings, previousAuthSourceDefaults)
	if !ok {
		return
	}
	if !h.persistSettingsUpdate(c, prepared, omitted) {
		return
	}
	h.writeSettingsUpdateResponse(c, previousSettings, previousAuthSourceDefaults, submittedReq)
}

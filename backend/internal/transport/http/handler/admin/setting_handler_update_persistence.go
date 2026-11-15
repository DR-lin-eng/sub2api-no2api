package admin

import (
	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/response"

	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) persistSettingsUpdate(
	c *gin.Context,
	prepared *preparedSettingsUpdate,
	omitted service.OmittedSettingKeys,
) bool {
	settings := buildSystemSettingsUpdate(prepared)
	authSourceDefaults := buildAuthSourceDefaultSettingsUpdate(prepared)
	if err := h.settingService.UpdateSettingsWithAuthSourceDefaultsOmitting(
		c.Request.Context(),
		settings,
		authSourceDefaults,
		omitted,
	); err != nil {
		response.ErrorFrom(c, err)
		return false
	}

	req := prepared.request
	// OpenAI fast policy uses a dedicated setting key and retains omission semantics.
	if req.OpenAIFastPolicySettings != nil {
		if err := h.settingService.SetOpenAIFastPolicySettings(
			c.Request.Context(),
			openaiFastPolicySettingsFromDTO(req.OpenAIFastPolicySettings),
		); err != nil {
			response.BadRequest(c, err.Error())
			return false
		}
	}

	// Payment settings are owned by PaymentConfigService. Omission must not
	// replace that separate document with zero values.
	if h.paymentConfigService != nil && hasPaymentFields(*req) {
		if err := h.paymentConfigService.UpdatePaymentConfig(
			c.Request.Context(),
			buildPaymentConfigUpdate(req),
		); err != nil {
			response.ErrorFrom(c, err)
			return false
		}
		if h.paymentService != nil {
			h.paymentService.RefreshProviders(c.Request.Context())
		}
	}

	return true
}

package admin

import (
	"context"
	"log/slog"
	"strings"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/response"

	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) writeSettingsUpdateResponse(
	c *gin.Context,
	previousSettings *service.SystemSettings,
	previousAuthSourceDefaults *service.AuthSourceDefaultSettings,
	submittedReq UpdateSettingsRequest,
) {
	updatedSettings, err := h.settingService.GetAllSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	if h.opsService != nil {
		h.opsService.SetMonitoringEnabled(updatedSettings.OpsMonitoringEnabled)
	}
	h.ensureDingTalkSyncAttributes(c.Request.Context(), updatedSettings)
	updatedAuthSourceDefaults, err := h.settingService.GetAuthSourceDefaultSettings(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	h.auditSettingsUpdate(c, previousSettings, updatedSettings, previousAuthSourceDefaults, updatedAuthSourceDefaults, submittedReq)
	// Reload payment config for response
	var updatedPaymentCfg *service.PaymentConfig
	if h.paymentConfigService != nil {
		updatedPaymentCfg, _ = h.paymentConfigService.GetPaymentConfig(c.Request.Context())
	}
	if updatedPaymentCfg == nil {
		updatedPaymentCfg = &service.PaymentConfig{}
	}
	passkeyConfigured, passkeyRPID, passkeyRPOrigins := h.settingService.PasskeyConfiguration()
	payload := buildSettingsUpdateResponsePayload(
		updatedSettings,
		updatedPaymentCfg,
		h.settingService.IsTotpEncryptionKeyConfigured(),
		passkeyConfigured,
		passkeyRPID,
		passkeyRPOrigins,
	)
	if fastPolicy, err := h.settingService.GetOpenAIFastPolicySettings(c.Request.Context()); err != nil {
		slog.Error("openai_fast_policy_settings_get_failed", "error", err)
	} else if fastPolicy != nil {
		payload.OpenAIFastPolicySettings = openaiFastPolicySettingsToDTO(fastPolicy)
	}

	// Default platform quotas（JSON map）—— 与 GetSettings 一致，避免保存后响应缺失该字段
	if platformQuotas, err := h.settingService.GetDefaultPlatformQuotas(c.Request.Context()); err != nil {
		slog.Error("default_platform_quotas_get_failed", "error", err)
	} else {
		payload.DefaultPlatformQuotas = platformQuotas
	}
	response.Success(c, systemSettingsResponseData(payload, updatedAuthSourceDefaults))
}

// ensureDingTalkSyncAttributes 在保存 settings 后，按 admin 配置的 (attr key, attr name)
// 兜底 upsert 对应 user attribute definition：不存在则创建；存在但 name 不同则更新 name
// （type/options/required 不变）。仅 internal_only + 对应 sync 开关开启时执行。
// 失败仅记录日志，不阻塞 settings 保存。
func (h *SettingHandler) ensureDingTalkSyncAttributes(ctx context.Context, settings *service.SystemSettings) {
	if h.userAttributeService == nil || settings == nil {
		return
	}
	if settings.DingTalkConnectCorpRestrictionPolicy != "internal_only" {
		return
	}
	if settings.DingTalkConnectSyncDisplayName {
		h.ensureUserAttributeDefinition(ctx, settings.DingTalkConnectSyncDisplayNameAttrKey, settings.DingTalkConnectSyncDisplayNameAttrName, "钉钉 internal_only 登录时同步的钉钉姓名", service.AttributeTypeText)
	}
	if settings.DingTalkConnectSyncCorpEmail {
		h.ensureUserAttributeDefinition(ctx, settings.DingTalkConnectSyncCorpEmailAttrKey, settings.DingTalkConnectSyncCorpEmailAttrName, "钉钉 internal_only 登录时同步的企业邮箱", service.AttributeTypeEmail)
	}
	if settings.DingTalkConnectSyncDept {
		h.ensureUserAttributeDefinition(ctx, settings.DingTalkConnectSyncDeptAttrKey, settings.DingTalkConnectSyncDeptAttrName, "钉钉 internal_only 登录时同步的完整部门路径（如：公司/研发部）", service.AttributeTypeText)
	}
}

func (h *SettingHandler) ensureUserAttributeDefinition(ctx context.Context, key, name, description string, attrType service.UserAttributeType) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	existing, err := h.userAttributeService.GetDefinitionByKey(ctx, key)
	if err == nil && existing != nil {
		if strings.TrimSpace(name) != "" && existing.Name != name {
			if _, err := h.userAttributeService.UpdateDefinition(ctx, existing.ID, service.UpdateAttributeDefinitionInput{
				Name: &name,
			}); err != nil {
				slog.Warn("dingtalk: update user attribute definition name failed", "key", key, "err", err.Error())
				return
			}
			slog.Info("dingtalk: updated user attribute definition name", "key", key, "name", name)
		}
		return
	}
	if _, err := h.userAttributeService.CreateDefinition(ctx, service.CreateAttributeDefinitionInput{
		Key:         key,
		Name:        name,
		Description: description,
		Type:        attrType,
		Enabled:     true,
	}); err != nil {
		slog.Warn("dingtalk: ensure user attribute definition failed", "key", key, "err", err.Error())
		return
	}
	slog.Info("dingtalk: created user attribute definition", "key", key, "name", name, "type", attrType)
}

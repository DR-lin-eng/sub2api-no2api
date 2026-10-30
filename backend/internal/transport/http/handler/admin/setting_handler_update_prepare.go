package admin

import (
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"

	"github.com/gin-gonic/gin"
)

type preparedSettingsUpdate struct {
	request                    *UpdateSettingsRequest
	previousSettings           *service.SystemSettings
	previousAuthSourceDefaults *service.AuthSourceDefaultSettings

	passkeyEnabled          bool
	sessionBindingEnabled   bool
	stepUpEnabled           bool
	localCaptchaEnabled     bool
	recaptchaEnabled        bool
	capEnabled              bool
	tencentCaptchaEnabled   bool
	aliyunCaptchaEnabled    bool
	clientIPResolutionMode  string
	clientIPTrustedProxies  []string
	affiliateRebateRate     float64
	affiliateFreezeHours    int
	affiliateDurationDays   int
	affiliatePerInviteeCap  float64
	adminRechargeRebate     bool
	loginAgreementMode      string
	loginAgreementUpdatedAt string
	loginAgreementDocuments []service.LoginAgreementDocument
	oidcUsePKCE             bool
	oidcValidateIDToken     bool
	purchaseEnabled         bool
	purchaseURL             string
	customMenuJSON          string
	customEndpointsJSON     string
	defaultSubscriptions    []service.DefaultSubscriptionSetting
}

func (h *SettingHandler) prepareSettingsUpdate(
	c *gin.Context,
	req *UpdateSettingsRequest,
	previousSettings *service.SystemSettings,
	previousAuthSourceDefaults *service.AuthSourceDefaultSettings,
) (*preparedSettingsUpdate, bool) {
	prepared := &preparedSettingsUpdate{
		request:                    req,
		previousSettings:           previousSettings,
		previousAuthSourceDefaults: previousAuthSourceDefaults,
	}
	if !h.resolveSettingsUpdateSecurity(c, prepared) {
		return nil, false
	}
	if !h.validateCoreSettingsUpdate(c, prepared) {
		return nil, false
	}
	if !h.validateIdentitySettingsUpdate(c, prepared) {
		return nil, false
	}
	if !validatePresentationSettingsUpdate(c, prepared) {
		return nil, false
	}
	if !validateRuntimeSettingsUpdate(c, prepared) {
		return nil, false
	}
	return prepared, true
}

// ensureActorTotpForStepUp requires an interactive admin session whose actor
// already has TOTP enabled before the global step-up gate can be enabled.
func (h *SettingHandler) ensureActorTotpForStepUp(c *gin.Context) bool {
	if reason, machine := middleware.MachineCredentialReason(c); machine {
		response.ErrorWithDetails(c, http.StatusForbidden,
			"Admin API key cannot enable step-up verification; use an admin session with TOTP enabled",
			reason, nil)
		return false
	}
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok || subject.UserID <= 0 {
		response.ErrorWithDetails(c, http.StatusForbidden,
			"Enabling step-up verification requires an authenticated admin session",
			"STEP_UP_ENABLE_REQUIRES_TOTP", nil)
		return false
	}
	if h.userService == nil {
		response.InternalError(c, "Step-up precondition check unavailable")
		return false
	}
	user, err := h.userService.GetByID(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return false
	}
	if !user.TotpEnabled {
		response.ErrorWithDetails(c, http.StatusBadRequest,
			"Enable two-factor authentication (TOTP) for your account before turning on step-up verification",
			"STEP_UP_ENABLE_REQUIRES_TOTP", nil)
		return false
	}
	return true
}

func (h *SettingHandler) resolveSettingsUpdateSecurity(c *gin.Context, prepared *preparedSettingsUpdate) bool {
	req := prepared.request
	previousSettings := prepared.previousSettings

	// 新增安全开关的请求字段为指针：省略字段=保持现值，避免旧客户端/脚本
	// 用不含新字段的全量 payload 保存设置时把安全开关静默重置。
	sessionBindingEnabled := previousSettings.SessionBindingEnabled
	if req.SessionBindingEnabled != nil {
		sessionBindingEnabled = *req.SessionBindingEnabled
	}
	stepUpEnabled := previousSettings.StepUpEnabled
	if req.StepUpEnabled != nil {
		stepUpEnabled = *req.StepUpEnabled
	}
	passkeyEnabled := previousSettings.PasskeyEnabled
	if req.PasskeyEnabled != nil {
		passkeyEnabled = *req.PasskeyEnabled
	}
	if passkeyEnabled {
		configured, _, _ := h.settingService.PasskeyConfiguration()
		if !configured {
			response.BadRequest(c, "Passkey sign-in requires a valid WebAuthn RP ID and allowed HTTPS origins in the deployment configuration")
			return false
		}
	}
	localCaptchaEnabled := previousSettings.LocalCaptchaEnabled
	if req.LocalCaptchaEnabled != nil {
		localCaptchaEnabled = *req.LocalCaptchaEnabled
	}
	recaptchaEnabled := previousSettings.RecaptchaEnabled
	if req.RecaptchaEnabled != nil {
		recaptchaEnabled = *req.RecaptchaEnabled
	}
	capEnabled := previousSettings.CapEnabled
	if req.CapEnabled != nil {
		capEnabled = *req.CapEnabled
	}
	tencentCaptchaEnabled := previousSettings.TencentCaptchaEnabled
	if req.TencentCaptchaEnabled != nil {
		tencentCaptchaEnabled = *req.TencentCaptchaEnabled
	}
	aliyunCaptchaEnabled := previousSettings.AliyunCaptchaEnabled
	if req.AliyunCaptchaEnabled != nil {
		aliyunCaptchaEnabled = *req.AliyunCaptchaEnabled
	}
	clientIPResolutionMode := previousSettings.ClientIPResolutionMode
	if req.ClientIPResolutionMode != nil {
		clientIPResolutionMode = *req.ClientIPResolutionMode
	}
	clientIPTrustedProxies := append([]string(nil), previousSettings.ClientIPTrustedProxies...)
	if req.ClientIPTrustedProxies != nil {
		clientIPTrustedProxies = append([]string(nil), (*req.ClientIPTrustedProxies)...)
	}
	providerCount := 0
	for _, enabled := range []bool{req.TurnstileEnabled, recaptchaEnabled, capEnabled, tencentCaptchaEnabled, aliyunCaptchaEnabled, localCaptchaEnabled} {
		if enabled {
			providerCount++
		}
	}
	if providerCount > 1 {
		response.BadRequest(c, "Only one human verification provider can be enabled")
		return false
	}

	// 开启敏感操作 step-up 门控属自锁风险操作：仅允许本人已启用 TOTP 的管理员会话开启，
	// 否则开启后操作者立即被挡在所有敏感操作之外。仅在 false→true 的开启瞬间校验，
	// 保持开启状态的常规设置保存不受影响。
	if stepUpEnabled && !previousSettings.StepUpEnabled {
		if !h.ensureActorTotpForStepUp(c) {
			return false
		}
	}
	// 关闭 step-up 门控本身就是敏感操作：防止拿到管理员会话的攻击者先关闸再执行导出/备份。
	// previousSettings 已证实开关处于开启状态，使用无条件门控变体，
	// 避免门控内部二次读取开关时因存储故障 fail-open（前端捕获 STEP_UP_REQUIRED 弹码重试）。
	if !stepUpEnabled && previousSettings.StepUpEnabled {
		if !middleware.EnforceStepUpAlways(c, h.totpService, h.userService) {
			return false
		}
	}

	prepared.passkeyEnabled = passkeyEnabled
	prepared.sessionBindingEnabled = sessionBindingEnabled
	prepared.stepUpEnabled = stepUpEnabled
	prepared.localCaptchaEnabled = localCaptchaEnabled
	prepared.recaptchaEnabled = recaptchaEnabled
	prepared.capEnabled = capEnabled
	prepared.tencentCaptchaEnabled = tencentCaptchaEnabled
	prepared.aliyunCaptchaEnabled = aliyunCaptchaEnabled
	prepared.clientIPResolutionMode = clientIPResolutionMode
	prepared.clientIPTrustedProxies = clientIPTrustedProxies
	return true
}

package admin

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"

	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) validateCoreSettingsUpdate(c *gin.Context, prepared *preparedSettingsUpdate) bool {
	req := prepared.request
	previousSettings := prepared.previousSettings
	recaptchaEnabled := prepared.recaptchaEnabled
	capEnabled := prepared.capEnabled
	tencentCaptchaEnabled := prepared.tencentCaptchaEnabled
	aliyunCaptchaEnabled := prepared.aliyunCaptchaEnabled

	// 验证参数
	if req.DefaultConcurrency < 1 {
		req.DefaultConcurrency = 1
	}
	if req.DefaultBalance < 0 {
		req.DefaultBalance = 0
	}
	affiliateRebateRate := previousSettings.AffiliateRebateRate
	if req.AffiliateRebateRate != nil {
		affiliateRebateRate = *req.AffiliateRebateRate
	}
	if affiliateRebateRate < service.AffiliateRebateRateMin {
		affiliateRebateRate = service.AffiliateRebateRateMin
	}
	if affiliateRebateRate > service.AffiliateRebateRateMax {
		affiliateRebateRate = service.AffiliateRebateRateMax
	}
	affiliateRebateFreezeHours := previousSettings.AffiliateRebateFreezeHours
	if req.AffiliateRebateFreezeHours != nil {
		affiliateRebateFreezeHours = *req.AffiliateRebateFreezeHours
	}
	if affiliateRebateFreezeHours < 0 {
		affiliateRebateFreezeHours = service.AffiliateRebateFreezeHoursDefault
	}
	if affiliateRebateFreezeHours > service.AffiliateRebateFreezeHoursMax {
		affiliateRebateFreezeHours = service.AffiliateRebateFreezeHoursMax
	}
	affiliateRebateDurationDays := previousSettings.AffiliateRebateDurationDays
	if req.AffiliateRebateDurationDays != nil {
		affiliateRebateDurationDays = *req.AffiliateRebateDurationDays
	}
	if affiliateRebateDurationDays < 0 {
		affiliateRebateDurationDays = service.AffiliateRebateDurationDaysDefault
	}
	if affiliateRebateDurationDays > service.AffiliateRebateDurationDaysMax {
		affiliateRebateDurationDays = service.AffiliateRebateDurationDaysMax
	}
	affiliateRebatePerInviteeCap := previousSettings.AffiliateRebatePerInviteeCap
	if req.AffiliateRebatePerInviteeCap != nil {
		affiliateRebatePerInviteeCap = *req.AffiliateRebatePerInviteeCap
	}
	if affiliateRebatePerInviteeCap < 0 {
		affiliateRebatePerInviteeCap = service.AffiliateRebatePerInviteeCapDefault
	}
	adminRechargeRebateEnabled := previousSettings.AdminRechargeRebateEnabled
	if req.AdminRechargeRebateEnabled != nil {
		adminRechargeRebateEnabled = *req.AdminRechargeRebateEnabled
	}
	// 通用表格配置：兼容旧客户端未传字段时保留当前值。
	if req.TableDefaultPageSize <= 0 {
		req.TableDefaultPageSize = previousSettings.TableDefaultPageSize
	}
	if req.TablePageSizeOptions == nil {
		req.TablePageSizeOptions = previousSettings.TablePageSizeOptions
	}
	req.SMTPHost = strings.TrimSpace(req.SMTPHost)
	req.SMTPUsername = strings.TrimSpace(req.SMTPUsername)
	req.SMTPPassword = strings.TrimSpace(req.SMTPPassword)
	req.SMTPFrom = strings.TrimSpace(req.SMTPFrom)
	req.SMTPFromName = strings.TrimSpace(req.SMTPFromName)
	if req.SMTPPort <= 0 {
		req.SMTPPort = 587
	}
	req.DefaultSubscriptions = normalizeDefaultSubscriptions(req.DefaultSubscriptions)
	req.AuthSourceDefaultEmailSubscriptions = normalizeOptionalDefaultSubscriptions(req.AuthSourceDefaultEmailSubscriptions)
	req.AuthSourceDefaultLinuxDoSubscriptions = normalizeOptionalDefaultSubscriptions(req.AuthSourceDefaultLinuxDoSubscriptions)
	req.AuthSourceDefaultOIDCSubscriptions = normalizeOptionalDefaultSubscriptions(req.AuthSourceDefaultOIDCSubscriptions)
	req.AuthSourceDefaultWeChatSubscriptions = normalizeOptionalDefaultSubscriptions(req.AuthSourceDefaultWeChatSubscriptions)
	req.AuthSourceDefaultDingTalkSubscriptions = normalizeOptionalDefaultSubscriptions(req.AuthSourceDefaultDingTalkSubscriptions)

	// SMTP 配置保护：如果请求中 smtp_host 为空但数据库中已有配置，则保留已有 SMTP 配置
	// 防止前端加载设置失败时空表单覆盖已保存的 SMTP 配置
	if req.SMTPHost == "" && previousSettings.SMTPHost != "" {
		req.SMTPHost = previousSettings.SMTPHost
		req.SMTPPort = previousSettings.SMTPPort
		req.SMTPUsername = previousSettings.SMTPUsername
		req.SMTPFrom = previousSettings.SMTPFrom
		req.SMTPFromName = previousSettings.SMTPFromName
		req.SMTPUseTLS = previousSettings.SMTPUseTLS
	}

	// Turnstile 参数验证
	if req.TurnstileEnabled {
		// 检查必填字段
		if req.TurnstileSiteKey == "" {
			response.BadRequest(c, "Turnstile Site Key is required when enabled")
			return false
		}
		// 如果未提供 secret key，使用已保存的值（留空保留当前值）
		if req.TurnstileSecretKey == "" {
			if previousSettings.TurnstileSecretKey == "" {
				response.BadRequest(c, "Turnstile Secret Key is required when enabled")
				return false
			}
			req.TurnstileSecretKey = previousSettings.TurnstileSecretKey
		}

		// 当 site_key 或 secret_key 任一变化时验证（避免配置错误导致无法登录）
		siteKeyChanged := previousSettings.TurnstileSiteKey != req.TurnstileSiteKey
		secretKeyChanged := previousSettings.TurnstileSecretKey != req.TurnstileSecretKey
		if siteKeyChanged || secretKeyChanged {
			if err := h.turnstileService.ValidateSecretKey(c.Request.Context(), req.TurnstileSecretKey); err != nil {
				response.ErrorFrom(c, err)
				return false
			}
		}
	}

	if recaptchaEnabled {
		if strings.TrimSpace(req.RecaptchaSiteKey) == "" && previousSettings.RecaptchaSiteKey != "" {
			req.RecaptchaSiteKey = previousSettings.RecaptchaSiteKey
		}
		if strings.TrimSpace(req.RecaptchaSiteKey) == "" {
			response.BadRequest(c, "reCAPTCHA Site Key is required when enabled")
			return false
		}
		if req.RecaptchaSecretKey == "" {
			if previousSettings.RecaptchaSecretKey == "" {
				response.BadRequest(c, "reCAPTCHA Secret Key is required when enabled")
				return false
			}
			req.RecaptchaSecretKey = previousSettings.RecaptchaSecretKey
		}
		siteKeyChanged := previousSettings.RecaptchaSiteKey != req.RecaptchaSiteKey
		secretKeyChanged := previousSettings.RecaptchaSecretKey != req.RecaptchaSecretKey
		if (siteKeyChanged || secretKeyChanged) && h.turnstileService != nil {
			if err := h.turnstileService.ValidateRecaptchaSecretKey(c.Request.Context(), req.RecaptchaSecretKey); err != nil {
				response.ErrorFrom(c, err)
				return false
			}
		}
	}

	if capEnabled {
		if strings.TrimSpace(req.CapAPIEndpoint) == "" && previousSettings.CapAPIEndpoint != "" {
			req.CapAPIEndpoint = previousSettings.CapAPIEndpoint
		}
		req.CapAPIEndpoint = strings.TrimRight(strings.TrimSpace(req.CapAPIEndpoint), "/")
		if err := service.ValidateCapAPIEndpoint(req.CapAPIEndpoint); err != nil {
			response.BadRequest(c, err.Error())
			return false
		}
		if req.CapSecretKey == "" {
			if previousSettings.CapSecretKey == "" {
				response.BadRequest(c, "Cap Secret Key is required when enabled")
				return false
			}
			req.CapSecretKey = previousSettings.CapSecretKey
		}
		endpointChanged := previousSettings.CapAPIEndpoint != req.CapAPIEndpoint
		secretKeyChanged := previousSettings.CapSecretKey != req.CapSecretKey
		if (endpointChanged || secretKeyChanged) && h.turnstileService != nil {
			if err := h.turnstileService.ValidateCapConfiguration(c.Request.Context(), req.CapAPIEndpoint, req.CapSecretKey); err != nil {
				response.ErrorFrom(c, err)
				return false
			}
		}
	}

	if strings.EqualFold(strings.TrimSpace(req.TencentCaptchaRegion), service.TencentCaptchaRegionINTL) {
		req.TencentCaptchaRegion = service.TencentCaptchaRegionINTL
	} else {
		req.TencentCaptchaRegion = service.TencentCaptchaRegionCN
	}
	if tencentCaptchaEnabled {
		req.TencentCaptchaAppID = strings.TrimSpace(req.TencentCaptchaAppID)
		appID, err := strconv.ParseUint(req.TencentCaptchaAppID, 10, 64)
		if err != nil || appID == 0 {
			response.BadRequest(c, "Tencent Captcha CaptchaAppId must be a positive integer when enabled")
			return false
		}

		req.TencentCaptchaAppSecretKey = strings.TrimSpace(req.TencentCaptchaAppSecretKey)
		req.TencentCaptchaCloudSecretID = strings.TrimSpace(req.TencentCaptchaCloudSecretID)
		req.TencentCaptchaCloudSecretKey = strings.TrimSpace(req.TencentCaptchaCloudSecretKey)
		if req.TencentCaptchaAppSecretKey == "" {
			req.TencentCaptchaAppSecretKey = previousSettings.TencentCaptchaAppSecretKey
		}
		if req.TencentCaptchaCloudSecretID == "" {
			req.TencentCaptchaCloudSecretID = previousSettings.TencentCaptchaCloudSecretID
		}
		if req.TencentCaptchaCloudSecretKey == "" {
			req.TencentCaptchaCloudSecretKey = previousSettings.TencentCaptchaCloudSecretKey
		}
		if req.TencentCaptchaAppSecretKey == "" {
			response.BadRequest(c, "Tencent Captcha AppSecretKey is required when enabled")
			return false
		}
		if req.TencentCaptchaCloudSecretID == "" {
			response.BadRequest(c, "Tencent Cloud SecretId is required when Tencent Captcha is enabled")
			return false
		}
		if req.TencentCaptchaCloudSecretKey == "" {
			response.BadRequest(c, "Tencent Cloud SecretKey is required when Tencent Captcha is enabled")
			return false
		}
	}

	req.AliyunCaptchaAccessKeyID = strings.TrimSpace(req.AliyunCaptchaAccessKeyID)
	req.AliyunCaptchaAccessKeySecret = strings.TrimSpace(req.AliyunCaptchaAccessKeySecret)
	req.AliyunCaptchaSceneID = strings.TrimSpace(req.AliyunCaptchaSceneID)
	req.AliyunCaptchaPrefix = strings.TrimSpace(req.AliyunCaptchaPrefix)
	if strings.EqualFold(strings.TrimSpace(req.AliyunCaptchaRegion), service.AliyunCaptchaRegionSGP) {
		req.AliyunCaptchaRegion = service.AliyunCaptchaRegionSGP
	} else {
		req.AliyunCaptchaRegion = service.AliyunCaptchaRegionCN
	}
	if aliyunCaptchaEnabled {
		if req.AliyunCaptchaAccessKeyID == "" {
			req.AliyunCaptchaAccessKeyID = previousSettings.AliyunCaptchaAccessKeyID
		}
		if req.AliyunCaptchaAccessKeySecret == "" {
			req.AliyunCaptchaAccessKeySecret = previousSettings.AliyunCaptchaAccessKeySecret
		}
		if req.AliyunCaptchaAccessKeyID == "" {
			response.BadRequest(c, "Aliyun Captcha AccessKey ID is required when enabled")
			return false
		}
		if req.AliyunCaptchaAccessKeySecret == "" {
			response.BadRequest(c, "Aliyun Captcha AccessKey Secret is required when enabled")
			return false
		}
		if req.AliyunCaptchaSceneID == "" {
			response.BadRequest(c, "Aliyun Captcha Scene ID is required when enabled")
			return false
		}
		if req.AliyunCaptchaPrefix == "" {
			response.BadRequest(c, "Aliyun Captcha Prefix is required when enabled")
			return false
		}
		if err := service.ValidateAliyunCaptchaPrefix(req.AliyunCaptchaPrefix); err != nil {
			response.BadRequest(c, err.Error())
			return false
		}

		credentialsChanged := !previousSettings.AliyunCaptchaEnabled ||
			previousSettings.AliyunCaptchaAccessKeyID != req.AliyunCaptchaAccessKeyID ||
			previousSettings.AliyunCaptchaAccessKeySecret != req.AliyunCaptchaAccessKeySecret ||
			previousSettings.AliyunCaptchaSceneID != req.AliyunCaptchaSceneID ||
			previousSettings.AliyunCaptchaRegion != req.AliyunCaptchaRegion
		if credentialsChanged {
			if h.turnstileService == nil {
				response.ErrorFrom(c, service.ErrAliyunCaptchaNotConfigured)
				return false
			}
			err := h.turnstileService.ValidateAliyunCaptchaConfiguration(c.Request.Context(), service.AliyunCaptchaConfig{
				Enabled:         true,
				AccessKeyID:     req.AliyunCaptchaAccessKeyID,
				AccessKeySecret: req.AliyunCaptchaAccessKeySecret,
				SceneID:         req.AliyunCaptchaSceneID,
				Prefix:          req.AliyunCaptchaPrefix,
				Region:          req.AliyunCaptchaRegion,
			})
			if err != nil {
				response.ErrorFrom(c, err)
				return false
			}
		}
	}

	// TOTP 双因素认证参数验证
	// 只有手动配置了加密密钥才允许启用 TOTP 功能
	if req.TotpEnabled && !previousSettings.TotpEnabled {
		// 尝试启用 TOTP，检查加密密钥是否已手动配置
		if !h.settingService.IsTotpEncryptionKeyConfigured() {
			response.BadRequest(c, "Cannot enable TOTP: a stable TOTP encryption key is not available. Configure TOTP_ENCRYPTION_KEY consistently on every instance or verify database secret bootstrap.")
			return false
		}
	}
	loginAgreementMode := strings.ToLower(strings.TrimSpace(req.LoginAgreementMode))
	if loginAgreementMode == "" {
		loginAgreementMode = strings.ToLower(strings.TrimSpace(previousSettings.LoginAgreementMode))
	}
	switch loginAgreementMode {
	case "", "modal":
		loginAgreementMode = "modal"
	case "checkbox":
	default:
		response.BadRequest(c, "Login agreement mode must be modal or checkbox")
		return false
	}
	loginAgreementUpdatedAt := strings.TrimSpace(req.LoginAgreementUpdatedAt)
	if loginAgreementUpdatedAt == "" {
		loginAgreementUpdatedAt = strings.TrimSpace(previousSettings.LoginAgreementUpdatedAt)
	}
	loginAgreementDocuments := loginAgreementDocumentsToService(req.LoginAgreementDocuments)
	if len(loginAgreementDocuments) == 0 {
		loginAgreementDocuments = previousSettings.LoginAgreementDocuments
	}
	for _, doc := range loginAgreementDocuments {
		if strings.TrimSpace(doc.Title) == "" {
			response.BadRequest(c, "Login agreement document title is required")
			return false
		}
		if len(doc.Title) > 80 {
			response.BadRequest(c, "Login agreement document title is too long (max 80 characters)")
			return false
		}
		if len(doc.ContentMD) > 200*1024 {
			response.BadRequest(c, "Login agreement document content is too large (max 200KB)")
			return false
		}
	}
	if req.LoginAgreementEnabled && len(loginAgreementDocuments) == 0 {
		response.BadRequest(c, "Login agreement documents are required when enabled")
		return false
	}

	prepared.affiliateRebateRate = affiliateRebateRate
	prepared.affiliateFreezeHours = affiliateRebateFreezeHours
	prepared.affiliateDurationDays = affiliateRebateDurationDays
	prepared.affiliatePerInviteeCap = affiliateRebatePerInviteeCap
	prepared.adminRechargeRebate = adminRechargeRebateEnabled
	prepared.loginAgreementMode = loginAgreementMode
	prepared.loginAgreementUpdatedAt = loginAgreementUpdatedAt
	prepared.loginAgreementDocuments = loginAgreementDocuments
	return true
}

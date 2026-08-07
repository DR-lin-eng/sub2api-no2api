package service

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/shared/ip"
)

func (s *SettingService) parseCoreSystemSettings(settings map[string]string) *SystemSettings {
	emailVerifyEnabled := settings[SettingKeyEmailVerifyEnabled] == "true"
	loginAgreementDocuments := parseLoginAgreementDocuments(settings[SettingKeyLoginAgreementDocuments])
	loginAgreementUpdatedAt := strings.TrimSpace(settings[SettingKeyLoginAgreementUpdatedAt])
	if loginAgreementUpdatedAt == "" {
		loginAgreementUpdatedAt = defaultLoginAgreementDate
	}
	clientIPResolutionMode := strings.TrimSpace(settings[SettingKeyClientIPResolutionMode])
	if ip.ValidateResolutionMode(clientIPResolutionMode) != nil {
		clientIPResolutionMode = ip.ResolutionModeAutoCompat
	}
	clientIPTrustedProxies := make([]string, 0)
	if raw := strings.TrimSpace(settings[SettingKeyClientIPTrustedProxies]); raw != "" {
		if err := json.Unmarshal([]byte(raw), &clientIPTrustedProxies); err != nil {
			clientIPTrustedProxies = make([]string, 0)
		}
	}
	result := &SystemSettings{
		RegistrationEnabled:                    settings[SettingKeyRegistrationEnabled] == "true",
		EmailVerifyEnabled:                     emailVerifyEnabled,
		RegistrationEmailSuffixWhitelist:       ParseRegistrationEmailSuffixWhitelist(settings[SettingKeyRegistrationEmailSuffixWhitelist]),
		PromoCodeEnabled:                       settings[SettingKeyPromoCodeEnabled] != "false", // 默认启用
		PasswordResetEnabled:                   emailVerifyEnabled && settings[SettingKeyPasswordResetEnabled] == "true",
		FrontendURL:                            settings[SettingKeyFrontendURL],
		InvitationCodeEnabled:                  settings[SettingKeyInvitationCodeEnabled] == "true",
		TotpEnabled:                            settings[SettingKeyTotpEnabled] == "true",
		PasskeyEnabled:                         s.passkeySettingEnabled(settings),
		SessionBindingEnabled:                  settings[SettingKeySessionBindingEnabled] == "true", // 默认关闭
		StepUpEnabled:                          settings[SettingKeyStepUpEnabled] == "true",         // 默认关闭
		AuditLogRetentionDays:                  parseAuditLogRetentionDays(settings[SettingKeyAuditLogRetentionDays]),
		LoginAgreementEnabled:                  settings[SettingKeyLoginAgreementEnabled] == "true",
		LoginAgreementMode:                     normalizeLoginAgreementMode(settings[SettingKeyLoginAgreementMode]),
		LoginAgreementUpdatedAt:                loginAgreementUpdatedAt,
		LoginAgreementDocuments:                loginAgreementDocuments,
		SMTPHost:                               settings[SettingKeySMTPHost],
		SMTPUsername:                           settings[SettingKeySMTPUsername],
		SMTPFrom:                               settings[SettingKeySMTPFrom],
		SMTPFromName:                           settings[SettingKeySMTPFromName],
		SMTPUseTLS:                             settings[SettingKeySMTPUseTLS] == "true",
		SMTPPasswordConfigured:                 settings[SettingKeySMTPPassword] != "",
		TurnstileEnabled:                       settings[SettingKeyTurnstileEnabled] == "true",
		TurnstileSiteKey:                       settings[SettingKeyTurnstileSiteKey],
		TurnstileSecretKeyConfigured:           settings[SettingKeyTurnstileSecretKey] != "",
		RecaptchaEnabled:                       settings[SettingKeyRecaptchaEnabled] == "true",
		RecaptchaSiteKey:                       settings[SettingKeyRecaptchaSiteKey],
		RecaptchaSecretKeyConfigured:           settings[SettingKeyRecaptchaSecretKey] != "",
		CapEnabled:                             settings[SettingKeyCapEnabled] == "true",
		CapAPIEndpoint:                         settings[SettingKeyCapAPIEndpoint],
		CapSecretKeyConfigured:                 settings[SettingKeyCapSecretKey] != "",
		TencentCaptchaEnabled:                  settings[SettingKeyTencentCaptchaEnabled] == "true",
		TencentCaptchaAppID:                    settings[SettingKeyTencentCaptchaAppID],
		TencentCaptchaAppSecretKeyConfigured:   settings[SettingKeyTencentCaptchaAppSecretKey] != "",
		TencentCaptchaCloudSecretIDConfigured:  settings[SettingKeyTencentCaptchaCloudSecretID] != "",
		TencentCaptchaCloudSecretKeyConfigured: settings[SettingKeyTencentCaptchaCloudSecretKey] != "",
		TencentCaptchaRegion:                   normalizeTencentCaptchaRegion(settings[SettingKeyTencentCaptchaRegion]),
		AliyunCaptchaEnabled:                   settings[SettingKeyAliyunCaptchaEnabled] == "true",
		AliyunCaptchaAccessKeyIDConfigured:     settings[SettingKeyAliyunCaptchaAccessKeyID] != "",
		AliyunCaptchaAccessKeySecretConfigured: settings[SettingKeyAliyunCaptchaAccessKeySecret] != "",
		AliyunCaptchaSceneID:                   strings.TrimSpace(settings[SettingKeyAliyunCaptchaSceneID]),
		AliyunCaptchaPrefix:                    strings.TrimSpace(settings[SettingKeyAliyunCaptchaPrefix]),
		AliyunCaptchaRegion:                    normalizeAliyunCaptchaRegion(settings[SettingKeyAliyunCaptchaRegion]),
		LocalCaptchaEnabled:                    settings[SettingKeyLocalCaptchaEnabled] == "true", // 默认关闭
		APIKeyACLTrustForwardedIP:              clientIPResolutionMode != ip.ResolutionModeDirect,
		ClientIPResolutionMode:                 clientIPResolutionMode,
		ClientIPTrustedProxies:                 clientIPTrustedProxies,
		ClientIPResolutionStatus: ip.ResolutionStatus{
			Mode:                   clientIPResolutionMode,
			CloudflareRangesSource: "embedded",
		},
		SiteName:                     s.getStringOrDefault(settings, SettingKeySiteName, "Sub2API"),
		SiteLogo:                     settings[SettingKeySiteLogo],
		SiteSubtitle:                 s.getStringOrDefault(settings, SettingKeySiteSubtitle, "Subscription to API Conversion Platform"),
		APIBaseURL:                   settings[SettingKeyAPIBaseURL],
		ContactInfo:                  settings[SettingKeyContactInfo],
		DocURL:                       settings[SettingKeyDocURL],
		HomeContent:                  settings[SettingKeyHomeContent],
		CompactHomeEnabled:           settings[SettingKeyCompactHomeEnabled] == "true",
		HideCcsImportButton:          settings[SettingKeyHideCcsImportButton] == "true",
		PurchaseSubscriptionEnabled:  settings[SettingKeyPurchaseSubscriptionEnabled] == "true",
		PurchaseSubscriptionURL:      strings.TrimSpace(settings[SettingKeyPurchaseSubscriptionURL]),
		CustomMenuItems:              settings[SettingKeyCustomMenuItems],
		CustomEndpoints:              settings[SettingKeyCustomEndpoints],
		BackendModeEnabled:           settings[SettingKeyBackendModeEnabled] == "true",
		StreamModePerformanceEnabled: settings[SettingKeyStreamModePerformanceEnabled] == "true",
	}
	result.TableDefaultPageSize, result.TablePageSizeOptions = parseTablePreferences(
		settings[SettingKeyTableDefaultPageSize],
		settings[SettingKeyTablePageSizeOptions],
	)

	// 解析整数类型
	if port, err := strconv.Atoi(settings[SettingKeySMTPPort]); err == nil {
		result.SMTPPort = port
	} else {
		result.SMTPPort = 587
	}

	if concurrency, err := strconv.Atoi(settings[SettingKeyDefaultConcurrency]); err == nil {
		result.DefaultConcurrency = concurrency
	} else {
		result.DefaultConcurrency = s.cfg.Default.UserConcurrency
	}

	if rpm, err := strconv.Atoi(settings[SettingKeyDefaultUserRPMLimit]); err == nil && rpm >= 0 {
		result.DefaultUserRPMLimit = rpm
	}

	// 解析浮点数类型
	if balance, err := strconv.ParseFloat(settings[SettingKeyDefaultBalance], 64); err == nil {
		result.DefaultBalance = balance
	} else {
		result.DefaultBalance = s.cfg.Default.UserBalance
	}
	if rebateRate, err := strconv.ParseFloat(settings[SettingKeyAffiliateRebateRate], 64); err == nil {
		result.AffiliateRebateRate = clampAffiliateRebateRate(rebateRate)
	} else {
		result.AffiliateRebateRate = AffiliateRebateRateDefault
	}
	if freezeHours, err := strconv.Atoi(settings[SettingKeyAffiliateRebateFreezeHours]); err == nil && freezeHours >= 0 {
		if freezeHours > AffiliateRebateFreezeHoursMax {
			freezeHours = AffiliateRebateFreezeHoursMax
		}
		result.AffiliateRebateFreezeHours = freezeHours
	}
	if durationDays, err := strconv.Atoi(settings[SettingKeyAffiliateRebateDurationDays]); err == nil && durationDays >= 0 {
		if durationDays > AffiliateRebateDurationDaysMax {
			durationDays = AffiliateRebateDurationDaysMax
		}
		result.AffiliateRebateDurationDays = durationDays
	}
	if perInviteeCap, err := strconv.ParseFloat(settings[SettingKeyAffiliateRebatePerInviteeCap], 64); err == nil && perInviteeCap >= 0 {
		result.AffiliateRebatePerInviteeCap = perInviteeCap
	}
	result.AdminRechargeRebateEnabled = settings[SettingKeyAffiliateAdminRechargeEnabled] == "true"
	result.DefaultSubscriptions = parseDefaultSubscriptions(settings[SettingKeyDefaultSubscriptions])

	// 敏感信息直接返回，方便测试连接时使用
	result.SMTPPassword = settings[SettingKeySMTPPassword]
	result.TurnstileSecretKey = settings[SettingKeyTurnstileSecretKey]
	result.RecaptchaSecretKey = settings[SettingKeyRecaptchaSecretKey]
	result.CapSecretKey = settings[SettingKeyCapSecretKey]
	result.TencentCaptchaAppSecretKey = settings[SettingKeyTencentCaptchaAppSecretKey]
	result.TencentCaptchaCloudSecretID = settings[SettingKeyTencentCaptchaCloudSecretID]
	result.TencentCaptchaCloudSecretKey = settings[SettingKeyTencentCaptchaCloudSecretKey]
	result.AliyunCaptchaAccessKeyID = settings[SettingKeyAliyunCaptchaAccessKeyID]
	result.AliyunCaptchaAccessKeySecret = settings[SettingKeyAliyunCaptchaAccessKeySecret]

	return result
}

package admin

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler/dto"
)

func buildSystemSettingsUpdate(prepared *preparedSettingsUpdate) *service.SystemSettings {
	req := prepared.request
	previousSettings := prepared.previousSettings
	passkeyEnabled := prepared.passkeyEnabled
	sessionBindingEnabled := prepared.sessionBindingEnabled
	stepUpEnabled := prepared.stepUpEnabled
	localCaptchaEnabled := prepared.localCaptchaEnabled
	recaptchaEnabled := prepared.recaptchaEnabled
	capEnabled := prepared.capEnabled
	tencentCaptchaEnabled := prepared.tencentCaptchaEnabled
	aliyunCaptchaEnabled := prepared.aliyunCaptchaEnabled
	clientIPResolutionMode := prepared.clientIPResolutionMode
	clientIPTrustedProxies := prepared.clientIPTrustedProxies
	affiliateRebateRate := prepared.affiliateRebateRate
	affiliateRebateFreezeHours := prepared.affiliateFreezeHours
	affiliateRebateDurationDays := prepared.affiliateDurationDays
	affiliateRebatePerInviteeCap := prepared.affiliatePerInviteeCap
	adminRechargeRebateEnabled := prepared.adminRechargeRebate
	loginAgreementMode := prepared.loginAgreementMode
	loginAgreementUpdatedAt := prepared.loginAgreementUpdatedAt
	loginAgreementDocuments := prepared.loginAgreementDocuments
	oidcUsePKCE := prepared.oidcUsePKCE
	oidcValidateIDToken := prepared.oidcValidateIDToken
	purchaseEnabled := prepared.purchaseEnabled
	purchaseURL := prepared.purchaseURL
	customMenuJSON := prepared.customMenuJSON
	customEndpointsJSON := prepared.customEndpointsJSON
	defaultSubscriptions := prepared.defaultSubscriptions

	return &service.SystemSettings{
		// 系统全局 platform quota 默认值（整体替换语义）
		DefaultPlatformQuotas: req.DefaultPlatformQuotas,

		RegistrationEnabled:              req.RegistrationEnabled,
		EmailVerifyEnabled:               req.EmailVerifyEnabled,
		RegistrationEmailSuffixWhitelist: req.RegistrationEmailSuffixWhitelist,
		PromoCodeEnabled:                 req.PromoCodeEnabled,
		PasswordResetEnabled:             req.PasswordResetEnabled,
		FrontendURL:                      req.FrontendURL,
		InvitationCodeEnabled:            req.InvitationCodeEnabled,
		TotpEnabled:                      req.TotpEnabled,
		PasskeyEnabled:                   passkeyEnabled,
		SessionBindingEnabled:            sessionBindingEnabled,
		StepUpEnabled:                    stepUpEnabled,
		AuditLogRetentionDays:            req.AuditLogRetentionDays,
		LoginAgreementEnabled:            req.LoginAgreementEnabled,
		LoginAgreementMode:               loginAgreementMode,
		LoginAgreementUpdatedAt:          loginAgreementUpdatedAt,
		LoginAgreementDocuments:          loginAgreementDocuments,
		SMTPHost:                         req.SMTPHost,
		SMTPPort:                         req.SMTPPort,
		SMTPUsername:                     req.SMTPUsername,
		SMTPPassword:                     req.SMTPPassword,
		SMTPFrom:                         req.SMTPFrom,
		SMTPFromName:                     req.SMTPFromName,
		SMTPUseTLS:                       req.SMTPUseTLS,
		TurnstileEnabled:                 req.TurnstileEnabled,
		TurnstileSiteKey:                 req.TurnstileSiteKey,
		TurnstileSecretKey:               req.TurnstileSecretKey,
		RecaptchaEnabled:                 recaptchaEnabled,
		RecaptchaSiteKey:                 req.RecaptchaSiteKey,
		RecaptchaSecretKey:               req.RecaptchaSecretKey,
		CapEnabled:                       capEnabled,
		CapAPIEndpoint:                   req.CapAPIEndpoint,
		CapSecretKey:                     req.CapSecretKey,
		TencentCaptchaEnabled:            tencentCaptchaEnabled,
		TencentCaptchaAppID:              req.TencentCaptchaAppID,
		TencentCaptchaAppSecretKey:       req.TencentCaptchaAppSecretKey,
		TencentCaptchaCloudSecretID:      req.TencentCaptchaCloudSecretID,
		TencentCaptchaCloudSecretKey:     req.TencentCaptchaCloudSecretKey,
		TencentCaptchaRegion:             req.TencentCaptchaRegion,
		AliyunCaptchaEnabled:             aliyunCaptchaEnabled,
		AliyunCaptchaAccessKeyID:         req.AliyunCaptchaAccessKeyID,
		AliyunCaptchaAccessKeySecret:     req.AliyunCaptchaAccessKeySecret,
		AliyunCaptchaSceneID:             req.AliyunCaptchaSceneID,
		AliyunCaptchaPrefix:              req.AliyunCaptchaPrefix,
		AliyunCaptchaRegion:              req.AliyunCaptchaRegion,
		LocalCaptchaEnabled:              localCaptchaEnabled,
		// The deprecated boolean is accepted but intentionally ignored so a
		// cached pre-upgrade admin page cannot re-enable the v0.1.161 regression.
		APIKeyACLTrustForwardedIP:                          previousSettings.APIKeyACLTrustForwardedIP,
		ClientIPResolutionMode:                             clientIPResolutionMode,
		ClientIPTrustedProxies:                             clientIPTrustedProxies,
		LinuxDoConnectEnabled:                              req.LinuxDoConnectEnabled,
		LinuxDoConnectClientID:                             req.LinuxDoConnectClientID,
		LinuxDoConnectClientSecret:                         req.LinuxDoConnectClientSecret,
		LinuxDoConnectRedirectURL:                          req.LinuxDoConnectRedirectURL,
		DingTalkConnectEnabled:                             req.DingTalkConnectEnabled,
		DingTalkConnectClientID:                            req.DingTalkConnectClientID,
		DingTalkConnectClientSecret:                        req.DingTalkConnectClientSecret,
		DingTalkConnectRedirectURL:                         req.DingTalkConnectRedirectURL,
		DingTalkConnectCorpRestrictionPolicy:               req.DingTalkConnectCorpRestrictionPolicy,
		DingTalkConnectInternalCorpID:                      req.DingTalkConnectInternalCorpID,
		DingTalkConnectBypassRegistration:                  req.DingTalkConnectBypassRegistration,
		DingTalkConnectSyncCorpEmail:                       req.DingTalkConnectSyncCorpEmail,
		DingTalkConnectSyncDisplayName:                     req.DingTalkConnectSyncDisplayName,
		DingTalkConnectSyncDept:                            req.DingTalkConnectSyncDept,
		DingTalkConnectSyncCorpEmailAttrKey:                req.DingTalkConnectSyncCorpEmailAttrKey,
		DingTalkConnectSyncDisplayNameAttrKey:              req.DingTalkConnectSyncDisplayNameAttrKey,
		DingTalkConnectSyncDeptAttrKey:                     req.DingTalkConnectSyncDeptAttrKey,
		DingTalkConnectSyncCorpEmailAttrName:               req.DingTalkConnectSyncCorpEmailAttrName,
		DingTalkConnectSyncDisplayNameAttrName:             req.DingTalkConnectSyncDisplayNameAttrName,
		DingTalkConnectSyncDeptAttrName:                    req.DingTalkConnectSyncDeptAttrName,
		WeChatConnectEnabled:                               req.WeChatConnectEnabled,
		WeChatConnectAppID:                                 req.WeChatConnectAppID,
		WeChatConnectAppSecret:                             req.WeChatConnectAppSecret,
		WeChatConnectOpenAppID:                             req.WeChatConnectOpenAppID,
		WeChatConnectOpenAppSecret:                         req.WeChatConnectOpenAppSecret,
		WeChatConnectMPAppID:                               req.WeChatConnectMPAppID,
		WeChatConnectMPAppSecret:                           req.WeChatConnectMPAppSecret,
		WeChatConnectMobileAppID:                           req.WeChatConnectMobileAppID,
		WeChatConnectMobileAppSecret:                       req.WeChatConnectMobileAppSecret,
		WeChatConnectOpenEnabled:                           req.WeChatConnectOpenEnabled,
		WeChatConnectMPEnabled:                             req.WeChatConnectMPEnabled,
		WeChatConnectMobileEnabled:                         req.WeChatConnectMobileEnabled,
		WeChatConnectMode:                                  req.WeChatConnectMode,
		WeChatConnectScopes:                                req.WeChatConnectScopes,
		WeChatConnectRedirectURL:                           req.WeChatConnectRedirectURL,
		WeChatConnectFrontendRedirectURL:                   req.WeChatConnectFrontendRedirectURL,
		OIDCConnectEnabled:                                 req.OIDCConnectEnabled,
		OIDCConnectProviderName:                            req.OIDCConnectProviderName,
		OIDCConnectClientID:                                req.OIDCConnectClientID,
		OIDCConnectClientSecret:                            req.OIDCConnectClientSecret,
		OIDCConnectIssuerURL:                               req.OIDCConnectIssuerURL,
		OIDCConnectDiscoveryURL:                            req.OIDCConnectDiscoveryURL,
		OIDCConnectAuthorizeURL:                            req.OIDCConnectAuthorizeURL,
		OIDCConnectTokenURL:                                req.OIDCConnectTokenURL,
		OIDCConnectUserInfoURL:                             req.OIDCConnectUserInfoURL,
		OIDCConnectJWKSURL:                                 req.OIDCConnectJWKSURL,
		OIDCConnectScopes:                                  req.OIDCConnectScopes,
		OIDCConnectRedirectURL:                             req.OIDCConnectRedirectURL,
		OIDCConnectFrontendRedirectURL:                     req.OIDCConnectFrontendRedirectURL,
		OIDCConnectTokenAuthMethod:                         req.OIDCConnectTokenAuthMethod,
		OIDCConnectUsePKCE:                                 oidcUsePKCE,
		OIDCConnectValidateIDToken:                         oidcValidateIDToken,
		OIDCConnectAllowedSigningAlgs:                      req.OIDCConnectAllowedSigningAlgs,
		OIDCConnectClockSkewSeconds:                        req.OIDCConnectClockSkewSeconds,
		OIDCConnectRequireEmailVerified:                    req.OIDCConnectRequireEmailVerified,
		OIDCConnectUserInfoEmailPath:                       req.OIDCConnectUserInfoEmailPath,
		OIDCConnectUserInfoIDPath:                          req.OIDCConnectUserInfoIDPath,
		OIDCConnectUserInfoUsernamePath:                    req.OIDCConnectUserInfoUsernamePath,
		GitHubOAuthEnabled:                                 req.GitHubOAuthEnabled,
		GitHubOAuthClientID:                                req.GitHubOAuthClientID,
		GitHubOAuthClientSecret:                            req.GitHubOAuthClientSecret,
		GitHubOAuthRedirectURL:                             req.GitHubOAuthRedirectURL,
		GitHubOAuthFrontendRedirectURL:                     req.GitHubOAuthFrontendRedirectURL,
		GoogleOAuthEnabled:                                 req.GoogleOAuthEnabled,
		GoogleOAuthClientID:                                req.GoogleOAuthClientID,
		GoogleOAuthClientSecret:                            req.GoogleOAuthClientSecret,
		GoogleOAuthRedirectURL:                             req.GoogleOAuthRedirectURL,
		GoogleOAuthFrontendRedirectURL:                     req.GoogleOAuthFrontendRedirectURL,
		SiteName:                                           req.SiteName,
		SiteLogo:                                           req.SiteLogo,
		SiteSubtitle:                                       req.SiteSubtitle,
		APIBaseURL:                                         req.APIBaseURL,
		ContactInfo:                                        req.ContactInfo,
		DocURL:                                             req.DocURL,
		HomeContent:                                        req.HomeContent,
		CompactHomeEnabled:                                 req.CompactHomeEnabled,
		HideCcsImportButton:                                req.HideCcsImportButton,
		PurchaseSubscriptionEnabled:                        purchaseEnabled,
		PurchaseSubscriptionURL:                            purchaseURL,
		TableDefaultPageSize:                               req.TableDefaultPageSize,
		TablePageSizeOptions:                               req.TablePageSizeOptions,
		CustomMenuItems:                                    customMenuJSON,
		CustomEndpoints:                                    customEndpointsJSON,
		DefaultConcurrency:                                 req.DefaultConcurrency,
		DefaultBalance:                                     req.DefaultBalance,
		AffiliateRebateRate:                                affiliateRebateRate,
		AffiliateRebateFreezeHours:                         affiliateRebateFreezeHours,
		AffiliateRebateDurationDays:                        affiliateRebateDurationDays,
		AffiliateRebatePerInviteeCap:                       affiliateRebatePerInviteeCap,
		AdminRechargeRebateEnabled:                         adminRechargeRebateEnabled,
		DefaultUserRPMLimit:                                req.DefaultUserRPMLimit,
		DefaultSubscriptions:                               defaultSubscriptions,
		EnableModelFallback:                                req.EnableModelFallback,
		FallbackModelAnthropic:                             req.FallbackModelAnthropic,
		FallbackModelOpenAI:                                req.FallbackModelOpenAI,
		FallbackModelGemini:                                req.FallbackModelGemini,
		FallbackModelAntigravity:                           req.FallbackModelAntigravity,
		EnableIdentityPatch:                                req.EnableIdentityPatch,
		IdentityPatchPrompt:                                req.IdentityPatchPrompt,
		MinClaudeCodeVersion:                               req.MinClaudeCodeVersion,
		MaxClaudeCodeVersion:                               req.MaxClaudeCodeVersion,
		AllowUngroupedKeyScheduling:                        req.AllowUngroupedKeyScheduling,
		SchedulerV2Enabled:                                 boolValueOrDefault(req.SchedulerV2Enabled, previousSettings.SchedulerV2Enabled),
		SchedulerV2CandidateLimit:                          intValueOrDefault(req.SchedulerV2CandidateLimit, previousSettings.SchedulerV2CandidateLimit),
		SchedulerV2ScanLimit:                               intValueOrDefault(req.SchedulerV2ScanLimit, previousSettings.SchedulerV2ScanLimit),
		RequestPriorityAdmissionEnabled:                    boolValueOrDefault(req.RequestPriorityAdmissionEnabled, previousSettings.RequestPriorityAdmissionEnabled),
		RequestPriorityPendingLimitPerInstance:             intValueOrDefault(req.RequestPriorityPendingLimitPerInstance, previousSettings.RequestPriorityPendingLimitPerInstance),
		RequestPriorityPendingMiBPerInstance:               intValueOrDefault(req.RequestPriorityPendingMiBPerInstance, previousSettings.RequestPriorityPendingMiBPerInstance),
		BackendModeEnabled:                                 req.BackendModeEnabled,
		StreamModePerformanceEnabled:                       boolValueOrDefault(req.StreamModePerformanceEnabled, previousSettings.StreamModePerformanceEnabled),
		OpenAIWSModeRouterV2Enabled:                        boolValueOrDefault(req.OpenAIWSModeRouterV2Enabled, previousSettings.OpenAIWSModeRouterV2Enabled),
		OpenAIVisibleOutputTTFTEnabled:                     boolValueOrDefault(req.OpenAIVisibleOutputTTFTEnabled, previousSettings.OpenAIVisibleOutputTTFTEnabled),
		AllowUserViewErrorRequests:                         boolValueOrDefault(req.AllowUserViewErrorRequests, previousSettings.AllowUserViewErrorRequests),
		AllowUserViewUsageDetails:                          boolValueOrDefault(req.AllowUserViewUsageDetails, previousSettings.AllowUserViewUsageDetails),
		OpsMonitoringEnabled:                               boolValueOrDefault(req.OpsMonitoringEnabled, previousSettings.OpsMonitoringEnabled),
		OpsRealtimeMonitoringEnabled:                       boolValueOrDefault(req.OpsRealtimeMonitoringEnabled, previousSettings.OpsRealtimeMonitoringEnabled),
		OpsQueryModeDefault:                                stringSetting(req.OpsQueryModeDefault, previousSettings.OpsQueryModeDefault),
		OpsMetricsIntervalSeconds:                          intValueOrDefault(req.OpsMetricsIntervalSeconds, previousSettings.OpsMetricsIntervalSeconds),
		EnableFingerprintUnification:                       boolValueOrDefault(req.EnableFingerprintUnification, previousSettings.EnableFingerprintUnification),
		EnableMetadataPassthrough:                          boolValueOrDefault(req.EnableMetadataPassthrough, previousSettings.EnableMetadataPassthrough),
		EnableCCHSigning:                                   boolValueOrDefault(req.EnableCCHSigning, previousSettings.EnableCCHSigning),
		EnableClaudeOAuthSystemPromptInjection:             boolValueOrDefault(req.EnableClaudeOAuthSystemPromptInjection, previousSettings.EnableClaudeOAuthSystemPromptInjection),
		ClaudeOAuthSystemPrompt:                            stringSetting(req.ClaudeOAuthSystemPrompt, previousSettings.ClaudeOAuthSystemPrompt),
		ClaudeOAuthSystemPromptBlocks:                      stringSetting(req.ClaudeOAuthSystemPromptBlocks, previousSettings.ClaudeOAuthSystemPromptBlocks),
		EnableAnthropicCacheTTL1hInjection:                 boolValueOrDefault(req.EnableAnthropicCacheTTL1hInjection, previousSettings.EnableAnthropicCacheTTL1hInjection),
		RewriteMessageCacheControl:                         boolValueOrDefault(req.RewriteMessageCacheControl, previousSettings.RewriteMessageCacheControl),
		EnableClientDatelineNormalization:                  boolValueOrDefault(req.EnableClientDatelineNormalization, previousSettings.EnableClientDatelineNormalization),
		AntigravityUserAgentVersion:                        stringSetting(req.AntigravityUserAgentVersion, previousSettings.AntigravityUserAgentVersion),
		OpenAICodexUserAgent:                               stringSetting(req.OpenAICodexUserAgent, previousSettings.OpenAICodexUserAgent),
		OpenAICodexClientVersion:                           stringSetting(req.OpenAICodexClientVersion, previousSettings.OpenAICodexClientVersion),
		OpenAICodexClientVersionSynced:                     previousSettings.OpenAICodexClientVersionSynced,
		OpenAICodexVersionAutoSyncEnabled:                  boolValueOrDefault(req.OpenAICodexVersionAutoSyncEnabled, previousSettings.OpenAICodexVersionAutoSyncEnabled),
		MinCodexVersion:                                    strings.TrimSpace(req.MinCodexVersion),
		MaxCodexVersion:                                    strings.TrimSpace(req.MaxCodexVersion),
		CodexCLIOnlyBlacklist:                              strings.TrimSpace(req.CodexCLIOnlyBlacklist),
		CodexCLIOnlyWhitelist:                              strings.TrimSpace(req.CodexCLIOnlyWhitelist),
		CodexCLIOnlyAllowAppServerClients:                  boolValueOrDefault(req.CodexCLIOnlyAllowAppServerClients, previousSettings.CodexCLIOnlyAllowAppServerClients),
		CodexCLIOnlyEngineFingerprintSignals:               strings.TrimSpace(req.CodexCLIOnlyEngineFingerprintSignals),
		PaymentVisibleMethodAlipaySource:                   trimmedStringValueOrDefault(req.PaymentVisibleMethodAlipaySource, previousSettings.PaymentVisibleMethodAlipaySource),
		PaymentVisibleMethodWxpaySource:                    trimmedStringValueOrDefault(req.PaymentVisibleMethodWxpaySource, previousSettings.PaymentVisibleMethodWxpaySource),
		PaymentVisibleMethodAlipayEnabled:                  boolValueOrDefault(req.PaymentVisibleMethodAlipayEnabled, previousSettings.PaymentVisibleMethodAlipayEnabled),
		PaymentVisibleMethodWxpayEnabled:                   boolValueOrDefault(req.PaymentVisibleMethodWxpayEnabled, previousSettings.PaymentVisibleMethodWxpayEnabled),
		OpenAILowUpstreamRatePriorityEnabled:               boolValueOrDefault(req.OpenAILowUpstreamRatePriorityEnabled, previousSettings.OpenAILowUpstreamRatePriorityEnabled),
		OpenAIOAuthSchedulingRateMultiplier:                float64ValueOrDefault(req.OpenAIOAuthSchedulingRateMultiplier, previousSettings.OpenAIOAuthSchedulingRateMultiplier),
		OpenAIContentSessionBurstBalanceEnabled:            boolValueOrDefault(req.OpenAIContentSessionBurstBalanceEnabled, previousSettings.OpenAIContentSessionBurstBalanceEnabled),
		OpenAIAdvancedSchedulerEnabled:                     boolValueOrDefault(req.OpenAIAdvancedSchedulerEnabled, previousSettings.OpenAIAdvancedSchedulerEnabled),
		OpenAIAdvancedSchedulerStickyWeightedEnabled:       boolValueOrDefault(req.OpenAIAdvancedSchedulerStickyWeightedEnabled, previousSettings.OpenAIAdvancedSchedulerStickyWeightedEnabled),
		OpenAIAdvancedSchedulerSubscriptionPriorityEnabled: boolValueOrDefault(req.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled, previousSettings.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled),
		OpenAIAdvancedSchedulerLBTopK:                      stringSetting(req.OpenAIAdvancedSchedulerLBTopK, previousSettings.OpenAIAdvancedSchedulerLBTopK),
		OpenAIAdvancedSchedulerWeightPriority:              stringSetting(req.OpenAIAdvancedSchedulerWeightPriority, previousSettings.OpenAIAdvancedSchedulerWeightPriority),
		OpenAIAdvancedSchedulerWeightLoad:                  stringSetting(req.OpenAIAdvancedSchedulerWeightLoad, previousSettings.OpenAIAdvancedSchedulerWeightLoad),
		OpenAIAdvancedSchedulerWeightQueue:                 stringSetting(req.OpenAIAdvancedSchedulerWeightQueue, previousSettings.OpenAIAdvancedSchedulerWeightQueue),
		OpenAIAdvancedSchedulerWeightErrorRate:             stringSetting(req.OpenAIAdvancedSchedulerWeightErrorRate, previousSettings.OpenAIAdvancedSchedulerWeightErrorRate),
		OpenAIAdvancedSchedulerWeightTTFT:                  stringSetting(req.OpenAIAdvancedSchedulerWeightTTFT, previousSettings.OpenAIAdvancedSchedulerWeightTTFT),
		OpenAIAdvancedSchedulerWeightReset:                 stringSetting(req.OpenAIAdvancedSchedulerWeightReset, previousSettings.OpenAIAdvancedSchedulerWeightReset),
		OpenAIAdvancedSchedulerWeightQuotaHeadroom:         stringSetting(req.OpenAIAdvancedSchedulerWeightQuotaHeadroom, previousSettings.OpenAIAdvancedSchedulerWeightQuotaHeadroom),
		OpenAIAdvancedSchedulerWeightUpstreamCost:          stringSetting(req.OpenAIAdvancedSchedulerWeightUpstreamCost, previousSettings.OpenAIAdvancedSchedulerWeightUpstreamCost),
		OpenAIAdvancedSchedulerWeightPreviousResponse:      stringSetting(req.OpenAIAdvancedSchedulerWeightPreviousResponse, previousSettings.OpenAIAdvancedSchedulerWeightPreviousResponse),
		OpenAIAdvancedSchedulerWeightSessionSticky:         stringSetting(req.OpenAIAdvancedSchedulerWeightSessionSticky, previousSettings.OpenAIAdvancedSchedulerWeightSessionSticky),
		BalanceLowNotifyEnabled:                            boolValueOrDefault(req.BalanceLowNotifyEnabled, previousSettings.BalanceLowNotifyEnabled),
		BalanceLowNotifyThreshold:                          float64ValueOrDefault(req.BalanceLowNotifyThreshold, previousSettings.BalanceLowNotifyThreshold),
		BalanceLowNotifyRechargeURL:                        stringSetting(req.BalanceLowNotifyRechargeURL, previousSettings.BalanceLowNotifyRechargeURL),
		SubscriptionExpiryNotifyEnabled:                    boolValueOrDefault(req.SubscriptionExpiryNotifyEnabled, previousSettings.SubscriptionExpiryNotifyEnabled),
		AccountQuotaNotifyEnabled:                          boolValueOrDefault(req.AccountQuotaNotifyEnabled, previousSettings.AccountQuotaNotifyEnabled),
		AccountQuotaNotifyEmails:                           notifyEmailEntriesValueOrDefault(req.AccountQuotaNotifyEmails, previousSettings.AccountQuotaNotifyEmails),
		ChannelMonitorEnabled:                              boolValueOrDefault(req.ChannelMonitorEnabled, previousSettings.ChannelMonitorEnabled),
		ChannelMonitorDefaultIntervalSeconds:               intValueOrDefault(req.ChannelMonitorDefaultIntervalSeconds, previousSettings.ChannelMonitorDefaultIntervalSeconds),
		ChannelMonitorLatencyUnit:                          stringSetting(req.ChannelMonitorLatencyUnit, previousSettings.ChannelMonitorLatencyUnit),
		ChannelMonitorPublicShareEnabled:                   boolValueOrDefault(req.ChannelMonitorPublicShareEnabled, previousSettings.ChannelMonitorPublicShareEnabled),
		ChannelMonitorPublicShareRequireAuth:               boolValueOrDefault(req.ChannelMonitorPublicShareRequireAuth, previousSettings.ChannelMonitorPublicShareRequireAuth),
		AvailableChannelsEnabled:                           boolValueOrDefault(req.AvailableChannelsEnabled, previousSettings.AvailableChannelsEnabled),
		SupportChatEnabled:                                 boolValueOrDefault(req.SupportChatEnabled, previousSettings.SupportChatEnabled),
		ModelPlazaEnabled:                                  boolValueOrDefault(req.ModelPlazaEnabled, previousSettings.ModelPlazaEnabled),
		ModelPlazaRequireAuth:                              boolValueOrDefault(req.ModelPlazaRequireAuth, previousSettings.ModelPlazaRequireAuth),
		ModelPlazaAutoPublicModels:                         boolValueOrDefault(req.ModelPlazaAutoPublicModels, previousSettings.ModelPlazaAutoPublicModels),
		ModelPlazaDescription:                              stringSetting(req.ModelPlazaDescription, previousSettings.ModelPlazaDescription),
		MediaStudioEnabled:                                 boolValueOrDefault(req.MediaStudioEnabled, previousSettings.MediaStudioEnabled),
		AffiliateEnabled:                                   boolValueOrDefault(req.AffiliateEnabled, previousSettings.AffiliateEnabled),
		RiskControlEnabled:                                 boolValueOrDefault(req.RiskControlEnabled, previousSettings.RiskControlEnabled),
		CyberSessionBlockEnabled:                           boolValueOrDefault(req.CyberSessionBlockEnabled, previousSettings.CyberSessionBlockEnabled),
		CyberSessionBlockTTLSeconds:                        intValueOrDefault(req.CyberSessionBlockTTLSeconds, previousSettings.CyberSessionBlockTTLSeconds),
	}
}

func buildAuthSourceDefaultSettingsUpdate(prepared *preparedSettingsUpdate) *service.AuthSourceDefaultSettings {
	req := prepared.request
	previousAuthSourceDefaults := prepared.previousAuthSourceDefaults

	// req.AuthSourceXxxPlatformQuotas 为 nil 表示本次请求未包含该 source 的 quota 配置（保留 previousAuthSourceDefaults 中的值）；
	// non-nil（含 empty map）表示整体覆盖：empty map = 清空该 source 的所有 quota 配置。
	return &service.AuthSourceDefaultSettings{
		Email: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultEmailBalance, previousAuthSourceDefaults.Email.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultEmailConcurrency, previousAuthSourceDefaults.Email.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultEmailSubscriptions, previousAuthSourceDefaults.Email.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultEmailGrantOnSignup, previousAuthSourceDefaults.Email.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultEmailGrantOnFirstBind, previousAuthSourceDefaults.Email.GrantOnFirstBind),
			PlatformQuotas:   platformQuotasValueOrDefault(req.AuthSourceEmailPlatformQuotas, previousAuthSourceDefaults.Email.PlatformQuotas),
		},
		LinuxDo: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultLinuxDoBalance, previousAuthSourceDefaults.LinuxDo.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultLinuxDoConcurrency, previousAuthSourceDefaults.LinuxDo.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultLinuxDoSubscriptions, previousAuthSourceDefaults.LinuxDo.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultLinuxDoGrantOnSignup, previousAuthSourceDefaults.LinuxDo.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultLinuxDoGrantOnFirstBind, previousAuthSourceDefaults.LinuxDo.GrantOnFirstBind),
			PlatformQuotas:   platformQuotasValueOrDefault(req.AuthSourceLinuxDoPlatformQuotas, previousAuthSourceDefaults.LinuxDo.PlatformQuotas),
		},
		OIDC: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultOIDCBalance, previousAuthSourceDefaults.OIDC.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultOIDCConcurrency, previousAuthSourceDefaults.OIDC.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultOIDCSubscriptions, previousAuthSourceDefaults.OIDC.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultOIDCGrantOnSignup, previousAuthSourceDefaults.OIDC.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultOIDCGrantOnFirstBind, previousAuthSourceDefaults.OIDC.GrantOnFirstBind),
			PlatformQuotas:   platformQuotasValueOrDefault(req.AuthSourceOIDCPlatformQuotas, previousAuthSourceDefaults.OIDC.PlatformQuotas),
		},
		WeChat: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultWeChatBalance, previousAuthSourceDefaults.WeChat.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultWeChatConcurrency, previousAuthSourceDefaults.WeChat.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultWeChatSubscriptions, previousAuthSourceDefaults.WeChat.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultWeChatGrantOnSignup, previousAuthSourceDefaults.WeChat.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultWeChatGrantOnFirstBind, previousAuthSourceDefaults.WeChat.GrantOnFirstBind),
			PlatformQuotas:   platformQuotasValueOrDefault(req.AuthSourceWeChatPlatformQuotas, previousAuthSourceDefaults.WeChat.PlatformQuotas),
		},
		GitHub: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultGitHubBalance, previousAuthSourceDefaults.GitHub.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultGitHubConcurrency, previousAuthSourceDefaults.GitHub.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultGitHubSubscriptions, previousAuthSourceDefaults.GitHub.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultGitHubGrantOnSignup, previousAuthSourceDefaults.GitHub.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultGitHubGrantOnFirstBind, previousAuthSourceDefaults.GitHub.GrantOnFirstBind),
			PlatformQuotas:   platformQuotasValueOrDefault(req.AuthSourceGitHubPlatformQuotas, previousAuthSourceDefaults.GitHub.PlatformQuotas),
		},
		Google: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultGoogleBalance, previousAuthSourceDefaults.Google.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultGoogleConcurrency, previousAuthSourceDefaults.Google.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultGoogleSubscriptions, previousAuthSourceDefaults.Google.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultGoogleGrantOnSignup, previousAuthSourceDefaults.Google.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultGoogleGrantOnFirstBind, previousAuthSourceDefaults.Google.GrantOnFirstBind),
			PlatformQuotas:   platformQuotasValueOrDefault(req.AuthSourceGooglePlatformQuotas, previousAuthSourceDefaults.Google.PlatformQuotas),
		},
		DingTalk: service.ProviderDefaultGrantSettings{
			Balance:          float64ValueOrDefault(req.AuthSourceDefaultDingTalkBalance, previousAuthSourceDefaults.DingTalk.Balance),
			Concurrency:      intValueOrDefault(req.AuthSourceDefaultDingTalkConcurrency, previousAuthSourceDefaults.DingTalk.Concurrency),
			Subscriptions:    defaultSubscriptionsValueOrDefault(req.AuthSourceDefaultDingTalkSubscriptions, previousAuthSourceDefaults.DingTalk.Subscriptions),
			GrantOnSignup:    boolValueOrDefault(req.AuthSourceDefaultDingTalkGrantOnSignup, previousAuthSourceDefaults.DingTalk.GrantOnSignup),
			GrantOnFirstBind: boolValueOrDefault(req.AuthSourceDefaultDingTalkGrantOnFirstBind, previousAuthSourceDefaults.DingTalk.GrantOnFirstBind),
			PlatformQuotas:   platformQuotasValueOrDefault(req.AuthSourceDingTalkPlatformQuotas, previousAuthSourceDefaults.DingTalk.PlatformQuotas),
		},
		ForceEmailOnThirdPartySignup: boolValueOrDefault(req.ForceEmailOnThirdPartySignup, previousAuthSourceDefaults.ForceEmailOnThirdPartySignup),
	}
}

func trimmedStringValueOrDefault(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return strings.TrimSpace(*value)
}

func notifyEmailEntriesValueOrDefault(
	value *[]dto.NotifyEmailEntry,
	fallback []service.NotifyEmailEntry,
) []service.NotifyEmailEntry {
	if value == nil {
		return fallback
	}
	return dto.NotifyEmailEntriesToService(*value)
}

func buildPaymentConfigUpdate(req *UpdateSettingsRequest) service.UpdatePaymentConfigRequest {
	return service.UpdatePaymentConfigRequest{
		Enabled:                       req.PaymentEnabled,
		MinAmount:                     req.PaymentMinAmount,
		MaxAmount:                     req.PaymentMaxAmount,
		DailyLimit:                    req.PaymentDailyLimit,
		OrderTimeoutMin:               req.PaymentOrderTimeoutMin,
		MaxPendingOrders:              req.PaymentMaxPendingOrders,
		EnabledTypes:                  req.PaymentEnabledTypes,
		BalanceDisabled:               req.PaymentBalanceDisabled,
		BalanceRechargeMultiplier:     req.PaymentBalanceRechargeMultiplier,
		SubscriptionUSDToCNYRate:      req.PaymentSubscriptionUSDToCNYRate,
		RechargeFeeRate:               req.PaymentRechargeFeeRate,
		LoadBalanceStrategy:           req.PaymentLoadBalanceStrat,
		ProductNamePrefix:             req.PaymentProductNamePrefix,
		ProductNameSuffix:             req.PaymentProductNameSuffix,
		HelpImageURL:                  req.PaymentHelpImageURL,
		HelpText:                      req.PaymentHelpText,
		CancelRateLimitEnabled:        req.PaymentCancelRateLimitEnabled,
		CancelRateLimitMax:            req.PaymentCancelRateLimitMax,
		CancelRateLimitWindow:         req.PaymentCancelRateLimitWindow,
		CancelRateLimitUnit:           req.PaymentCancelRateLimitUnit,
		CancelRateLimitMode:           req.PaymentCancelRateLimitMode,
		AlipayForceQRCode:             req.PaymentAlipayForceQRCode,
		AlipayMobilePrecreateDeepLink: req.PaymentAlipayMobilePrecreateDeepLink,
	}
}

// hasPaymentFields reports whether the caller explicitly supplied any setting
// owned by PaymentConfigService.
func hasPaymentFields(req UpdateSettingsRequest) bool {
	return req.PaymentEnabled != nil || req.PaymentMinAmount != nil ||
		req.PaymentMaxAmount != nil || req.PaymentDailyLimit != nil ||
		req.PaymentOrderTimeoutMin != nil || req.PaymentMaxPendingOrders != nil ||
		req.PaymentEnabledTypes != nil || req.PaymentBalanceDisabled != nil ||
		req.PaymentBalanceRechargeMultiplier != nil || req.PaymentSubscriptionUSDToCNYRate != nil ||
		req.PaymentRechargeFeeRate != nil ||
		req.PaymentLoadBalanceStrat != nil || req.PaymentProductNamePrefix != nil ||
		req.PaymentProductNameSuffix != nil || req.PaymentHelpImageURL != nil ||
		req.PaymentHelpText != nil || req.PaymentCancelRateLimitEnabled != nil ||
		req.PaymentCancelRateLimitMax != nil || req.PaymentCancelRateLimitWindow != nil ||
		req.PaymentCancelRateLimitUnit != nil || req.PaymentCancelRateLimitMode != nil ||
		req.PaymentAlipayForceQRCode != nil || req.PaymentAlipayMobilePrecreateDeepLink != nil
}

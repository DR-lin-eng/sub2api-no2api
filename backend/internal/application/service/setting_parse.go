package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/Wei-Shaw/sub2api/internal/shared/ip"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
)

// InitializeDefaultSettings 初始化默认设置
func (s *SettingService) InitializeDefaultSettings(ctx context.Context) error {
	// 检查是否已有设置
	_, err := s.settingRepo.GetValue(ctx, SettingKeyRegistrationEnabled)
	if err == nil {
		// 已有设置，不需要初始化
		return nil
	}
	if !errors.Is(err, ErrSettingNotFound) {
		return fmt.Errorf("check existing settings: %w", err)
	}

	oidcUsePKCEDefault := true
	oidcValidateIDTokenDefault := true
	if s != nil && s.cfg != nil {
		if s.cfg.OIDC.UsePKCEExplicit {
			oidcUsePKCEDefault = s.cfg.OIDC.UsePKCE
		}
		if s.cfg.OIDC.ValidateIDTokenExplicit {
			oidcValidateIDTokenDefault = s.cfg.OIDC.ValidateIDToken
		}
	}
	loginAgreementDocumentsJSON, err := marshalLoginAgreementDocuments(defaultLoginAgreementDocuments())
	if err != nil {
		return err
	}
	// 初始化默认设置。IPv6 的旧配置开关只作为首次启动的迁移默认值；
	// 后续实际开关由管理员设置持久化控制。
	ipv6EgressDefault := "false"
	if s != nil && s.cfg != nil && s.cfg.IPv6Egress.Enabled {
		ipv6EgressDefault = "true"
	}
	// 初始化默认设置
	defaults := map[string]string{
		SettingKeyRegistrationEnabled:                       "true",
		SettingKeyEmailVerifyEnabled:                        "false",
		SettingKeyRegistrationEmailSuffixWhitelist:          "[]",
		SettingKeyPromoCodeEnabled:                          "true", // 默认启用优惠码功能
		SettingKeyLoginAgreementEnabled:                     "false",
		SettingKeyLoginAgreementMode:                        defaultLoginAgreementMode,
		SettingKeyLoginAgreementUpdatedAt:                   defaultLoginAgreementDate,
		SettingKeyLoginAgreementDocuments:                   loginAgreementDocumentsJSON,
		SettingKeyTencentCaptchaEnabled:                     "false",
		SettingKeyTencentCaptchaAppID:                       "",
		SettingKeyTencentCaptchaAppSecretKey:                "",
		SettingKeyTencentCaptchaCloudSecretID:               "",
		SettingKeyTencentCaptchaCloudSecretKey:              "",
		SettingKeyTencentCaptchaRegion:                      TencentCaptchaRegionCN,
		SettingKeyAliyunCaptchaEnabled:                      "false",
		SettingKeyAliyunCaptchaAccessKeyID:                  "",
		SettingKeyAliyunCaptchaAccessKeySecret:              "",
		SettingKeyAliyunCaptchaSceneID:                      "",
		SettingKeyAliyunCaptchaPrefix:                       "",
		SettingKeyAliyunCaptchaRegion:                       AliyunCaptchaRegionCN,
		SettingKeyAPIKeyACLTrustForwardedIP:                 "false",
		SettingKeyClientIPResolutionMode:                    ip.ResolutionModeAutoCompat,
		SettingKeyClientIPTrustedProxies:                    "[]",
		SettingKeySiteName:                                  "Sub2API",
		SettingKeySiteLogo:                                  "",
		SettingKeyPurchaseSubscriptionEnabled:               "false",
		SettingKeyPurchaseSubscriptionURL:                   "",
		SettingKeyTableDefaultPageSize:                      "20",
		SettingKeyTablePageSizeOptions:                      "[10,20,50,100]",
		SettingKeyCustomMenuItems:                           "[]",
		SettingKeyCustomEndpoints:                           "[]",
		SettingKeyWeChatConnectEnabled:                      "false",
		SettingKeyWeChatConnectAppID:                        "",
		SettingKeyWeChatConnectAppSecret:                    "",
		SettingKeyWeChatConnectOpenAppID:                    "",
		SettingKeyWeChatConnectOpenAppSecret:                "",
		SettingKeyWeChatConnectMPAppID:                      "",
		SettingKeyWeChatConnectMPAppSecret:                  "",
		SettingKeyWeChatConnectMobileAppID:                  "",
		SettingKeyWeChatConnectMobileAppSecret:              "",
		SettingKeyWeChatConnectOpenEnabled:                  "false",
		SettingKeyWeChatConnectMPEnabled:                    "false",
		SettingKeyWeChatConnectMobileEnabled:                "false",
		SettingKeyWeChatConnectMode:                         "open",
		SettingKeyWeChatConnectScopes:                       "snsapi_login",
		SettingKeyWeChatConnectRedirectURL:                  "",
		SettingKeyWeChatConnectFrontendRedirectURL:          defaultWeChatConnectFrontend,
		SettingKeyGitHubOAuthEnabled:                        "false",
		SettingKeyGitHubOAuthClientID:                       "",
		SettingKeyGitHubOAuthClientSecret:                   "",
		SettingKeyGitHubOAuthRedirectURL:                    "",
		SettingKeyGitHubOAuthFrontendRedirectURL:            defaultGitHubOAuthFrontend,
		SettingKeyGoogleOAuthEnabled:                        "false",
		SettingKeyGoogleOAuthClientID:                       "",
		SettingKeyGoogleOAuthClientSecret:                   "",
		SettingKeyGoogleOAuthRedirectURL:                    "",
		SettingKeyGoogleOAuthFrontendRedirectURL:            defaultGoogleOAuthFrontend,
		SettingKeyOIDCConnectEnabled:                        "false",
		SettingKeyOIDCConnectProviderName:                   "OIDC",
		SettingKeyOIDCConnectClientID:                       "",
		SettingKeyOIDCConnectClientSecret:                   "",
		SettingKeyOIDCConnectIssuerURL:                      "",
		SettingKeyOIDCConnectDiscoveryURL:                   "",
		SettingKeyOIDCConnectAuthorizeURL:                   "",
		SettingKeyOIDCConnectTokenURL:                       "",
		SettingKeyOIDCConnectUserInfoURL:                    "",
		SettingKeyOIDCConnectJWKSURL:                        "",
		SettingKeyOIDCConnectScopes:                         "openid email profile",
		SettingKeyOIDCConnectRedirectURL:                    "",
		SettingKeyOIDCConnectFrontendRedirectURL:            "/auth/oidc/callback",
		SettingKeyOIDCConnectTokenAuthMethod:                "client_secret_post",
		SettingKeyOIDCConnectUsePKCE:                        strconv.FormatBool(oidcUsePKCEDefault),
		SettingKeyOIDCConnectValidateIDToken:                strconv.FormatBool(oidcValidateIDTokenDefault),
		SettingKeyOIDCConnectAllowedSigningAlgs:             "RS256,ES256,PS256",
		SettingKeyOIDCConnectClockSkewSeconds:               "120",
		SettingKeyOIDCConnectRequireEmailVerified:           "false",
		SettingKeyOIDCConnectUserInfoEmailPath:              "",
		SettingKeyOIDCConnectUserInfoIDPath:                 "",
		SettingKeyOIDCConnectUserInfoUsernamePath:           "",
		SettingKeyDefaultConcurrency:                        strconv.Itoa(s.cfg.Default.UserConcurrency),
		SettingKeyDefaultBalance:                            strconv.FormatFloat(s.cfg.Default.UserBalance, 'f', 8, 64),
		SettingKeyAffiliateRebateRate:                       strconv.FormatFloat(AffiliateRebateRateDefault, 'f', 8, 64),
		SettingKeyAffiliateRebateFreezeHours:                strconv.Itoa(AffiliateRebateFreezeHoursDefault),
		SettingKeyAffiliateRebateDurationDays:               strconv.Itoa(AffiliateRebateDurationDaysDefault),
		SettingKeyAffiliateRebatePerInviteeCap:              strconv.FormatFloat(AffiliateRebatePerInviteeCapDefault, 'f', 2, 64),
		SettingKeyDefaultUserRPMLimit:                       "0",
		SettingKeyDefaultSubscriptions:                      "[]",
		SettingKeyAuthSourceDefaultEmailBalance:             "0",
		SettingKeyAuthSourceDefaultEmailConcurrency:         "5",
		SettingKeyAuthSourceDefaultEmailSubscriptions:       "[]",
		SettingKeyAuthSourceDefaultEmailGrantOnSignup:       "false",
		SettingKeyAuthSourceDefaultEmailGrantOnFirstBind:    "false",
		SettingKeyAuthSourceDefaultLinuxDoBalance:           "0",
		SettingKeyAuthSourceDefaultLinuxDoConcurrency:       "5",
		SettingKeyAuthSourceDefaultLinuxDoSubscriptions:     "[]",
		SettingKeyAuthSourceDefaultLinuxDoGrantOnSignup:     "false",
		SettingKeyAuthSourceDefaultLinuxDoGrantOnFirstBind:  "false",
		SettingKeyAuthSourceDefaultOIDCBalance:              "0",
		SettingKeyAuthSourceDefaultOIDCConcurrency:          "5",
		SettingKeyAuthSourceDefaultOIDCSubscriptions:        "[]",
		SettingKeyAuthSourceDefaultOIDCGrantOnSignup:        "false",
		SettingKeyAuthSourceDefaultOIDCGrantOnFirstBind:     "false",
		SettingKeyAuthSourceDefaultWeChatBalance:            "0",
		SettingKeyAuthSourceDefaultWeChatConcurrency:        "5",
		SettingKeyAuthSourceDefaultWeChatSubscriptions:      "[]",
		SettingKeyAuthSourceDefaultWeChatGrantOnSignup:      "false",
		SettingKeyAuthSourceDefaultWeChatGrantOnFirstBind:   "false",
		SettingKeyAuthSourceDefaultGitHubBalance:            "0",
		SettingKeyAuthSourceDefaultGitHubConcurrency:        "5",
		SettingKeyAuthSourceDefaultGitHubSubscriptions:      "[]",
		SettingKeyAuthSourceDefaultGitHubGrantOnSignup:      "false",
		SettingKeyAuthSourceDefaultGitHubGrantOnFirstBind:   "false",
		SettingKeyAuthSourceDefaultGoogleBalance:            "0",
		SettingKeyAuthSourceDefaultGoogleConcurrency:        "5",
		SettingKeyAuthSourceDefaultGoogleSubscriptions:      "[]",
		SettingKeyAuthSourceDefaultGoogleGrantOnSignup:      "false",
		SettingKeyAuthSourceDefaultGoogleGrantOnFirstBind:   "false",
		SettingKeyAuthSourceDefaultDingTalkBalance:          "0",
		SettingKeyAuthSourceDefaultDingTalkConcurrency:      "5",
		SettingKeyAuthSourceDefaultDingTalkSubscriptions:    "[]",
		SettingKeyAuthSourceDefaultDingTalkGrantOnSignup:    "false",
		SettingKeyAuthSourceDefaultDingTalkGrantOnFirstBind: "false",
		SettingKeyForceEmailOnThirdPartySignup:              "false",
		SettingKeySMTPPort:                                  "587",
		SettingKeySMTPUseTLS:                                "false",
		// Model fallback defaults
		SettingKeyEnableModelFallback:      "false",
		SettingKeyFallbackModelAnthropic:   "claude-3-5-sonnet-20241022",
		SettingKeyFallbackModelOpenAI:      "gpt-4o",
		SettingKeyFallbackModelGemini:      "gemini-2.5-pro",
		SettingKeyFallbackModelAntigravity: "gemini-2.5-pro",
		// Identity patch defaults
		SettingKeyEnableIdentityPatch: "true",
		SettingKeyIdentityPatchPrompt: "",

		// Ops monitoring defaults (vNext)
		SettingKeyOpsMonitoringEnabled:         "true",
		SettingKeyOpsRealtimeMonitoringEnabled: "true",
		SettingKeyOpsQueryModeDefault:          "auto",
		SettingKeyOpsMetricsIntervalSeconds:    "60",

		// Channel monitor defaults (enabled, 60s)
		SettingKeyChannelMonitorEnabled:                "true",
		SettingKeyChannelMonitorDefaultIntervalSeconds: "60",
		SettingKeyChannelMonitorLatencyUnit:            "ms",
		SettingKeyChannelMonitorPublicShareEnabled:     "false",
		SettingKeyChannelMonitorPublicShareRequireAuth: "false",

		// Available channels feature (default disabled; opt-in)
		SettingKeyAvailableChannelsEnabled: "false",

		// Support chat feature (default disabled; explicit opt-in)
		SettingKeySupportChatEnabled:       "false",
		SettingKeySupportChatRetentionDays: "0",

		// Model plaza feature (default disabled; public access when enabled)
		SettingKeyModelPlazaEnabled:          "false",
		SettingKeyModelPlazaRequireAuth:      "false",
		SettingKeyModelPlazaAutoPublicModels: "false",
		SettingKeyModelPlazaDescription:      "",

		// Media Studio feature (default disabled; opt-in)
		SettingKeyMediaStudioEnabled: "false",

		// IPv6 egress master switch (defaults to the legacy deployment value)
		SettingKeyIPv6EgressUIEnabled: ipv6EgressDefault,

		// Affiliate (邀请返利) feature (default disabled; opt-in)
		SettingKeyAffiliateEnabled:              "false",
		SettingKeyAffiliateAdminRechargeEnabled: strconv.FormatBool(AdminRechargeRebateEnabledDefault),

		// 风控中心功能（默认关闭，显式启用）
		SettingKeyRiskControlEnabled: "false",

		// cyber 会话屏蔽（默认关闭，TTL 默认 3600s）
		SettingKeyCyberSessionBlockEnabled:    "false",
		SettingKeyCyberSessionBlockTTLSeconds: "3600",

		// Claude Code version check (default: empty = disabled)
		SettingKeyMinClaudeCodeVersion: "",
		SettingKeyMaxClaudeCodeVersion: "",

		// codex_cli_only 加固（默认：版本不检查、名单空、默认种子指纹信号）
		SettingKeyMinCodexVersion:                      "",
		SettingKeyMaxCodexVersion:                      "",
		SettingKeyCodexCLIOnlyBlacklist:                "",
		SettingKeyCodexCLIOnlyWhitelist:                "",
		SettingKeyCodexCLIOnlyAllowAppServerClients:    "false",
		SettingKeyCodexCLIOnlyEngineFingerprintSignals: openai.DefaultEngineFingerprintSignalsJSON(),

		// 分组隔离（默认不允许未分组 Key 调度）
		SettingKeyAllowUngroupedKeyScheduling:                        "false",
		SettingKeyOpenAILowUpstreamRatePriorityEnabled:               "false",
		SettingKeyOpenAIOAuthSchedulingRateMultiplier:                "1",
		SettingKeySchedulerV2Enabled:                                 "false",
		SettingKeySchedulerV2CandidateLimit:                          strconv.Itoa(DefaultSchedulerCandidateFetchLimit),
		SettingKeySchedulerV2ScanLimit:                               strconv.Itoa(DefaultSchedulerCandidateScanLimit),
		SettingKeyRequestPriorityAdmissionEnabled:                    "false",
		SettingKeyRequestPriorityPendingLimitPerInstance:             strconv.Itoa(DefaultRequestPriorityPendingLimitPerInstance),
		SettingKeyRequestPriorityPendingMiBPerInstance:               strconv.Itoa(DefaultRequestPriorityPendingMiBPerInstance),
		SettingKeyOpenAIWSModeRouterV2Enabled:                        strconv.FormatBool(s.defaultOpenAIWSModeRouterV2Enabled()),
		SettingKeyOpenAIVisibleOutputTTFTEnabled:                     "true",
		SettingKeyEnableAnthropicCacheTTL1hInjection:                 "false",
		SettingKeyRewriteMessageCacheControl:                         strconv.FormatBool(s.defaultRewriteMessageCacheControl()),
		SettingKeyEnableClientDatelineNormalization:                  "true",
		SettingKeyAntigravityUserAgentVersion:                        "",
		SettingKeyOpenAICodexUserAgent:                               "",
		SettingKeyOpenAICodexClientVersion:                           "",
		SettingKeyOpenAICodexClientVersionSynced:                     "",
		SettingKeyOpenAICodexVersionAutoSyncEnabled:                  "true",
		SettingPaymentVisibleMethodAlipaySource:                      "",
		SettingPaymentVisibleMethodWxpaySource:                       "",
		SettingPaymentVisibleMethodAlipayEnabled:                     "false",
		SettingPaymentVisibleMethodWxpayEnabled:                      "false",
		SettingKeyOpenAIContentSessionBurstBalanceEnabled:            "false",
		openAIAdvancedSchedulerSettingKey:                            "false",
		SettingKeyOpenAIAdvancedSchedulerStickyWeightedEnabled:       "false",
		SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled: "false",
		SettingKeyOpenAIAdvancedSchedulerLBTopK:                      "",
		SettingKeyOpenAIAdvancedSchedulerWeightPriority:              "",
		SettingKeyOpenAIAdvancedSchedulerWeightLoad:                  "",
		SettingKeyOpenAIAdvancedSchedulerWeightQueue:                 "",
		SettingKeyOpenAIAdvancedSchedulerWeightErrorRate:             "",
		SettingKeyOpenAIAdvancedSchedulerWeightTTFT:                  "",
		SettingKeyOpenAIAdvancedSchedulerWeightReset:                 "",
		SettingKeyOpenAIAdvancedSchedulerWeightQuotaHeadroom:         "",
		SettingKeyOpenAIAdvancedSchedulerWeightUpstreamCost:          "",
		SettingKeyOpenAIAdvancedSchedulerWeightPreviousResponse:      "",
		SettingKeyOpenAIAdvancedSchedulerWeightSessionSticky:         "",

		SettingKeyAllowUserViewErrorRequests: "false",
		SettingKeyAllowUserViewUsageDetails:  "false",
	}

	return s.settingRepo.SetMultiple(ctx, defaults)
}

// parseSettings 解析设置到结构体
func (s *SettingService) parseSettings(settings map[string]string) *SystemSettings {
	result := s.parseCoreSystemSettings(settings)
	s.applyLinuxDoConnectSettings(result, settings)
	s.applyDingTalkConnectSettings(result, settings)
	s.applyOIDCConnectSettings(result, settings)
	s.applyEmailOAuthSettings(result, settings)
	s.applyWeChatConnectSettings(result, settings)
	s.applyFeatureSettings(result, settings)
	s.applyGatewaySettings(result, settings)
	s.applyNotificationAndVisibilitySettings(result, settings)
	return result
}

func parseSchedulerV2Limits(candidateRaw, scanRaw string) (int, int) {
	candidateLimit := DefaultSchedulerCandidateFetchLimit
	scanLimit := DefaultSchedulerCandidateScanLimit
	if parsed, err := strconv.Atoi(strings.TrimSpace(candidateRaw)); err == nil {
		candidateLimit = parsed
	}
	if parsed, err := strconv.Atoi(strings.TrimSpace(scanRaw)); err == nil {
		scanLimit = parsed
	}
	if ValidateSchedulerV2Limits(candidateLimit, scanLimit) != nil {
		return DefaultSchedulerCandidateFetchLimit, DefaultSchedulerCandidateScanLimit
	}
	return candidateLimit, scanLimit
}

func clampAffiliateRebateRate(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return AffiliateRebateRateDefault
	}
	if value < AffiliateRebateRateMin {
		return AffiliateRebateRateMin
	}
	if value > AffiliateRebateRateMax {
		return AffiliateRebateRateMax
	}
	return value
}

func isFalseSettingValue(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "false", "0", "off", "disabled":
		return true
	default:
		return false
	}
}

func normalizeVisibleMethodSettingSource(method, source string, enabled bool) (string, error) {
	_ = enabled
	source = strings.TrimSpace(source)
	if source == "" {
		return "", nil
	}

	normalized := NormalizeVisibleMethodSource(method, source)
	if normalized == "" {
		return "", infraerrors.BadRequest(
			"INVALID_PAYMENT_VISIBLE_METHOD_SOURCE",
			fmt.Sprintf("%s source must be one of the supported payment providers", method),
		)
	}
	return normalized, nil
}

func (s *SettingService) openAIAdvancedSchedulerEffectiveLBTopK() string {
	if s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIWS.LBTopK > 0 {
		return strconv.Itoa(s.cfg.Gateway.OpenAIWS.LBTopK)
	}
	return "7"
}

func (s *SettingService) openAIAdvancedSchedulerEffectiveWeights() config.GatewayOpenAIWSSchedulerScoreWeights {
	defaults := config.GatewayOpenAIWSSchedulerScoreWeights{
		Priority:         1.0,
		Load:             1.0,
		Queue:            0.7,
		ErrorRate:        0.8,
		TTFT:             0.5,
		Reset:            0.0,
		QuotaHeadroom:    0.0,
		UpstreamCost:     0.0,
		PreviousResponse: 5.0,
		SessionSticky:    3.0,
	}
	if s == nil || s.cfg == nil {
		return defaults
	}

	weights := s.cfg.Gateway.OpenAIWS.SchedulerScoreWeights
	if !weights.IsValid() {
		return defaults
	}
	return weights
}

func formatOpenAIAdvancedSchedulerFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func (s *SettingService) normalizeOpenAIAdvancedSchedulerOverrides(settings *SystemSettings) error {
	if rate := settings.OpenAIOAuthSchedulingRateMultiplier; rate < 0 || math.IsNaN(rate) || math.IsInf(rate, 0) {
		return infraerrors.BadRequest("INVALID_OPENAI_OAUTH_SCHEDULING_RATE_MULTIPLIER", "OpenAI OAuth scheduling rate multiplier must be a finite non-negative number")
	}

	lbTopK, err := normalizeOptionalPositiveIntString(settings.OpenAIAdvancedSchedulerLBTopK)
	if err != nil {
		return infraerrors.BadRequest("INVALID_OPENAI_ADVANCED_SCHEDULER_LB_TOP_K", "openai advanced scheduler TopK must be a positive integer or empty")
	}
	settings.OpenAIAdvancedSchedulerLBTopK = lbTopK

	weights := []*string{
		&settings.OpenAIAdvancedSchedulerWeightPriority,
		&settings.OpenAIAdvancedSchedulerWeightLoad,
		&settings.OpenAIAdvancedSchedulerWeightQueue,
		&settings.OpenAIAdvancedSchedulerWeightErrorRate,
		&settings.OpenAIAdvancedSchedulerWeightTTFT,
		&settings.OpenAIAdvancedSchedulerWeightReset,
		&settings.OpenAIAdvancedSchedulerWeightQuotaHeadroom,
		&settings.OpenAIAdvancedSchedulerWeightUpstreamCost,
		&settings.OpenAIAdvancedSchedulerWeightPreviousResponse,
		&settings.OpenAIAdvancedSchedulerWeightSessionSticky,
	}
	for _, target := range weights {
		normalized, err := normalizeOptionalNonNegativeFloatString(*target)
		if err != nil {
			return infraerrors.BadRequest("INVALID_OPENAI_ADVANCED_SCHEDULER_WEIGHT", "openai advanced scheduler weights must be non-negative numbers or empty")
		}
		*target = normalized
	}

	// 与 config.Validate 的 "scheduler_score_weights must not all be zero" 保持一致：
	// 覆盖值（空则回退到生效的配置值）叠加后的基础权重和不允许为 0，
	// 否则调度会静默退化为 TopK 内均匀随机。
	effective := s.openAIAdvancedSchedulerEffectiveWeights()
	resolved := config.GatewayOpenAIWSSchedulerScoreWeights{
		Priority:         resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightPriority, effective.Priority),
		Load:             resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightLoad, effective.Load),
		Queue:            resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightQueue, effective.Queue),
		ErrorRate:        resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightErrorRate, effective.ErrorRate),
		TTFT:             resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightTTFT, effective.TTFT),
		Reset:            resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightReset, effective.Reset),
		QuotaHeadroom:    resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightQuotaHeadroom, effective.QuotaHeadroom),
		UpstreamCost:     resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightUpstreamCost, effective.UpstreamCost),
		PreviousResponse: resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightPreviousResponse, effective.PreviousResponse),
		SessionSticky:    resolveOpenAIAdvancedSchedulerWeight(settings.OpenAIAdvancedSchedulerWeightSessionSticky, effective.SessionSticky),
	}
	if !resolved.IsValid() {
		return infraerrors.BadRequest("INVALID_OPENAI_ADVANCED_SCHEDULER_WEIGHT", "openai advanced scheduler weights must have finite non-zero base and total sums")
	}
	return nil
}

func parseOpenAIOAuthSchedulingRateMultiplier(raw string) float64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return defaultOpenAIOAuthSchedulingRateMultiplier
	}
	return value
}

// resolveOpenAIAdvancedSchedulerWeight 返回覆盖值（已归一化的非空字符串），空则回退默认值。
func resolveOpenAIAdvancedSchedulerWeight(normalized string, fallback float64) float64 {
	if normalized == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return fallback
	}
	return value
}

func normalizeOptionalPositiveIntString(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return "", fmt.Errorf("invalid positive integer")
	}
	return strconv.Itoa(value), nil
}

func normalizeOptionalNonNegativeFloatString(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return "", fmt.Errorf("invalid non-negative float")
	}
	return strconv.FormatFloat(value, 'f', -1, 64), nil
}

func parseDefaultSubscriptions(raw string) []DefaultSubscriptionSetting {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	var items []DefaultSubscriptionSetting
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}

	normalized := make([]DefaultSubscriptionSetting, 0, len(items))
	for _, item := range items {
		if item.GroupID <= 0 || item.ValidityDays <= 0 {
			continue
		}
		if item.ValidityDays > MaxValidityDays {
			item.ValidityDays = MaxValidityDays
		}
		normalized = append(normalized, item)
	}

	return normalized
}

func parseProviderDefaultGrantSettings(settings map[string]string, keys authSourceDefaultKeySet) ProviderDefaultGrantSettings {
	result := ProviderDefaultGrantSettings{
		Balance:          defaultAuthSourceBalance,
		Concurrency:      defaultAuthSourceConcurrency,
		Subscriptions:    []DefaultSubscriptionSetting{},
		GrantOnSignup:    false,
		GrantOnFirstBind: false,
	}

	if v, err := strconv.ParseFloat(strings.TrimSpace(settings[keys.balance]), 64); err == nil {
		result.Balance = v
	}
	if v, err := strconv.Atoi(strings.TrimSpace(settings[keys.concurrency])); err == nil {
		result.Concurrency = v
	}
	if items := parseDefaultSubscriptions(settings[keys.subscriptions]); items != nil {
		result.Subscriptions = items
	}
	if raw, ok := settings[keys.grantOnSignup]; ok {
		result.GrantOnSignup = raw == "true"
	}
	if raw, ok := settings[keys.grantOnFirstBind]; ok {
		result.GrantOnFirstBind = raw == "true"
	}

	if raw := settings[keys.platformQuotas]; raw != "" {
		parsed := map[string]*DefaultPlatformQuotaSetting{}
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			slog.Warn("[Setting] parseProviderDefaultGrantSettings: unmarshal auth source platform quotas failed", "source", keys.source, "error", err)
		} else {
			result.PlatformQuotas = parsed
		}
	}

	return result
}

func writeProviderDefaultGrantUpdates(updates map[string]string, keys authSourceDefaultKeySet, settings ProviderDefaultGrantSettings) {
	updates[keys.balance] = strconv.FormatFloat(settings.Balance, 'f', 8, 64)
	updates[keys.concurrency] = strconv.Itoa(settings.Concurrency)

	subscriptions := settings.Subscriptions
	if subscriptions == nil {
		subscriptions = []DefaultSubscriptionSetting{}
	}
	raw, err := json.Marshal(subscriptions)
	if err != nil {
		raw = []byte("[]")
	}
	updates[keys.subscriptions] = string(raw)
	updates[keys.grantOnSignup] = strconv.FormatBool(settings.GrantOnSignup)
	updates[keys.grantOnFirstBind] = strconv.FormatBool(settings.GrantOnFirstBind)

	// auth source platform quota：整体替换语义。
	// nil = 请求未携带该字段，跳过写入以保留既有配置（与系统层 buildSystemSettingsUpdates 的
	// DefaultPlatformQuotas nil 守卫一致）；非 nil（含空 map）才整体替换。二者语义不可混同。
	if keys.platformQuotas != "" && settings.PlatformQuotas != nil {
		blob, err := json.Marshal(settings.PlatformQuotas)
		if err != nil {
			blob = []byte("{}")
		}
		updates[keys.platformQuotas] = string(blob)
	}
}

func mergeProviderDefaultGrantSettings(globalDefaults ProviderDefaultGrantSettings, providerDefaults ProviderDefaultGrantSettings) ProviderDefaultGrantSettings {
	result := ProviderDefaultGrantSettings{
		Balance:          globalDefaults.Balance,
		Concurrency:      globalDefaults.Concurrency,
		Subscriptions:    append([]DefaultSubscriptionSetting(nil), globalDefaults.Subscriptions...),
		GrantOnSignup:    providerDefaults.GrantOnSignup,
		GrantOnFirstBind: providerDefaults.GrantOnFirstBind,
	}

	// 注意：不能把 parse 默认值 (defaultAuthSourceBalance / defaultAuthSourceConcurrency)
	// 当作"未配置"哨兵——admin 完全有权显式设成相同的值，那时仍应覆盖 globalDefaults。
	// 旧实现的 `!= defaultAuthSourceConcurrency` 会把 admin 设的 5 与 fallback 5 混淆，
	// 导致渠道发放退回到全局默认（如 1），表现为"管理员设 5、新用户实际拿 1"。
	if providerDefaults.Balance >= 0 {
		result.Balance = providerDefaults.Balance
	}
	if providerDefaults.Concurrency > 0 {
		result.Concurrency = providerDefaults.Concurrency
	}
	if len(providerDefaults.Subscriptions) > 0 {
		result.Subscriptions = append([]DefaultSubscriptionSetting(nil), providerDefaults.Subscriptions...)
	}

	return result
}

func parseTablePreferences(defaultPageSizeRaw, optionsRaw string) (int, []int) {
	defaultPageSize := 20
	if v, err := strconv.Atoi(strings.TrimSpace(defaultPageSizeRaw)); err == nil {
		defaultPageSize = v
	}

	var options []int
	if strings.TrimSpace(optionsRaw) != "" {
		_ = json.Unmarshal([]byte(optionsRaw), &options)
	}

	return normalizeTablePreferences(defaultPageSize, options)
}

func normalizeTablePreferences(defaultPageSize int, options []int) (int, []int) {
	const minPageSize = 5
	const maxPageSize = 1000
	const fallbackPageSize = 20

	seen := make(map[int]struct{}, len(options))
	normalizedOptions := make([]int, 0, len(options))
	for _, option := range options {
		if option < minPageSize || option > maxPageSize {
			continue
		}
		if _, ok := seen[option]; ok {
			continue
		}
		seen[option] = struct{}{}
		normalizedOptions = append(normalizedOptions, option)
	}
	sort.Ints(normalizedOptions)

	if defaultPageSize < minPageSize || defaultPageSize > maxPageSize {
		defaultPageSize = fallbackPageSize
	}

	if len(normalizedOptions) == 0 {
		normalizedOptions = []int{10, 20, 50}
	}

	return defaultPageSize, normalizedOptions
}

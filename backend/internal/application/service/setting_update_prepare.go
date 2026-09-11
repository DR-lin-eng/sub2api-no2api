package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/Wei-Shaw/sub2api/internal/shared/ip"
)

func (s *SettingService) prepareSystemSettingsUpdate(ctx context.Context, settings *SystemSettings) (string, error) {
	// Preserve compatibility with older internal callers that construct a
	// zero-value SystemSettings while updating an unrelated setting.
	if settings.SchedulerV2CandidateLimit == 0 && settings.SchedulerV2ScanLimit == 0 {
		settings.SchedulerV2CandidateLimit = DefaultSchedulerCandidateFetchLimit
		settings.SchedulerV2ScanLimit = DefaultSchedulerCandidateScanLimit
	}
	if err := ValidateSchedulerV2Limits(settings.SchedulerV2CandidateLimit, settings.SchedulerV2ScanLimit); err != nil {
		return "", infraerrors.BadRequest("INVALID_SCHEDULER_V2_LIMITS", err.Error())
	}
	requestPrioritySettings := normalizeRequestPriorityAdmissionSettings(RequestPriorityAdmissionSettings{
		Enabled:                 settings.RequestPriorityAdmissionEnabled,
		PendingLimitPerInstance: settings.RequestPriorityPendingLimitPerInstance,
		PendingMiBPerInstance:   settings.RequestPriorityPendingMiBPerInstance,
	})
	settings.RequestPriorityPendingLimitPerInstance = requestPrioritySettings.PendingLimitPerInstance
	settings.RequestPriorityPendingMiBPerInstance = requestPrioritySettings.PendingMiBPerInstance
	if err := s.validateDefaultSubscriptionGroups(ctx, settings.DefaultSubscriptions); err != nil {
		return "", err
	}
	normalizedWhitelist, err := NormalizeRegistrationEmailSuffixWhitelist(settings.RegistrationEmailSuffixWhitelist)
	if err != nil {
		return "", infraerrors.BadRequest("INVALID_REGISTRATION_EMAIL_SUFFIX_WHITELIST", err.Error())
	}
	if normalizedWhitelist == nil {
		normalizedWhitelist = []string{}
	}
	settings.RegistrationEmailSuffixWhitelist = normalizedWhitelist
	alipaySource, err := normalizeVisibleMethodSettingSource("alipay", settings.PaymentVisibleMethodAlipaySource, settings.PaymentVisibleMethodAlipayEnabled)
	if err != nil {
		return "", err
	}
	wxpaySource, err := normalizeVisibleMethodSettingSource("wxpay", settings.PaymentVisibleMethodWxpaySource, settings.PaymentVisibleMethodWxpayEnabled)
	if err != nil {
		return "", err
	}
	if err := s.normalizeOpenAIAdvancedSchedulerOverrides(settings); err != nil {
		return "", err
	}
	if settings.OpenAISessionIDRateLimitPerMinute < 0 || settings.OpenAISessionIDRateLimitPerMinute > 1000000 {
		return "", infraerrors.BadRequest("INVALID_OPENAI_SESSION_ID_RATE_LIMIT", "OpenAI session ID rate limit must be between 0 and 1000000 per minute")
	}
	if strings.TrimSpace(settings.ClientIPResolutionMode) == "" {
		if s.clientIPResolver != nil {
			settings.ClientIPResolutionMode, settings.ClientIPTrustedProxies = s.clientIPResolver.CurrentConfiguration()
		} else {
			settings.ClientIPResolutionMode = ip.ResolutionModeAutoCompat
		}
	}
	settings.ClientIPResolutionMode = strings.TrimSpace(settings.ClientIPResolutionMode)
	if err := ip.ValidateResolutionMode(settings.ClientIPResolutionMode); err != nil {
		return "", infraerrors.BadRequest("INVALID_CLIENT_IP_RESOLUTION_MODE", err.Error())
	}
	if settings.ClientIPTrustedProxies == nil {
		settings.ClientIPTrustedProxies = make([]string, 0)
	}
	normalizedClientIPTrustedProxies, err := ip.NormalizeTrustedProxies(settings.ClientIPTrustedProxies)
	if err != nil {
		return "", infraerrors.BadRequest("INVALID_CLIENT_IP_TRUSTED_PROXIES", err.Error())
	}
	settings.ClientIPTrustedProxies = normalizedClientIPTrustedProxies
	clientIPTrustedProxiesJSON, err := json.Marshal(settings.ClientIPTrustedProxies)
	if err != nil {
		return "", fmt.Errorf("marshal client IP trusted proxies: %w", err)
	}
	settings.APIKeyACLTrustForwardedIP = settings.ClientIPResolutionMode != ip.ResolutionModeDirect
	settings.PaymentVisibleMethodAlipaySource = alipaySource
	settings.PaymentVisibleMethodWxpaySource = wxpaySource
	settings.WeChatConnectAppID = strings.TrimSpace(settings.WeChatConnectAppID)
	settings.WeChatConnectAppSecret = strings.TrimSpace(settings.WeChatConnectAppSecret)
	settings.WeChatConnectOpenAppID = strings.TrimSpace(firstNonEmpty(settings.WeChatConnectOpenAppID, settings.WeChatConnectAppID))
	settings.WeChatConnectOpenAppSecret = strings.TrimSpace(firstNonEmpty(settings.WeChatConnectOpenAppSecret, settings.WeChatConnectAppSecret))
	settings.WeChatConnectMPAppID = strings.TrimSpace(firstNonEmpty(settings.WeChatConnectMPAppID, settings.WeChatConnectAppID))
	settings.WeChatConnectMPAppSecret = strings.TrimSpace(firstNonEmpty(settings.WeChatConnectMPAppSecret, settings.WeChatConnectAppSecret))
	settings.WeChatConnectMobileAppID = strings.TrimSpace(firstNonEmpty(settings.WeChatConnectMobileAppID, settings.WeChatConnectAppID))
	settings.WeChatConnectMobileAppSecret = strings.TrimSpace(firstNonEmpty(settings.WeChatConnectMobileAppSecret, settings.WeChatConnectAppSecret))
	settings.WeChatConnectMode = normalizeWeChatConnectStoredMode(
		settings.WeChatConnectOpenEnabled,
		settings.WeChatConnectMPEnabled,
		settings.WeChatConnectMobileEnabled,
		settings.WeChatConnectMode,
	)
	settings.WeChatConnectScopes = normalizeWeChatConnectScopeSetting(settings.WeChatConnectScopes, settings.WeChatConnectMode)
	settings.WeChatConnectRedirectURL = strings.TrimSpace(settings.WeChatConnectRedirectURL)
	settings.WeChatConnectFrontendRedirectURL = strings.TrimSpace(settings.WeChatConnectFrontendRedirectURL)
	if settings.WeChatConnectFrontendRedirectURL == "" {
		settings.WeChatConnectFrontendRedirectURL = defaultWeChatConnectFrontend
	}
	settings.GitHubOAuthRedirectURL = strings.TrimSpace(settings.GitHubOAuthRedirectURL)
	settings.GitHubOAuthFrontendRedirectURL = strings.TrimSpace(settings.GitHubOAuthFrontendRedirectURL)
	if settings.GitHubOAuthFrontendRedirectURL == "" {
		settings.GitHubOAuthFrontendRedirectURL = defaultGitHubOAuthFrontend
	}
	settings.GoogleOAuthRedirectURL = strings.TrimSpace(settings.GoogleOAuthRedirectURL)
	settings.GoogleOAuthFrontendRedirectURL = strings.TrimSpace(settings.GoogleOAuthFrontendRedirectURL)
	if settings.GoogleOAuthFrontendRedirectURL == "" {
		settings.GoogleOAuthFrontendRedirectURL = defaultGoogleOAuthFrontend
	}
	return string(clientIPTrustedProxiesJSON), nil
}

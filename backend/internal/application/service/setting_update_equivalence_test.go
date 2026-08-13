//go:build unit

package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	clientip "github.com/Wei-Shaw/sub2api/internal/shared/ip"
	"github.com/stretchr/testify/require"
)

var benchmarkSystemSettingUpdates map[string]string

func newSystemSettingsUpdateFixture(svc *SettingService) *SystemSettings {
	settings := svc.parseSettings(nil)
	settings.RegistrationEmailSuffixWhitelist = []string{" @Example.COM ", "*.EDU.CN"}
	settings.LoginAgreementDocuments = []LoginAgreementDocument{{
		ID:        "terms",
		Title:     " Terms ",
		ContentMD: "Fixture terms",
	}}
	settings.SMTPPassword = " smtp-secret "
	settings.TurnstileSecretKey = "turnstile-secret"
	settings.RecaptchaSecretKey = "recaptcha-secret"
	settings.CapSecretKey = "cap-secret"
	settings.AliyunCaptchaEnabled = true
	settings.AliyunCaptchaAccessKeyID = " aliyun-id "
	settings.AliyunCaptchaAccessKeySecret = " aliyun-secret "
	settings.AliyunCaptchaSceneID = " scene-1 "
	settings.AliyunCaptchaPrefix = " prefix-1 "
	settings.AliyunCaptchaRegion = AliyunCaptchaRegionSGP
	settings.LinuxDoConnectClientSecret = "linuxdo-secret"
	settings.DingTalkConnectClientSecret = "dingtalk-secret"
	settings.OIDCConnectClientSecret = "oidc-secret"
	settings.GitHubOAuthClientSecret = " github-secret "
	settings.GoogleOAuthClientSecret = " google-secret "
	settings.WeChatConnectAppID = " legacy-app "
	settings.WeChatConnectAppSecret = " legacy-secret "
	settings.WeChatConnectOpenAppID = " open-app "
	settings.WeChatConnectOpenAppSecret = " open-secret "
	settings.WeChatConnectMPAppID = " mp-app "
	settings.WeChatConnectMPAppSecret = " mp-secret "
	settings.WeChatConnectMobileAppID = " mobile-app "
	settings.WeChatConnectMobileAppSecret = " mobile-secret "
	settings.ClientIPResolutionMode = clientip.ResolutionModeTrustedProxy
	settings.ClientIPTrustedProxies = []string{"192.0.2.7", "2001:db8::/32"}
	settings.PaymentVisibleMethodAlipaySource = "alipay"
	settings.PaymentVisibleMethodWxpaySource = "easypay"
	settings.TableDefaultPageSize = 50
	settings.TablePageSizeOptions = []int{20, 50, 100}
	settings.OpsMetricsIntervalSeconds = 17
	settings.ChannelMonitorDefaultIntervalSeconds = 31
	settings.CyberSessionBlockTTLSeconds = 47
	settings.OpenAIOAuthSchedulingRateMultiplier = 0.25
	settings.OpenAIAdvancedSchedulerLBTopK = " 3 "
	settings.OpenAIAdvancedSchedulerWeightPriority = "2.5"
	settings.OpenAIAdvancedSchedulerWeightLoad = "1"
	settings.ClaudeOAuthSystemPromptBlocks = `[{"type":"text","text":"fixture"}]`
	quota := 1.25
	settings.DefaultPlatformQuotas = map[string]*DefaultPlatformQuotaSetting{
		"openai": {DailyLimitUSD: &quota},
	}
	return settings
}

func digestSystemSettingUpdates(t testing.TB, updates map[string]string) string {
	t.Helper()
	keys := make([]string, 0, len(updates))
	for key := range updates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	entries := make([][2]string, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, [2]string{key, updates[key]})
	}
	payload, err := json.Marshal(entries)
	require.NoError(t, err)
	return fmt.Sprintf("%x", sha256.Sum256(payload))
}

func TestBuildSystemSettingsUpdatesGolden(t *testing.T) {
	svc := NewSettingService(nil, &config.Config{})
	settings := newSystemSettingsUpdateFixture(svc)

	updates, err := svc.buildSystemSettingsUpdates(context.Background(), settings)
	require.NoError(t, err)

	require.Equal(t, 225, len(updates), "system setting key count")
	require.Equal(t, "8e64bdc8b5ced0bb30c8ed8b45d72e1e3c2cbf904fadbc8f84fe02697d734699", digestSystemSettingUpdates(t, updates), "system setting digest")
	require.Equal(t, []string{"@example.com", "*.edu.cn"}, settings.RegistrationEmailSuffixWhitelist)
	require.Equal(t, clientip.ResolutionModeTrustedProxy, settings.ClientIPResolutionMode)
	require.Equal(t, []string{"192.0.2.7/32", "2001:db8::/32"}, settings.ClientIPTrustedProxies)
	require.Equal(t, "open-app", settings.WeChatConnectOpenAppID)
	require.Equal(t, "open-secret", settings.WeChatConnectOpenAppSecret)
	require.Equal(t, TencentCaptchaRegionCN, settings.TencentCaptchaRegion)
	require.Equal(t, "3", settings.OpenAIAdvancedSchedulerLBTopK)
}

func TestBuildSystemSettingsUpdatesPreservesSecretOverwriteSemantics(t *testing.T) {
	svc := NewSettingService(nil, &config.Config{})
	secretKeys := []string{
		SettingKeySMTPPassword,
		SettingKeyTurnstileSecretKey,
		SettingKeyRecaptchaSecretKey,
		SettingKeyCapSecretKey,
		SettingKeyAliyunCaptchaAccessKeySecret,
		SettingKeyLinuxDoConnectClientSecret,
		SettingKeyDingTalkConnectClientSecret,
		SettingKeyOIDCConnectClientSecret,
		SettingKeyGitHubOAuthClientSecret,
		SettingKeyGoogleOAuthClientSecret,
		SettingKeyWeChatConnectAppSecret,
		SettingKeyWeChatConnectOpenAppSecret,
		SettingKeyWeChatConnectMPAppSecret,
		SettingKeyWeChatConnectMobileAppSecret,
	}

	emptyUpdates, err := svc.buildSystemSettingsUpdates(context.Background(), &SystemSettings{})
	require.NoError(t, err)
	for _, key := range secretKeys {
		require.NotContains(t, emptyUpdates, key)
	}

	settings := newSystemSettingsUpdateFixture(svc)
	updates, err := svc.buildSystemSettingsUpdates(context.Background(), settings)
	require.NoError(t, err)
	want := map[string]string{
		SettingKeySMTPPassword:                 " smtp-secret ",
		SettingKeyTurnstileSecretKey:           "turnstile-secret",
		SettingKeyRecaptchaSecretKey:           "recaptcha-secret",
		SettingKeyCapSecretKey:                 "cap-secret",
		SettingKeyAliyunCaptchaAccessKeySecret: "aliyun-secret",
		SettingKeyLinuxDoConnectClientSecret:   "linuxdo-secret",
		SettingKeyDingTalkConnectClientSecret:  "dingtalk-secret",
		SettingKeyOIDCConnectClientSecret:      "oidc-secret",
		SettingKeyGitHubOAuthClientSecret:      "github-secret",
		SettingKeyGoogleOAuthClientSecret:      "google-secret",
		SettingKeyWeChatConnectAppSecret:       "legacy-secret",
		SettingKeyWeChatConnectOpenAppSecret:   "open-secret",
		SettingKeyWeChatConnectMPAppSecret:     "mp-secret",
		SettingKeyWeChatConnectMobileAppSecret: "mobile-secret",
	}
	for key, value := range want {
		require.Equal(t, value, updates[key], key)
	}
}

func TestBuildSystemSettingsUpdatesPreservesFirstErrorOrder(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*SystemSettings)
		reason string
	}{
		{
			name: "scheduler before registration whitelist",
			mutate: func(settings *SystemSettings) {
				settings.SchedulerV2CandidateLimit = 10
				settings.SchedulerV2ScanLimit = 1
				settings.RegistrationEmailSuffixWhitelist = []string{"@invalid_domain"}
			},
			reason: "INVALID_SCHEDULER_V2_LIMITS",
		},
		{
			name: "default subscription before registration whitelist",
			mutate: func(settings *SystemSettings) {
				settings.DefaultSubscriptions = []DefaultSubscriptionSetting{
					{GroupID: 7, ValidityDays: 30},
					{GroupID: 7, ValidityDays: 60},
				}
				settings.RegistrationEmailSuffixWhitelist = []string{"@invalid_domain"}
			},
			reason: "DEFAULT_SUBSCRIPTION_GROUP_DUPLICATE",
		},
		{
			name: "registration whitelist before payment source",
			mutate: func(settings *SystemSettings) {
				settings.RegistrationEmailSuffixWhitelist = []string{"@invalid_domain"}
				settings.PaymentVisibleMethodAlipaySource = "invalid"
			},
			reason: "INVALID_REGISTRATION_EMAIL_SUFFIX_WHITELIST",
		},
		{
			name: "payment source before scheduler overrides",
			mutate: func(settings *SystemSettings) {
				settings.PaymentVisibleMethodAlipaySource = "invalid"
				settings.OpenAIOAuthSchedulingRateMultiplier = -1
			},
			reason: "INVALID_PAYMENT_VISIBLE_METHOD_SOURCE",
		},
		{
			name: "scheduler overrides before client ip mode",
			mutate: func(settings *SystemSettings) {
				settings.OpenAIOAuthSchedulingRateMultiplier = -1
				settings.ClientIPResolutionMode = "invalid"
			},
			reason: "INVALID_OPENAI_OAUTH_SCHEDULING_RATE_MULTIPLIER",
		},
		{
			name: "client ip mode before trusted proxies",
			mutate: func(settings *SystemSettings) {
				settings.ClientIPResolutionMode = "invalid"
				settings.ClientIPTrustedProxies = []string{"invalid"}
			},
			reason: "INVALID_CLIENT_IP_RESOLUTION_MODE",
		},
		{
			name: "trusted proxies before prompt blocks",
			mutate: func(settings *SystemSettings) {
				settings.ClientIPResolutionMode = clientip.ResolutionModeAutoCompat
				settings.ClientIPTrustedProxies = []string{"invalid"}
				settings.ClaudeOAuthSystemPromptBlocks = "{"
			},
			reason: "INVALID_CLIENT_IP_TRUSTED_PROXIES",
		},
		{
			name: "prompt blocks before platform quotas",
			mutate: func(settings *SystemSettings) {
				settings.ClaudeOAuthSystemPromptBlocks = "{"
				settings.DefaultPlatformQuotas = map[string]*DefaultPlatformQuotaSetting{"invalid": nil}
			},
			reason: "INVALID_CLAUDE_OAUTH_SYSTEM_PROMPT_BLOCKS",
		},
		{
			name: "platform quotas last",
			mutate: func(settings *SystemSettings) {
				settings.DefaultPlatformQuotas = map[string]*DefaultPlatformQuotaSetting{"invalid": nil}
			},
			reason: "INVALID_DEFAULT_PLATFORM_QUOTA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := NewSettingService(nil, &config.Config{})
			settings := &SystemSettings{}
			tt.mutate(settings)
			_, err := svc.buildSystemSettingsUpdates(context.Background(), settings)
			require.Error(t, err)
			require.Equal(t, tt.reason, infraerrors.Reason(err))
		})
	}
}

func BenchmarkBuildSystemSettingsUpdates(b *testing.B) {
	svc := NewSettingService(nil, &config.Config{})
	base := newSystemSettingsUpdateFixture(svc)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		settings := *base
		updates, err := svc.buildSystemSettingsUpdates(context.Background(), &settings)
		if err != nil {
			b.Fatal(err)
		}
		benchmarkSystemSettingUpdates = updates
	}
}

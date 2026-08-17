package admin

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestDiffSettings_PreservesDomainOrderAndSecretSemantics(t *testing.T) {
	quota := 10.0
	before := &service.SystemSettings{}
	after := &service.SystemSettings{
		RegistrationEnabled:                  true,
		TurnstileSiteKey:                     "site-key",
		LinuxDoConnectEnabled:                true,
		SiteName:                             "Sub2API",
		DefaultConcurrency:                   4,
		OpsMonitoringEnabled:                 true,
		MinClaudeCodeVersion:                 "1.2.3",
		PaymentVisibleMethodAlipaySource:     "alipay",
		OpenAILowUpstreamRatePriorityEnabled: true,
		BalanceLowNotifyEnabled:              true,
		ChannelMonitorEnabled:                true,
		IPv6EgressUIEnabled:                  true,
		RiskControlEnabled:                   true,
		DefaultPlatformQuotas: map[string]*service.DefaultPlatformQuotaSetting{
			"anthropic": {DailyLimitUSD: &quota},
		},
	}
	beforeAuthSourceDefaults := &service.AuthSourceDefaultSettings{}
	afterAuthSourceDefaults := &service.AuthSourceDefaultSettings{
		Email:                        service.ProviderDefaultGrantSettings{Balance: 5},
		ForceEmailOnThirdPartySignup: true,
	}
	req := UpdateSettingsRequest{
		SMTPPassword:               "new-smtp-password",
		TurnstileSecretKey:         "new-turnstile-secret",
		LinuxDoConnectClientSecret: "new-linuxdo-secret",
	}

	changed := diffSettings(before, after, beforeAuthSourceDefaults, afterAuthSourceDefaults, req)

	require.Equal(t, []string{
		"registration_enabled",
		"smtp_password",
		"turnstile_site_key",
		"turnstile_secret_key",
		"linuxdo_connect_enabled",
		"linuxdo_connect_client_secret",
		"site_name",
		"default_concurrency",
		"ops_monitoring_enabled",
		"min_claude_code_version",
		"payment_visible_method_alipay_source",
		"openai_low_upstream_rate_priority_enabled",
		"balance_low_notify_enabled",
		"channel_monitor_enabled",
		"ipv6_egress_ui_enabled",
		"risk_control_enabled",
		service.SettingKeyDefaultPlatformQuotas,
		"auth_source_default_email_balance",
		"force_email_on_third_party_signup",
	}, changed)
}

func BenchmarkDiffSettings_AllUnchanged(b *testing.B) {
	settings := &service.SystemSettings{}
	authSourceDefaults := &service.AuthSourceDefaultSettings{}
	req := UpdateSettingsRequest{}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = diffSettings(settings, settings, authSourceDefaults, authSourceDefaults, req)
	}
}

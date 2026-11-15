package service

import (
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
)

var benchmarkParsedSystemSettings *SystemSettings

func BenchmarkSettingServiceParseSettings(b *testing.B) {
	service := NewSettingService(nil, &config.Config{})
	tests := []struct {
		name     string
		settings map[string]string
	}{
		{name: "defaults", settings: map[string]string{}},
		{name: "persisted", settings: map[string]string{
			SettingKeyRegistrationEnabled:               "true",
			SettingKeyEmailVerifyEnabled:                "true",
			SettingKeyClientIPResolutionMode:            "trusted_proxy",
			SettingKeyClientIPTrustedProxies:            `["10.0.0.0/8","2001:db8::/32"]`,
			SettingKeyDefaultConcurrency:                "8",
			SettingKeyDefaultBalance:                    "12.5",
			SettingKeyOIDCConnectEnabled:                "true",
			SettingKeyOIDCConnectIssuerURL:              "https://issuer.example.com",
			SettingKeyOIDCConnectScopes:                 "openid email profile",
			SettingKeyOpenAIAdvancedSchedulerWeightLoad: "2.5",
			SettingKeyAccountQuotaNotifyEmails:          `[{"email":"ops@example.com"}]`,
			SettingKeyDefaultPlatformQuotas:             `{"openai":{"daily":10}}`,
		}},
	}

	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchmarkParsedSystemSettings = service.parseSettings(test.settings)
			}
		})
	}
}

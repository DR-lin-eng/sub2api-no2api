package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCNProviderPlatformsUseIndependentOpenAICompatiblePools(t *testing.T) {
	tests := []struct {
		platform string
		baseURL  string
		model    string
	}{
		{PlatformKimi, DefaultKimiPayGBaseURL, "kimi-k3"},
		{PlatformZhipu, DefaultZhipuPayGBaseURL, "glm-5.3"},
		{PlatformDeepseek, DefaultDeepseekBaseURL, "deepseek-chat"},
		{PlatformMiniMax, DefaultMiniMaxBaseURL, "MiniMax-M3"},
	}
	for _, tt := range tests {
		t.Run(tt.platform, func(t *testing.T) {
			account := &Account{
				ID: 42, Platform: tt.platform, Type: AccountTypeAPIKey,
				Credentials: map[string]any{"api_key": "provider-key"},
			}
			require.True(t, IsCNProvider(tt.platform))
			require.True(t, account.IsCNProvider())
			require.True(t, account.IsOpenAICompatible())
			require.Equal(t, tt.baseURL, account.GetOpenAIBaseURL())
			require.Equal(t, "provider-key", account.GetOpenAIProtocolAPIKey())
			require.Equal(t, tt.platform, normalizeOpenAICompatiblePlatform(tt.platform))
			require.True(t, shouldForwardOpenAIResponsesViaRawChatCompletions(account))
			platform, ok := DetectModelPlatform(tt.model)
			require.True(t, ok)
			require.Equal(t, tt.platform, platform)
			require.Equal(t, "", (&Group{Platform: tt.platform}).ResolveMessagesDispatchModel("claude-sonnet-4-6"))
			require.Equal(t, tt.platform, QuotaPlatform(context.Background(), &APIKey{Group: &Group{Platform: tt.platform}}))
		})
	}
}

func TestCNProviderBaseURLOverridesAndCodingDefaults(t *testing.T) {
	custom := &Account{
		Platform: PlatformDeepseek, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "key", "base_url": " https://relay.example/v1 "},
	}
	require.Equal(t, "https://relay.example/v1", custom.GetOpenAIBaseURL())

	kimiCoding := &Account{
		Platform: PlatformKimi, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "key", "account_mode": AccountModeCoding},
	}
	require.Equal(t, DefaultKimiCodingBaseURL, kimiCoding.GetOpenAIBaseURL())
	require.True(t, kimiCoding.IsCodingPlan())
}

func BenchmarkNormalizeCNProviderPlatform(b *testing.B) {
	platforms := []string{PlatformKimi, PlatformZhipu, PlatformDeepseek, PlatformMiniMax}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = normalizeOpenAICompatiblePlatform(platforms[i&3])
	}
}

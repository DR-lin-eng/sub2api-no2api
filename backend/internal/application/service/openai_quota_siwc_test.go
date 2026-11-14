package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/stretchr/testify/require"
)

func TestSIWCQuotaQueryAndResetUseSIWCHeadersAndWHAMWindows(t *testing.T) {
	account := &Account{
		ID:       901,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"auth_mode":          OpenAIAuthModeSIWC,
			"access_token":       "siwc-access-token",
			"chatgpt_account_id": "acct-siwc-quota",
		},
	}
	repo := &stubQuotaAccountRepo{accounts: map[int64]*Account{account.ID: account}}
	tokenCache := &stubQuotaTokenCache{tokens: map[string]string{
		OpenAITokenCacheKey(account): "siwc-access-token",
	}}
	tokenProvider := NewOpenAITokenProvider(repo, tokenCache, nil)
	usageCalls := 0
	creditCalls := 0
	resetCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer siwc-access-token", r.Header.Get("Authorization"))
		require.Equal(t, "acct-siwc-quota", r.Header.Get("ChatGPT-Account-Id"))
		require.Contains(t, r.Header.Get("User-Agent"), openai.SIWCAgentNameHint)
		require.Equal(t, openaiQuotaCodexOriginator, r.Header.Get("Originator"), "WHAM keeps its quota originator contract")
		switch r.URL.Path {
		case "/backend-api/wham/usage":
			usageCalls++
			_, _ = w.Write([]byte(`{"plan_type":"plus","rate_limits_by_limit_id":{"codex":{"limit_id":"codex","primary":{"usedPercent":42,"windowDurationMins":300,"resetsAt":1799990000},"secondary":{"usedPercent":71,"windowDurationMins":10080,"resetsAt":1800990000}}}}`))
		case "/backend-api/wham/rate-limit-reset-credits":
			creditCalls++
			_, _ = w.Write([]byte(`{"available_count":1,"credits":[{"reset_type":"codex_rate_limits","status":"available","expires_at":"2099-01-01T00:00:00Z"}]}`))
		case "/backend-api/wham/rate-limit-reset-credits/consume":
			resetCalls++
			require.Equal(t, http.MethodPost, r.Method)
			var body map[string]string
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			require.NotEmpty(t, body["redeem_request_id"])
			_, _ = w.Write([]byte(`{"code":"ok","windows_reset":2,"credit":{"status":"redeemed"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	svc := NewOpenAIQuotaService(repo, nil, tokenProvider, newQuotaRedirectingFactory(server))
	usage, err := svc.QueryUsage(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, 1, usageCalls)
	require.Equal(t, 1, creditCalls)
	require.Equal(t, "plus", usage.PlanType)
	require.NotNil(t, usage.RateLimitsByLimitID["codex"].Primary)
	require.InDelta(t, 42, usage.RateLimitsByLimitID["codex"].Primary.UsedPercent, 0.001)
	require.Equal(t, 1, usage.RateLimitResetCredits.AvailableCount)
	require.Len(t, usage.RateLimitResetCredits.Credits, 1)

	reset, err := svc.ResetCredit(context.Background(), account.ID)
	require.NoError(t, err)
	require.Equal(t, 1, resetCalls)
	require.Equal(t, "ok", reset.Code)
	require.Equal(t, 2, reset.WindowsReset)
}

func TestSIWCQuotaAccountStillUsesExistingSnapshotAndFilterShape(t *testing.T) {
	account := &Account{
		ID: 902, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Credentials: map[string]any{"auth_mode": OpenAIAuthModeSIWC, "chatgpt_account_id": "acct"},
		Extra: map[string]any{"codex_rate_limit_snapshot": map[string]any{
			"rate_limits_by_limit_id": map[string]any{"codex": map[string]any{"primary": map[string]any{"usedPercent": 50}}},
		}},
	}
	require.True(t, account.IsOpenAISIWC())
	require.Contains(t, strings.ToLower(account.GetCredential("auth_mode")), "siwc")
}

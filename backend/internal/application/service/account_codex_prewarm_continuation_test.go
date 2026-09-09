package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/codexsimulation"
	"github.com/stretchr/testify/require"
)

var benchmarkCodexPrewarmContinuationEnabled bool

func TestAccount_IsCodexPrewarmContinuationEnabled(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{
			name: "enabled for OpenAI OAuth",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Extra:    map[string]any{CodexPrewarmContinuationExtraKey: true},
			},
			want: true,
		},
		{
			name: "disabled for OpenAI API key",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeAPIKey,
				Extra:    map[string]any{CodexPrewarmContinuationExtraKey: true},
			},
		},
		{
			name: "disabled for non OpenAI OAuth",
			account: &Account{
				Platform: PlatformAnthropic,
				Type:     AccountTypeOAuth,
				Extra:    map[string]any{CodexPrewarmContinuationExtraKey: true},
			},
		},
		{
			name: "disabled for malformed value",
			account: &Account{
				Platform: PlatformOpenAI,
				Type:     AccountTypeOAuth,
				Extra:    map[string]any{CodexPrewarmContinuationExtraKey: "true"},
			},
		},
		{name: "disabled for nil account", account: nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.account.IsCodexPrewarmContinuationEnabled(); got != tc.want {
				t.Fatalf("IsCodexPrewarmContinuationEnabled() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestAccount_IsCodexPrewarmContinuationEnabledHonorsGlobalForce(t *testing.T) {
	codexsimulation.SetPrewarmContinuationEnabled(true)
	t.Cleanup(func() { codexsimulation.SetPrewarmContinuationEnabled(false) })
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.True(t, account.IsCodexPrewarmContinuationEnabled())
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	require.False(t, apiKey.IsCodexPrewarmContinuationEnabled())
}

func BenchmarkAccount_IsCodexPrewarmContinuationEnabled(b *testing.B) {
	accounts := map[string]*Account{
		"disabled": {
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{"other_setting": true},
		},
		"enabled": {
			Platform: PlatformOpenAI,
			Type:     AccountTypeOAuth,
			Extra:    map[string]any{CodexPrewarmContinuationExtraKey: true},
		},
	}

	for name, account := range accounts {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				benchmarkCodexPrewarmContinuationEnabled = account.IsCodexPrewarmContinuationEnabled()
			}
		})
	}
}

func TestAccount_CodexPrewarmContinuationBypassesOnlyLocal429Blocks(t *testing.T) {
	future := time.Now().Add(time.Hour)
	enabledExtra := map[string]any{CodexPrewarmContinuationExtraKey: true}
	newAccount := func() *Account {
		return &Account{
			Platform:    PlatformOpenAI,
			Type:        AccountTypeOAuth,
			Status:      StatusActive,
			Schedulable: true,
			Extra:       enabledExtra,
		}
	}

	t.Run("account rate limit remains schedulable", func(t *testing.T) {
		account := newAccount()
		account.RateLimitResetAt = &future
		require.True(t, account.IsSchedulable())
	})

	t.Run("429 temp state remains schedulable", func(t *testing.T) {
		account := newAccount()
		account.TempUnschedulableUntil = &future
		account.TempUnschedulableReason = `{"status_code":429,"error_message":"limited"}`
		require.True(t, account.IsSchedulable())
	})

	t.Run("401 temp state still blocks", func(t *testing.T) {
		account := newAccount()
		account.TempUnschedulableUntil = &future
		account.TempUnschedulableReason = `{"status_code":401,"error_message":"unauthorized"}`
		require.False(t, account.IsSchedulable())
	})

	t.Run("malformed temp state still blocks", func(t *testing.T) {
		account := newAccount()
		account.TempUnschedulableUntil = &future
		account.TempUnschedulableReason = "rate limited"
		require.False(t, account.IsSchedulable())
	})

	t.Run("overload still blocks", func(t *testing.T) {
		account := newAccount()
		account.OverloadUntil = &future
		require.False(t, account.IsSchedulable())
	})

	t.Run("disabled switch keeps rate limit block", func(t *testing.T) {
		account := newAccount()
		account.Extra = map[string]any{}
		account.RateLimitResetAt = &future
		require.False(t, account.IsSchedulable())
	})
}

func TestAccount_CodexPrewarmContinuationBypassesOnly429ModelLimits(t *testing.T) {
	resetAt := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	tests := []struct {
		name   string
		reason string
		want   bool
	}{
		{name: "Codex quota 429", reason: openAIModelRateLimitReason, want: true},
		{name: "Codex fallback 429", reason: openAIModelRateLimitReason + ":no_reset_time", want: true},
		{name: "custom temp 429", reason: `{"status_code":429}`, want: true},
		{name: "model not found", reason: upstreamModelNotFoundReason, want: false},
		{name: "plan gated", reason: upstreamCodexPlanGatedModelReason, want: false},
		{name: "custom temp 503", reason: `{"status_code":503}`, want: false},
		{name: "unknown legacy reason", reason: "", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			account := &Account{
				Platform:    PlatformOpenAI,
				Type:        AccountTypeOAuth,
				Status:      StatusActive,
				Schedulable: true,
				Extra: map[string]any{
					CodexPrewarmContinuationExtraKey: true,
					modelRateLimitsKey: map[string]any{
						"gpt-5.5": map[string]any{
							"rate_limit_reset_at": resetAt,
							"reason":              tc.reason,
						},
					},
				},
			}
			require.Equal(t, tc.want, account.IsSchedulableForModelWithContext(context.Background(), "gpt-5.5"))
			if tc.want {
				require.Zero(t, account.GetModelRateLimitRemainingTime("gpt-5.5"))
			} else {
				require.Positive(t, account.GetModelRateLimitRemainingTime("gpt-5.5"))
			}
		})
	}
}

func BenchmarkAccount_IsSchedulableCodex429Bypass(b *testing.B) {
	future := time.Now().Add(time.Hour)
	accounts := map[string]*Account{
		"account_rate_limit": {
			Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
			RateLimitResetAt: &future,
			Extra:            map[string]any{CodexPrewarmContinuationExtraKey: true},
		},
		"temp_429": {
			Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true,
			TempUnschedulableUntil:  &future,
			TempUnschedulableReason: `{"status_code":429}`,
			Extra:                   map[string]any{CodexPrewarmContinuationExtraKey: true},
		},
	}

	for name, account := range accounts {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				benchmarkCodexPrewarmContinuationEnabled = account.IsSchedulable()
			}
		})
	}
}

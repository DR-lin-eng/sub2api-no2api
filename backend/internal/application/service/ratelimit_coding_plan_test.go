//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCodingPlanQuota403DoesNotDisableCredential(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	reset := now.Add(7 * 24 * time.Hour)
	account := &Account{ID: 71, Platform: PlatformKimi, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"account_mode": AccountModeCoding, "base_url": "https://api.kimi.com/coding"},
		Extra: cnQuotaExtraUpdates(PlatformKimi, []CNQuotaTier{
			{Window: "5h", UsedPercent: 20, ResetAt: now.Add(time.Hour).Format(time.RFC3339)},
			{Window: "weekly", UsedPercent: 100, ResetAt: reset.Format(time.RFC3339)},
		}, now)}
	repo := &rateLimitAccountRepoStub{}
	svc := &RateLimitService{accountRepo: repo}
	for range 4 {
		require.False(t, svc.handle403(context.Background(), account, "quota will reset", []byte(`{"error":{"type":"access_terminated_error"}}`)))
	}
	require.Zero(t, repo.setErrorCalls)
	require.Equal(t, 4, repo.rateLimitedCalls)
	require.Equal(t, reset, repo.lastRateLimitedResetAt)
}

func TestCodingPlanQuota403DoesNotReclassifyOrdinaryAccounts(t *testing.T) {
	body := []byte(`{"error":{"type":"access_terminated_error"}}`)
	require.False(t, isCodingPlanQuotaExhausted403(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, body, "usage limit"))
	require.False(t, isCodingPlanQuotaExhausted403(&Account{Platform: PlatformKimi, Type: AccountTypeAPIKey}, body, "usage limit"))
}

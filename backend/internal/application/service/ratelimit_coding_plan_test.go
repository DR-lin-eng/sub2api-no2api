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

func TestZhipuPlatformOverload1305UsesShort429Cooldown(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	windowReset := now.Add(4 * time.Hour)
	account := &Account{
		ID:       88,
		Platform: PlatformZhipu,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"account_mode": AccountModeCoding,
			"base_url":     "https://open.bigmodel.cn/api/coding/paas/v4",
		},
		Extra: cnQuotaExtraUpdates(PlatformZhipu, []CNQuotaTier{
			{Window: "5h", UsedPercent: 35, ResetAt: windowReset.Format(time.RFC3339)},
		}, now),
	}
	repo := &rateLimitAccountRepoStub{}
	svc := &RateLimitService{accountRepo: repo}
	body := []byte(`{"error":{"code":"1305","message":"当前API请求过多，请稍后重试。"}}`)
	svc.handle429(context.Background(), account, nil, body, "glm-4.6")

	require.Zero(t, repo.setErrorCalls)
	require.Equal(t, 1, repo.rateLimitedCalls)
	require.Less(t, time.Until(repo.lastRateLimitedResetAt), 5*time.Minute,
		"Zhipu 1305 platform overload 429 must use short seconds-level cooldown instead of multi-hour window reset")
}

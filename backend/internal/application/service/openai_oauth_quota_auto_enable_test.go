//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type openAIOAuthQuotaAutoEnableRepoStub struct {
	mockAccountRepoForGemini
	account        *Account
	enableSingle   bool
	enableBatchIDs []int64
	singleCalls    int
	batchCalls     int
	lastBatchLimit int
	lastBatchNow   time.Time
}

func (r *openAIOAuthQuotaAutoEnableRepoStub) GetByID(context.Context, int64) (*Account, error) {
	return r.account, nil
}

func (r *openAIOAuthQuotaAutoEnableRepoStub) AutoEnableOpenAIOAuthAccountIfMarked(context.Context, int64) (bool, error) {
	r.singleCalls++
	if r.enableSingle && r.account != nil {
		r.account.Schedulable = true
		delete(r.account.Extra, AccountSchedulingDisabledReasonExtraKey)
		delete(r.account.Extra, AccountAutoEnableSourceExtraKey)
		delete(r.account.Extra, AccountAutoEnableAtExtraKey)
	}
	return r.enableSingle, nil
}

func (r *openAIOAuthQuotaAutoEnableRepoStub) AutoEnableOpenAIOAuthAccountsAfterQuotaReset(_ context.Context, now time.Time, limit int) ([]int64, error) {
	r.batchCalls++
	r.lastBatchNow = now
	r.lastBatchLimit = limit
	return r.enableBatchIDs, nil
}

func openAIOAuthQuotaAutoEnableSettings(t *testing.T, afterReset, whenAvailable bool) *SettingService {
	t.Helper()
	repo := newMockSettingRepo()
	repo.data[SettingKeyRateLimit429CooldownSettings] = `{"enabled":true,"cooldown_seconds":5,"auto_disable_enabled":true,"auto_disable_threshold":3,"auto_enable_after_quota_reset_enabled":` +
		boolText(afterReset) + `,"auto_enable_when_quota_available_enabled":` + boolText(whenAvailable) + `}`
	return NewSettingService(repo, &config.Config{})
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func autoDisabledOpenAIOAuthAccount() *Account {
	return &Account{
		ID:          71,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: false,
		Extra: map[string]any{
			AccountSchedulingDisabledReasonExtraKey: "failure threshold reached",
			AccountAutoEnableSourceExtraKey:         AccountAutoEnableSourceOpenAIOAuthFailure,
			AccountAutoEnableAtExtraKey:             time.Now().Add(-time.Minute).Format(time.RFC3339),
		},
	}
}

func availableOpenAIQuotaUsage() *OpenAIQuotaUsage {
	return &OpenAIQuotaUsage{RateLimitsByLimitID: map[string]OpenAIAppServerRateLimitBucket{
		"codex": {
			LimitID: "codex",
			Primary: &OpenAIAppServerRateLimitWindow{UsedPercent: 42, WindowDurationMins: 10080},
		},
	}}
}

func TestMaybeAutoEnableOpenAIAccountAfterQuotaQuery(t *testing.T) {
	t.Run("known available quota restores only marked OAuth account", func(t *testing.T) {
		account := autoDisabledOpenAIOAuthAccount()
		repo := &openAIOAuthQuotaAutoEnableRepoStub{account: account, enableSingle: true}
		counter := &openAIFailureCounterStub{count: 3}
		svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		svc.SetSettingService(openAIOAuthQuotaAutoEnableSettings(t, false, true))
		svc.SetOpenAIFailureCounterCache(counter)

		updated, enabled, err := svc.MaybeAutoEnableOpenAIAccountAfterQuotaQuery(context.Background(), account.ID, availableOpenAIQuotaUsage())

		require.NoError(t, err)
		require.True(t, enabled)
		require.True(t, updated.Schedulable)
		require.Equal(t, 1, repo.singleCalls)
		require.Equal(t, 1, counter.resetCalls)
		require.Empty(t, updated.Extra[AccountAutoEnableSourceExtraKey])
	})

	for _, tc := range []struct {
		name    string
		account *Account
		usage   *OpenAIQuotaUsage
	}{
		{name: "manual pause stays paused", account: func() *Account {
			account := autoDisabledOpenAIOAuthAccount()
			delete(account.Extra, AccountAutoEnableSourceExtraKey)
			return account
		}(), usage: availableOpenAIQuotaUsage()},
		{name: "API key stays paused", account: func() *Account {
			account := autoDisabledOpenAIOAuthAccount()
			account.Type = AccountTypeAPIKey
			return account
		}(), usage: availableOpenAIQuotaUsage()},
		{name: "unknown quota stays paused", account: autoDisabledOpenAIOAuthAccount(), usage: &OpenAIQuotaUsage{}},
		{name: "exhausted quota stays paused", account: autoDisabledOpenAIOAuthAccount(), usage: &OpenAIQuotaUsage{
			RateLimit: &OpenAIRateLimit{LimitReached: true},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &openAIOAuthQuotaAutoEnableRepoStub{account: tc.account, enableSingle: true}
			svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
			svc.SetSettingService(openAIOAuthQuotaAutoEnableSettings(t, false, true))

			_, enabled, err := svc.MaybeAutoEnableOpenAIAccountAfterQuotaQuery(context.Background(), tc.account.ID, tc.usage)

			require.NoError(t, err)
			require.False(t, enabled)
			require.Zero(t, repo.singleCalls)
		})
	}
}

func TestAutoEnableOpenAIAccountsAfterQuotaResetHonorsSwitch(t *testing.T) {
	now := time.Now().UTC()
	repo := &openAIOAuthQuotaAutoEnableRepoStub{enableBatchIDs: []int64{71, 72}}
	counter := &openAIFailureCounterStub{count: 3}
	svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
	svc.SetSettingService(openAIOAuthQuotaAutoEnableSettings(t, true, false))
	svc.SetOpenAIFailureCounterCache(counter)

	count, err := svc.AutoEnableOpenAIAccountsAfterQuotaReset(context.Background(), now)

	require.NoError(t, err)
	require.Equal(t, 2, count)
	require.Equal(t, 1, repo.batchCalls)
	require.Equal(t, openAIOAuthQuotaAutoEnableBatchSize, repo.lastBatchLimit)
	require.Equal(t, now, repo.lastBatchNow)
	require.Equal(t, 2, counter.resetCalls)

	repo.batchCalls = 0
	svc.SetSettingService(openAIOAuthQuotaAutoEnableSettings(t, false, false))
	svc.openAIFailurePolicyCache.Store((*cachedOpenAIFailurePolicySettings)(nil))
	count, err = svc.AutoEnableOpenAIAccountsAfterQuotaReset(context.Background(), now)
	require.NoError(t, err)
	require.Zero(t, count)
	require.Zero(t, repo.batchCalls)
}

package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestBuildOAuthPlanExpiryDistributionUsesRequestedGradients(t *testing.T) {
	now := time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC)
	accounts := []Account{
		{ID: 1, Type: AccountTypeOAuth, Credentials: map[string]any{"plan_type": "plus", "subscription_expires_at": now.Add(-time.Hour).Format(time.RFC3339)}},
		{ID: 2, Type: AccountTypeOAuth, Credentials: map[string]any{"plan_type": "plus", "subscription_expires_at": now.Add(12 * time.Hour).Format(time.RFC3339)}},
		{ID: 3, Type: AccountTypeOAuth, Credentials: map[string]any{"plan_type": "pro", "subscription_expires_at": now.Add(36 * time.Hour).Format(time.RFC3339)}},
		{ID: 4, Type: AccountTypeOAuth, Credentials: map[string]any{"plan_type": "pro", "subscription_expires_at": now.Add(4*24*time.Hour + time.Hour).Format(time.RFC3339)}},
		{ID: 5, Type: AccountTypeOAuth, Credentials: map[string]any{"plan_type": "team", "subscription_expires_at": now.Add(25*24*time.Hour + time.Hour).Format(time.RFC3339)}},
		{ID: 6, Type: AccountTypeOAuth, Credentials: map[string]any{"plan_type": "team"}},
		{ID: 7, Type: AccountTypeOAuth, Credentials: map[string]any{"plan_type": "invalid", "subscription_expires_at": "not-a-time"}},
		{ID: 8, Type: AccountTypeOAuth, ParentAccountID: oauthPlanExpiryInt64Ptr(99), Credentials: map[string]any{"plan_type": "plus", "subscription_expires_at": now.Add(24 * time.Hour).Format(time.RFC3339)}},
	}

	got := buildOAuthPlanExpiryDistribution(accounts, "openai", nil, now)
	require.Equal(t, int64(7), got.TotalOAuthAccounts)
	require.Equal(t, int64(5), got.AccountsWithExpiry)
	require.Equal(t, int64(2), got.AccountsWithoutExpiry)
	require.Equal(t, []string{
		"expired", "up_to_1", "up_to_2", "up_to_4", "up_to_5", "up_to_10",
		"up_to_15", "up_to_20", "up_to_25", "over_25",
	}, bucketKeys(got.Buckets))
	require.Equal(t, []int64{1, 1, 1, 0, 1, 0, 0, 0, 0, 1}, bucketCounts(got.Buckets))
	require.Equal(t, []OpsOAuthPlanSummary{
		{PlanType: "plus", Count: 2},
		{PlanType: "pro", Count: 2},
		{PlanType: "team", Count: 2},
		{PlanType: "invalid", Count: 1},
	}, derefPlanSummaries(got.Plans))
}

func TestOpsServiceGetOAuthPlanExpiryDistributionUsesNarrowRepository(t *testing.T) {
	now := time.Now().UTC()
	repo := &opsOAuthPlanExpiryRepoStub{accounts: []Account{{
		ID:          11,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"plan_type": "plus", "subscription_expires_at": now.Add(24 * time.Hour).Format(time.RFC3339)},
	}}}
	svc := NewOpsService(&opsRepoMock{}, nil, &config.Config{Ops: config.OpsConfig{Enabled: true}}, repo, nil, nil, nil, nil, nil, nil, nil)

	got, err := svc.GetOAuthPlanExpiryDistribution(context.Background(), "OPENAI", nil)
	require.NoError(t, err)
	require.Equal(t, "openai", got.Platform)
	require.Equal(t, int64(1), got.TotalOAuthAccounts)
	require.Equal(t, int64(1), got.AccountsWithExpiry)
	require.Equal(t, int64(1), got.Buckets[1].Count)
	require.Equal(t, 1, repo.calls)
}

type opsOAuthPlanExpiryRepoStub struct {
	AccountRepository
	accounts []Account
	calls    int
}

func (r *opsOAuthPlanExpiryRepoStub) ListOAuthPlanExpiryAccounts(_ context.Context, _ string, _ *int64) ([]Account, error) {
	r.calls++
	return r.accounts, nil
}

func oauthPlanExpiryInt64Ptr(value int64) *int64 { return &value }

func bucketKeys(items []*OpsOAuthPlanExpiryBucket) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.Key)
	}
	return out
}

func bucketCounts(items []*OpsOAuthPlanExpiryBucket) []int64 {
	out := make([]int64, 0, len(items))
	for _, item := range items {
		out = append(out, item.Count)
	}
	return out
}

func derefPlanSummaries(items []*OpsOAuthPlanSummary) []OpsOAuthPlanSummary {
	out := make([]OpsOAuthPlanSummary, 0, len(items))
	for _, item := range items {
		out = append(out, *item)
	}
	return out
}

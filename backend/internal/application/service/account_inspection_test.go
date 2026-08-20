package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/usagestats"
	"github.com/stretchr/testify/require"
)

type inspectionSettingRepoStub struct{ values map[string]string }

func (s *inspectionSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}
func (s *inspectionSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}
func (s *inspectionSettingRepoStub) Set(_ context.Context, key, value string) error {
	s.values[key] = value
	return nil
}
func (s *inspectionSettingRepoStub) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (s *inspectionSettingRepoStub) SetMultiple(_ context.Context, values map[string]string) error {
	for key, value := range values {
		s.values[key] = value
	}
	return nil
}
func (s *inspectionSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	return s.values, nil
}
func (s *inspectionSettingRepoStub) Delete(_ context.Context, key string) error {
	delete(s.values, key)
	return nil
}

type inspectionUsageRepoStub struct {
	UsageLogRepository
	stats map[int64]*usagestats.AccountHourlyUsageStats
}

func (s *inspectionUsageRepoStub) GetAccountHourlyUsageStatsBatch(_ context.Context, accountIDs []int64, _, _ time.Time) (map[int64]*usagestats.AccountHourlyUsageStats, error) {
	result := make(map[int64]*usagestats.AccountHourlyUsageStats, len(accountIDs))
	for _, id := range accountIDs {
		result[id] = s.stats[id]
	}
	return result, nil
}

type inspectionAccountRepoStub struct {
	AccountRepository
	accounts []Account
	updated  []int64
}

func (s *inspectionAccountRepoStub) ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]Account, error) {
	return append([]Account(nil), s.accounts...), nil
}
func (s *inspectionAccountRepoStub) BulkUpdate(_ context.Context, ids []int64, updates AccountBulkUpdate) (int64, error) {
	if updates.Schedulable == nil {
		return 0, errors.New("missing schedulable update")
	}
	s.updated = append([]int64(nil), ids...)
	for i := range s.accounts {
		for _, id := range ids {
			if s.accounts[i].ID == id {
				s.accounts[i].Schedulable = *updates.Schedulable
			}
		}
	}
	return int64(len(ids)), nil
}
func (s *inspectionAccountRepoStub) GetByIDs(_ context.Context, ids []int64) ([]*Account, error) {
	result := make([]*Account, 0, len(ids))
	for i := range s.accounts {
		for _, id := range ids {
			if s.accounts[i].ID == id {
				copy := s.accounts[i]
				result = append(result, &copy)
			}
		}
	}
	return result, nil
}

func TestAccountInspectionDefaultsMatchScriptThresholds(t *testing.T) {
	settings := DefaultAccountInspectionSettings()
	require.False(t, settings.Enabled)
	require.True(t, settings.AutoDisable)
	require.Equal(t, 60, settings.IntervalMinutes)
	require.Equal(t, 60, settings.LookbackMinutes)
	require.Equal(t, 30_000, settings.TTFTThresholdMs)
	require.InDelta(t, 0.60, settings.SuccessRateThreshold, 1e-9)
	require.Equal(t, 1, settings.MinRequests)
	require.True(t, settings.OAuthQuotaCheckEnabled)
}

func TestEvaluateAccountInspectionOAuthThresholdsAndQuota(t *testing.T) {
	now := time.Now().UTC()
	avg := 30_001.0
	stats := &usagestats.AccountHourlyUsageStats{
		TotalRequests:      10,
		SuccessfulRequests: 5,
		SuccessRate:        0.5,
		AvgFirstTokenMs:    &avg,
	}
	account := &Account{
		ID: 7, Name: "oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true,
		Extra: map[string]any{
			"codex_7d_used_percent": 100.0,
			"codex_7d_reset_at":     now.Add(time.Hour).Format(time.RFC3339),
		},
	}
	result := evaluateAccountInspection(account, stats, DefaultAccountInspectionSettings(), now)
	require.Equal(t, "flagged", result.Status)
	require.Equal(t, AccountInspectionActionReported, result.Action)
	require.ElementsMatch(t, []string{
		"first_token_over_threshold",
		"success_rate_below_threshold",
		"oauth_quota_exhausted:7d",
	}, result.Reasons)
}

func TestEvaluateAccountInspectionAPIKeyMetrics(t *testing.T) {
	now := time.Now().UTC()
	hitRate := 0.1
	stats := &usagestats.AccountHourlyUsageStats{
		TotalRequests: 4, SuccessfulRequests: 4, SuccessRate: 1, CacheHitRate: &hitRate,
	}
	limit := 10.0
	account := &Account{
		ID: 8, Name: "key", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, RateMultiplier: func() *float64 { v := 2.0; return &v }(),
		Extra: map[string]any{"quota_limit": limit, "quota_used": 9.5},
	}
	settings := DefaultAccountInspectionSettings()
	settings.APIKeyMinCacheHitRate = 0.5
	settings.APIKeyMaxRateMultiplier = 1.5
	settings.APIKeyMinRemainingQuota = 1
	result := evaluateAccountInspection(account, stats, settings, now)
	require.Equal(t, "flagged", result.Status)
	require.InDelta(t, 0.5, *result.RemainingQuota, 1e-9)
	require.Equal(t, "total", result.RemainingQuotaDimension)
	require.ElementsMatch(t, []string{
		"cache_hit_rate_below_threshold",
		"rate_multiplier_over_threshold",
		"remaining_quota_below_threshold",
	}, result.Reasons)
}

func TestAccountInspectionRemainingQuotaIgnoresExpiredWindows(t *testing.T) {
	account := &Account{
		Extra: map[string]any{
			"quota_daily_limit": 5.0,
			"quota_daily_used":  5.0,
			"quota_daily_start": time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
		},
	}
	remaining, dimension, unlimited := accountInspectionRemainingQuota(account)
	require.Nil(t, remaining)
	require.Empty(t, dimension)
	require.True(t, unlimited)
}

func TestAccountInspectionRunPersistsSnapshotAndDisablesFlaggedAccounts(t *testing.T) {
	settingsRepo := &inspectionSettingRepoStub{values: map[string]string{}}
	accountRepo := &inspectionAccountRepoStub{accounts: []Account{
		{ID: 1, Name: "slow", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true},
		{ID: 2, Name: "healthy", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true},
	}}
	avg := 30_001.0
	usageRepo := &inspectionUsageRepoStub{stats: map[int64]*usagestats.AccountHourlyUsageStats{
		1: {TotalRequests: 2, SuccessfulRequests: 2, SuccessRate: 1, AvgFirstTokenMs: &avg},
		2: {TotalRequests: 2, SuccessfulRequests: 2, SuccessRate: 1},
	}}
	usageService := NewAccountUsageService(nil, usageRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	svc := NewAccountInspectionService(accountRepo, usageService, settingsRepo)
	settings := DefaultAccountInspectionSettings()
	_, err := svc.UpdateSettings(context.Background(), &settings)
	require.NoError(t, err)
	state, err := svc.RunNow(context.Background(), "manual")
	require.NoError(t, err)
	require.Equal(t, AccountInspectionStatusSucceeded, state.Status)
	require.Equal(t, []int64{1}, accountRepo.updated)
	require.Equal(t, 1, state.Summary.Disabled)
	require.Equal(t, 1, state.Summary.Healthy)

	overview, err := svc.GetOverview(context.Background(), AccountInspectionListFilter{Page: 1, PageSize: 10, Status: "flagged"})
	require.NoError(t, err)
	require.Len(t, overview.Results.Items, 1)
	require.Equal(t, int64(1), overview.Results.Items[0].AccountID)
}

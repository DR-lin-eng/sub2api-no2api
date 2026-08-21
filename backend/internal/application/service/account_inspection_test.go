package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
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
	require.True(t, settings.OAuthAutoDisable)
	require.True(t, settings.APIKeyAutoDisable)
	require.Equal(t, settings.TTFTThresholdMs, settings.OAuthTTFTThresholdMs)
	require.Equal(t, settings.TTFTThresholdMs, settings.APIKeyTTFTThresholdMs)
	require.Empty(t, settings.ProtectedAccountIDs)
}

func TestAccountInspectionLegacySettingsPopulatePerTypePolicies(t *testing.T) {
	settingsRepo := &inspectionSettingRepoStub{values: map[string]string{
		SettingKeyAccountInspectionSettings: `{"enabled":true,"auto_disable":false,"min_requests":4,"ttft_threshold_ms":1200,"success_rate_threshold":0.8}`,
	}}
	svc := NewAccountInspectionService(nil, nil, settingsRepo)

	settings, err := svc.GetSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.AutoDisable)
	require.False(t, settings.OAuthAutoDisable)
	require.False(t, settings.APIKeyAutoDisable)
	require.Equal(t, 4, settings.OAuthMinRequests)
	require.Equal(t, 4, settings.APIKeyMinRequests)
	require.Equal(t, 1200, settings.OAuthTTFTThresholdMs)
	require.Equal(t, 1200, settings.APIKeyTTFTThresholdMs)
	require.InDelta(t, 0.8, settings.OAuthSuccessRateThreshold, 1e-9)
	require.InDelta(t, 0.8, settings.APIKeySuccessRateThreshold, 1e-9)
}

func TestAccountInspectionLegacyUpdatePreservesProtectionList(t *testing.T) {
	settingsRepo := &inspectionSettingRepoStub{values: map[string]string{}}
	svc := NewAccountInspectionService(nil, nil, settingsRepo)
	initial := DefaultAccountInspectionSettings()
	initial.ProtectedAccountIDs = []int64{11, 12}
	_, err := svc.UpdateSettings(context.Background(), &initial)
	require.NoError(t, err)

	legacy := DefaultAccountInspectionSettings()
	legacy.presentFields = map[string]bool{"auto_disable": true, "min_requests": true, "ttft_threshold_ms": true, "success_rate_threshold": true}
	_, err = svc.UpdateSettings(context.Background(), &legacy)
	require.NoError(t, err)
	updated, err := svc.GetSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, []int64{11, 12}, updated.ProtectedAccountIDs)
}

func TestAccountInspectionProgrammaticLegacySettingsPopulateTypePolicies(t *testing.T) {
	settings := AccountInspectionSettings{
		AutoDisable:          true,
		MinRequests:          3,
		TTFTThresholdMs:      900,
		SuccessRateThreshold: 0.75,
	}
	settings.normalize()
	require.True(t, settings.OAuthAutoDisable)
	require.True(t, settings.APIKeyAutoDisable)
	require.Equal(t, 3, settings.OAuthMinRequests)
	require.Equal(t, 3, settings.APIKeyMinRequests)
	require.Equal(t, 900, settings.OAuthTTFTThresholdMs)
	require.Equal(t, 900, settings.APIKeyTTFTThresholdMs)
	require.InDelta(t, 0.75, settings.OAuthSuccessRateThreshold, 1e-9)
	require.InDelta(t, 0.75, settings.APIKeySuccessRateThreshold, 1e-9)
}

func TestAccountInspectionExplicitTypeValuesOverrideLegacyFields(t *testing.T) {
	settingsRepo := &inspectionSettingRepoStub{values: map[string]string{
		SettingKeyAccountInspectionSettings: `{"auto_disable":true,"oauth_auto_disable":false,"api_key_auto_disable":true,"min_requests":1,"oauth_min_requests":3,"api_key_min_requests":1,"ttft_threshold_ms":30000,"oauth_ttft_threshold_ms":500,"api_key_ttft_threshold_ms":30000,"success_rate_threshold":0.6,"oauth_success_rate_threshold":0.9,"api_key_success_rate_threshold":0.6}`,
	}}
	svc := NewAccountInspectionService(nil, nil, settingsRepo)
	settings, err := svc.GetSettings(context.Background())
	require.NoError(t, err)
	require.False(t, settings.OAuthAutoDisable)
	require.True(t, settings.APIKeyAutoDisable)
	require.Equal(t, 3, settings.OAuthMinRequests)
	require.Equal(t, 1, settings.APIKeyMinRequests)
	require.Equal(t, 500, settings.OAuthTTFTThresholdMs)
	require.Equal(t, 30_000, settings.APIKeyTTFTThresholdMs)
	require.InDelta(t, 0.9, settings.OAuthSuccessRateThreshold, 1e-9)
	require.InDelta(t, 0.6, settings.APIKeySuccessRateThreshold, 1e-9)
}

func TestAccountInspectionRejectsInvalidProtectedAccountIDs(t *testing.T) {
	settingsRepo := &inspectionSettingRepoStub{values: map[string]string{}}
	svc := NewAccountInspectionService(nil, nil, settingsRepo)
	settings := DefaultAccountInspectionSettings()
	settings.ProtectedAccountIDs = []int64{0}
	_, err := svc.UpdateSettings(context.Background(), &settings)
	require.Error(t, err)
	require.Contains(t, err.Error(), "protected_account_ids")
}

func TestEvaluateAccountInspectionUsesIndependentTypeThresholds(t *testing.T) {
	now := time.Now().UTC()
	avg := 2_000.0
	stats := &usagestats.AccountHourlyUsageStats{TotalRequests: 2, SuccessfulRequests: 1, SuccessRate: 0.5, AvgFirstTokenMs: &avg}
	settings := DefaultAccountInspectionSettings()
	settings.OAuthTTFTThresholdMs = 1_000
	settings.OAuthSuccessRateThreshold = 0.9
	settings.APIKeyTTFTThresholdMs = 3_000
	settings.APIKeySuccessRateThreshold = 0.4

	oauth := evaluateAccountInspection(&Account{ID: 21, Name: "oauth", Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}, stats, settings, now)
	apiKey := evaluateAccountInspection(&Account{ID: 22, Name: "key", Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true}, stats, settings, now)
	require.ElementsMatch(t, []string{"first_token_over_threshold", "success_rate_below_threshold"}, oauth.Reasons)
	require.Empty(t, apiKey.Reasons)
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

func TestAccountInspectionHealthyResultsSerializeEmptyReasonsArray(t *testing.T) {
	now := time.Now().UTC()
	stats := &usagestats.AccountHourlyUsageStats{TotalRequests: 2, SuccessfulRequests: 2, SuccessRate: 1}
	account := &Account{ID: 9, Name: "healthy", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true}

	result := evaluateAccountInspection(account, stats, DefaultAccountInspectionSettings(), now)
	require.NotNil(t, result.Reasons)
	require.Empty(t, result.Reasons)

	payload, err := json.Marshal(result)
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	require.Equal(t, []any{}, decoded["reasons"])
}

func TestAccountInspectionLoadStateNormalizesMissingReasons(t *testing.T) {
	settingsRepo := &inspectionSettingRepoStub{values: map[string]string{
		SettingKeyAccountInspectionState: `{"status":"succeeded","results":[{"account_id":9,"name":"healthy"}]}`,
	}}
	svc := NewAccountInspectionService(nil, nil, settingsRepo)

	state, err := svc.loadState(context.Background())
	require.NoError(t, err)
	require.Len(t, state.Results, 1)
	require.NotNil(t, state.Results[0].Reasons)
	require.Empty(t, state.Results[0].Reasons)
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

func TestAccountInspectionAPIKeyQuotaUsageUsesHighestActiveWindow(t *testing.T) {
	now := time.Now().UTC()
	account := &Account{Extra: map[string]any{
		"quota_limit":        100.0,
		"quota_used":         25.0,
		"quota_daily_limit":  10.0,
		"quota_daily_used":   7.0,
		"quota_daily_start":  now.Add(-time.Hour).Format(time.RFC3339),
		"quota_weekly_limit": 20.0,
		"quota_weekly_used":  18.0,
		"quota_weekly_start": now.Add(-8 * 24 * time.Hour).Format(time.RFC3339),
	}}

	usedPercent, dimension := accountInspectionAPIKeyQuotaUsage(account)
	require.NotNil(t, usedPercent)
	require.InDelta(t, 70.0, *usedPercent, 1e-9)
	require.Equal(t, "daily", dimension)
}

func TestAccountInspectionAPIKeyQuotaUsageRetainsOverage(t *testing.T) {
	account := &Account{Extra: map[string]any{
		"quota_limit": 10.0,
		"quota_used":  12.5,
	}}

	usedPercent, dimension := accountInspectionAPIKeyQuotaUsage(account)
	require.NotNil(t, usedPercent)
	require.InDelta(t, 125.0, *usedPercent, 1e-9)
	require.Equal(t, "total", dimension)
}

func TestAccountInspectionOAuthQuotaUsageUsesHighestActiveWindow(t *testing.T) {
	now := time.Now().UTC()
	account := &Account{Extra: map[string]any{
		"codex_5h_used_percent":        95.0,
		"codex_5h_reset_at":            now.Add(-time.Minute).Format(time.RFC3339),
		"codex_7d_used_percent":        65.0,
		"codex_7d_reset_at":            now.Add(time.Hour).Format(time.RFC3339),
		"passive_usage_7d_utilization": 0.8,
		"passive_usage_7d_reset":       now.Add(2 * time.Hour).Unix(),
	}}

	usedPercent, dimension := accountInspectionOAuthQuotaUsage(account, now)
	require.NotNil(t, usedPercent)
	require.InDelta(t, 80.0, *usedPercent, 1e-9)
	require.Equal(t, "passive_7d", dimension)
}

func TestAccountInspectionQuotaDistributionBucketsAndAverage(t *testing.T) {
	values := []float64{0, 19.9, 20, 39.9, 40, 69.9, 70, 89.9, 90, 99.9, 100, 125}
	results := make([]AccountInspectionAccountResult, 0, len(values)+2)
	for _, value := range values {
		usedPercent := value
		results = append(results, AccountInspectionAccountResult{QuotaUsedPercent: &usedPercent})
	}
	nan := math.NaN()
	infinity := math.Inf(1)
	results = append(results,
		AccountInspectionAccountResult{},
		AccountInspectionAccountResult{},
		AccountInspectionAccountResult{QuotaUsedPercent: &nan},
		AccountInspectionAccountResult{QuotaUsedPercent: &infinity},
	)

	distribution := summarizeAccountInspectionQuotaDistribution(results)
	require.Equal(t, len(values), distribution.MeasuredAccounts)
	require.Equal(t, 4, distribution.UnknownAccounts)
	require.NotNil(t, distribution.AverageUsedPercent)
	require.InDelta(t, 764.5/float64(len(values)), *distribution.AverageUsedPercent, 1e-9)
	require.Equal(t, []string{"0_20", "20_40", "40_70", "70_90", "90_100", "over_100"}, []string{
		distribution.Buckets[0].Key,
		distribution.Buckets[1].Key,
		distribution.Buckets[2].Key,
		distribution.Buckets[3].Key,
		distribution.Buckets[4].Key,
		distribution.Buckets[5].Key,
	})
	require.Equal(t, []int{2, 2, 2, 2, 3, 1}, []int{
		distribution.Buckets[0].Count,
		distribution.Buckets[1].Count,
		distribution.Buckets[2].Count,
		distribution.Buckets[3].Count,
		distribution.Buckets[4].Count,
		distribution.Buckets[5].Count,
	})
}

func TestAccountInspectionRunPersistsSnapshotAndDisablesFlaggedAccounts(t *testing.T) {
	settingsRepo := &inspectionSettingRepoStub{values: map[string]string{}}
	now := time.Now().UTC()
	accountRepo := &inspectionAccountRepoStub{accounts: []Account{
		{ID: 1, Name: "slow", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Extra: map[string]any{
			"codex_5h_used_percent": 20.0,
			"codex_5h_reset_at":     now.Add(time.Hour).Format(time.RFC3339),
		}},
		{ID: 2, Name: "healthy", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Extra: map[string]any{
			"codex_7d_used_percent": 80.0,
			"codex_7d_reset_at":     now.Add(time.Hour).Format(time.RFC3339),
		}},
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
	require.Equal(t, 2, state.Summary.QuotaUsageDistribution.MeasuredAccounts)
	require.Equal(t, 0, state.Summary.QuotaUsageDistribution.UnknownAccounts)
	require.InDelta(t, 50.0, *state.Summary.QuotaUsageDistribution.AverageUsedPercent, 1e-9)

	overview, err := svc.GetOverview(context.Background(), AccountInspectionListFilter{Page: 1, PageSize: 10, Status: "flagged"})
	require.NoError(t, err)
	require.Len(t, overview.Results.Items, 1)
	require.Equal(t, int64(1), overview.Results.Items[0].AccountID)
	require.Equal(t, state.Summary.QuotaUsageDistribution, overview.Run.Summary.QuotaUsageDistribution)
}

func TestAccountInspectionRunProtectsConfiguredAccounts(t *testing.T) {
	settingsRepo := &inspectionSettingRepoStub{values: map[string]string{}}
	accountRepo := &inspectionAccountRepoStub{accounts: []Account{
		{ID: 31, Name: "protected", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true},
		{ID: 32, Name: "ordinary", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true},
	}}
	avg := 30_001.0
	usageRepo := &inspectionUsageRepoStub{stats: map[int64]*usagestats.AccountHourlyUsageStats{
		31: {TotalRequests: 2, SuccessfulRequests: 2, SuccessRate: 1, AvgFirstTokenMs: &avg},
		32: {TotalRequests: 2, SuccessfulRequests: 2, SuccessRate: 1, AvgFirstTokenMs: &avg},
	}}
	usageService := NewAccountUsageService(nil, usageRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	svc := NewAccountInspectionService(accountRepo, usageService, settingsRepo)
	settings := DefaultAccountInspectionSettings()
	settings.ProtectedAccountIDs = []int64{31, 31}
	_, err := svc.UpdateSettings(context.Background(), &settings)
	require.NoError(t, err)
	state, err := svc.RunNow(context.Background(), "manual")
	require.NoError(t, err)
	require.Equal(t, []int64{32}, accountRepo.updated)
	require.Equal(t, 1, state.Summary.Disabled)
	require.Equal(t, 1, state.Summary.Protected)
	byID := make(map[int64]AccountInspectionAccountResult, len(state.Results))
	for _, result := range state.Results {
		byID[result.AccountID] = result
	}
	require.Equal(t, AccountInspectionActionProtected, byID[31].Action)
	require.Equal(t, AccountInspectionActionDisabled, byID[32].Action)
}

func TestAccountInspectionRunSeparatesAutoDisableByType(t *testing.T) {
	settingsRepo := &inspectionSettingRepoStub{values: map[string]string{}}
	accountRepo := &inspectionAccountRepoStub{accounts: []Account{
		{ID: 41, Name: "oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true},
		{ID: 42, Name: "key", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true},
	}}
	avg := 30_001.0
	usageRepo := &inspectionUsageRepoStub{stats: map[int64]*usagestats.AccountHourlyUsageStats{
		41: {TotalRequests: 2, SuccessfulRequests: 2, SuccessRate: 1, AvgFirstTokenMs: &avg},
		42: {TotalRequests: 2, SuccessfulRequests: 2, SuccessRate: 1, AvgFirstTokenMs: &avg},
	}}
	usageService := NewAccountUsageService(nil, usageRepo, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	svc := NewAccountInspectionService(accountRepo, usageService, settingsRepo)
	settings := DefaultAccountInspectionSettings()
	settings.APIKeyAutoDisable = false
	_, err := svc.UpdateSettings(context.Background(), &settings)
	require.NoError(t, err)
	state, err := svc.RunNow(context.Background(), "manual")
	require.NoError(t, err)
	require.Equal(t, []int64{41}, accountRepo.updated)
	byID := make(map[int64]AccountInspectionAccountResult, len(state.Results))
	for _, result := range state.Results {
		byID[result.AccountID] = result
	}
	require.Equal(t, AccountInspectionActionDisabled, byID[41].Action)
	require.Equal(t, AccountInspectionActionReported, byID[42].Action)
}

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func cpaTestAccount(serverURL string, concurrency, perCredential int) *Account {
	return &Account{
		ID:          42,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: concurrency,
		Credentials: map[string]any{
			CPAModeCredentialKey:                     true,
			CPAManagementURLCredentialKey:            serverURL,
			CPAManagementKeyCredentialKey:            "management-secret",
			CPAConcurrencyPerCredentialCredentialKey: perCredential,
		},
	}
}

func TestCPAPoolCapacityAbnormalExclusionIsIndependentAndOffByDefault(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Hour)
	past := now.Add(-time.Minute)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, "/v0/management/auth-files", r.URL.Path)
		require.Equal(t, "Bearer management-secret", r.Header.Get("Authorization"))
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"files": []any{
			map[string]any{"status": "active", "disabled": false, "unavailable": false},
			map[string]any{"status": "active", "disabled": true, "unavailable": false},
			map[string]any{"status": "disabled", "disabled": false, "unavailable": false},
			map[string]any{"status": "error", "disabled": false, "unavailable": true, "next_retry_after": future},
			map[string]any{"status": "error", "disabled": false, "unavailable": true, "next_retry_after": past},
		}}))
	}))
	defer server.Close()

	service := newCPAPoolCapacityService()
	service.now = func() time.Time { return now }
	account := cpaTestAccount(server.URL, 10, 2)

	effective, available := service.effectiveConcurrency(context.Background(), account)
	require.True(t, available)
	require.Equal(t, 6, effective)
	require.Equal(t, int64(1), requests.Load())

	// CPA capacity replaces the configured fallback concurrency while enabled.
	account.Concurrency = 1
	effective, available = service.effectiveConcurrency(context.Background(), account)
	require.True(t, available)
	require.Equal(t, 6, effective)
	require.Equal(t, int64(1), requests.Load())

	status, err := service.capacityStatus(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, 5, status.TotalCredentials)
	require.Equal(t, 3, status.EnabledCredentials)
	require.Equal(t, 2, status.AbnormalCredentials)
	require.Equal(t, 1, status.AvailableCredentials)
	require.Equal(t, 3, status.CapacityCredentials)
	require.False(t, status.ExcludeAbnormalCredentials)
	require.Equal(t, 6, status.EffectiveConcurrency)
	require.Equal(t, CPACapacityStateFresh, status.State)

	account.Credentials[CPAExcludeAbnormalCredentialsCredentialKey] = true
	effective, available = service.effectiveConcurrency(context.Background(), account)
	require.True(t, available)
	require.Equal(t, 2, effective)
	status, err = service.capacityStatus(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, 1, status.AvailableCredentials)
	require.Equal(t, 1, status.CapacityCredentials)
	require.True(t, status.ExcludeAbnormalCredentials)
	require.Equal(t, 2, status.EffectiveConcurrency)
	require.Equal(t, CPACapacityStateFresh, status.State)
	require.Equal(t, int64(1), requests.Load(), "changing the policy must reuse the raw capacity snapshot")
}

func TestCPAPoolCapacityDefaultDoesNotDropAllAbnormalCredentialsToZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"files":[{"status":"error","unavailable":true},{"status":"error","unavailable":true}]}`))
	}))
	defer server.Close()

	service := newCPAPoolCapacityService()
	account := cpaTestAccount(server.URL, 100, 10)

	effective, available := service.effectiveConcurrency(context.Background(), account)
	require.True(t, available)
	require.Equal(t, 20, effective)

	account.Credentials[CPAExcludeAbnormalCredentialsCredentialKey] = true
	effective, available = service.effectiveConcurrency(context.Background(), account)
	require.False(t, available)
	require.Zero(t, effective)
}

func TestCPAPoolCapacitySingleflightAndNinetySecondCache(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		time.Sleep(25 * time.Millisecond)
		_, _ = w.Write([]byte(`{"files":[{"status":"active"}]}`))
	}))
	defer server.Close()

	service := newCPAPoolCapacityService()
	account := cpaTestAccount(server.URL, 100, 1)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for range 64 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			effective, available := service.effectiveConcurrency(context.Background(), account)
			require.True(t, available)
			require.Equal(t, 1, effective)
		}()
	}
	close(start)
	wait.Wait()
	require.Equal(t, int64(1), requests.Load())

	effective, available := service.effectiveConcurrency(context.Background(), account)
	require.True(t, available)
	require.Equal(t, 1, effective)
	require.Equal(t, int64(1), requests.Load())
}

func TestCPAPoolCapacityUsesStaleSnapshotOnceThenFailsClosed(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	var requests atomic.Int64
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		if fail.Load() {
			http.Error(w, "management unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"files":[{"status":"active"}]}`))
	}))
	defer server.Close()

	service := newCPAPoolCapacityService()
	service.now = func() time.Time { return now }
	account := cpaTestAccount(server.URL, 10, 1)

	effective, available := service.effectiveConcurrency(context.Background(), account)
	require.True(t, available)
	require.Equal(t, 1, effective)

	fail.Store(true)
	now = now.Add(91 * time.Second)
	effective, available = service.effectiveConcurrency(context.Background(), account)
	require.True(t, available)
	require.Equal(t, 1, effective)
	require.Equal(t, int64(2), requests.Load())

	now = now.Add(91 * time.Second)
	effective, available = service.effectiveConcurrency(context.Background(), account)
	require.False(t, available)
	require.Zero(t, effective)
	require.Equal(t, int64(3), requests.Load())
}

func TestGatewayNewSelectionResultReleasesAcquiredSlotWhenCPAPoolIsEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"files":[]}`))
	}))
	defer server.Close()

	gateway := &GatewayService{concurrencyService: NewConcurrencyService(nil)}
	released := atomic.Bool{}
	selection, err := gateway.newSelectionResult(
		context.Background(),
		cpaTestAccount(server.URL, 10, 1),
		true,
		func() { released.Store(true) },
		nil,
	)
	require.ErrorIs(t, err, errCPAPoolCapacityUnavailable)
	require.Nil(t, selection)
	require.True(t, released.Load())
}

func TestGatewayNewSelectionResultRefreshesWaitPlanConcurrency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"files":[{"status":"active"},{"status":"active"}]}`))
	}))
	defer server.Close()

	gateway := &GatewayService{concurrencyService: NewConcurrencyService(nil)}
	waitPlan := &AccountWaitPlan{AccountID: 42, MaxConcurrency: 10, Timeout: time.Second, MaxWaiting: 5}
	selection, err := gateway.newSelectionResult(
		context.Background(),
		cpaTestAccount(server.URL, 10, 1),
		false,
		nil,
		waitPlan,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, 2, selection.Account.Concurrency)
	require.Equal(t, 2, selection.WaitPlan.MaxConcurrency)
	require.Equal(t, 10, waitPlan.MaxConcurrency)
}

func TestApplyCPAPoolCapacityBatchReturnsOriginalSliceWhenUnchanged(t *testing.T) {
	service := NewConcurrencyService(nil)
	accounts := []Account{
		{ID: 1, Type: AccountTypeAPIKey, Concurrency: 10},
		{ID: 2, Type: AccountTypeOAuth, Concurrency: 20},
	}

	result := service.applyCPAPoolCapacityBatch(context.Background(), accounts)

	require.Len(t, result, len(accounts))
	require.True(t, &result[0] == &accounts[0], "unchanged batches must reuse the original backing array")
}

func TestApplyCPAPoolCapacityBatchCopiesOnlyWhenCapacityChanges(t *testing.T) {
	service := NewConcurrencyService(nil)
	cpaAccount := cpaTestAccount("https://cpa.example.com", 10, 1)
	config, enabled := cpaPoolConfigFromAccount(cpaAccount)
	require.True(t, enabled)
	service.cpaPoolCapacity.cache.Store(service.cpaPoolCapacity.cacheKey(config), cpaCapacityCacheEntry{
		snapshot: &cpaCapacitySnapshot{totalCredentials: 2, enabledCredentials: 2, availableCredentials: 2, fetchedAt: time.Now()},
	})
	accounts := []Account{
		{ID: 1, Type: AccountTypeAPIKey, Concurrency: 5},
		*cpaAccount,
		{ID: 3, Type: AccountTypeAPIKey, Concurrency: 8},
	}

	result := service.applyCPAPoolCapacityBatch(context.Background(), accounts)

	require.Len(t, result, len(accounts))
	require.False(t, &result[0] == &accounts[0], "changed batches must not mutate the source backing array")
	require.Equal(t, 2, result[1].Concurrency)
	require.Equal(t, int64(3), result[2].ID)
	require.Equal(t, 10, accounts[1].Concurrency)
}

func TestNormalizeCPACredentials(t *testing.T) {
	credentials := map[string]any{
		CPAModeCredentialKey:                       true,
		CPAManagementURLCredentialKey:              " https://cpa.example.com/ ",
		CPAManagementKeyCredentialKey:              " secret ",
		CPAConcurrencyPerCredentialCredentialKey:   float64(3),
		CPAExcludeAbnormalCredentialsCredentialKey: "true",
	}
	require.NoError(t, NormalizeCPACredentials(AccountTypeAPIKey, credentials))
	require.Equal(t, "https://cpa.example.com", credentials[CPAManagementURLCredentialKey])
	require.Equal(t, "secret", credentials[CPAManagementKeyCredentialKey])
	require.Equal(t, 3, credentials[CPAConcurrencyPerCredentialCredentialKey])
	require.Equal(t, true, credentials[CPAExcludeAbnormalCredentialsCredentialKey])
	require.True(t, IsSensitiveCredentialKey(CPAManagementKeyCredentialKey))

	credentials[CPAModeCredentialKey] = false
	require.NoError(t, NormalizeCPACredentials(AccountTypeAPIKey, credentials))
	require.NotContains(t, credentials, CPAModeCredentialKey)
	require.NotContains(t, credentials, CPAManagementURLCredentialKey)
	require.NotContains(t, credentials, CPAManagementKeyCredentialKey)
	require.NotContains(t, credentials, CPAConcurrencyPerCredentialCredentialKey)
	require.NotContains(t, credentials, CPAExcludeAbnormalCredentialsCredentialKey)
}

func TestNormalizeCPACredentialsFallsBackToBaseURLAndDefaultsToTen(t *testing.T) {
	credentials := map[string]any{
		CPAModeCredentialKey:          true,
		"base_url":                    "https://cpa.example.com/v1",
		CPAManagementKeyCredentialKey: " secret ",
	}
	require.NoError(t, NormalizeCPACredentials(AccountTypeAPIKey, credentials))
	require.NotContains(t, credentials, CPAManagementURLCredentialKey)
	require.Equal(t, DefaultCPAConcurrencyPerCredential, credentials[CPAConcurrencyPerCredentialCredentialKey])
	require.NotContains(t, credentials, CPAExcludeAbnormalCredentialsCredentialKey)
	account := &Account{Type: AccountTypeAPIKey, Credentials: credentials}
	config, enabled := cpaPoolConfigFromAccount(account)
	require.True(t, enabled)
	require.False(t, config.excludeAbnormalCredentials)
	require.Equal(t, "https://cpa.example.com/v1", config.managementURL)
	require.Equal(t, "https://cpa.example.com/v0/management/auth-files", cpaAuthFilesURL(config.managementURL))
}

func TestNormalizeCPACredentialsDropsAbnormalPolicyOutsideCPAMode(t *testing.T) {
	credentials := map[string]any{
		"base_url": "https://api.example.com/v1",
		CPAExcludeAbnormalCredentialsCredentialKey: true,
	}

	require.NoError(t, NormalizeCPACredentials(AccountTypeAPIKey, credentials))
	require.NotContains(t, credentials, CPAExcludeAbnormalCredentialsCredentialKey)
}

func TestCPAPoolCapacityForceSnapshotBypassesFreshCache(t *testing.T) {
	var available atomic.Int64
	available.Store(1)
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		files := make([]map[string]any, available.Load())
		for index := range files {
			files[index] = map[string]any{"status": "active"}
		}
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"files": files}))
	}))
	defer server.Close()

	capacityService := newCPAPoolCapacityService()
	account := cpaTestAccount(server.URL, 1, 10)
	status, err := capacityService.capacityStatus(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, 10, status.EffectiveConcurrency)

	available.Store(3)
	config, enabled := cpaPoolConfigFromAccount(account)
	require.True(t, enabled)
	snapshot, err := capacityService.forceSnapshot(context.Background(), config)
	require.NoError(t, err)
	status = cpaCapacityFromSnapshot(snapshot, config, CPACapacityStateFresh)
	require.Equal(t, 3, status.AvailableCredentials)
	require.Equal(t, 3, status.CapacityCredentials)
	require.Equal(t, 30, status.EffectiveConcurrency)
	require.Equal(t, int64(2), requests.Load())
}

func TestCPAPoolCapacityTestDoesNotPopulateSchedulerCache(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"files":[{"status":"active"}]}`))
	}))
	defer server.Close()

	concurrencyService := NewConcurrencyService(nil)
	account := cpaTestAccount(server.URL, 1, 10)
	result, err := concurrencyService.TestCPACapacity(context.Background(), account, CPATestInput{})
	require.NoError(t, err)
	require.Equal(t, 10, result.EffectiveConcurrency)
	require.Equal(t, int64(1), requests.Load())

	status, err := concurrencyService.GetCPACapacityStatus(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, 10, status.EffectiveConcurrency)
	require.Equal(t, int64(2), requests.Load(), "connection tests must not warm the scheduler cache")
}

func TestCPAPoolCapacityFailedForceRefreshPreservesStaleSnapshot(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	var fail atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"files":[{"status":"active"},{"status":"active"}]}`))
	}))
	defer server.Close()

	concurrencyService := NewConcurrencyService(nil)
	concurrencyService.cpaPoolCapacity.now = func() time.Time { return now }
	account := cpaTestAccount(server.URL, 1, 10)
	status, err := concurrencyService.GetCPACapacityStatus(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, 20, status.EffectiveConcurrency)

	fail.Store(true)
	now = now.Add(time.Second)
	_, err = concurrencyService.ForceRefreshCPACapacity(context.Background(), account)
	require.Error(t, err)

	status, err = concurrencyService.GetCPACapacityStatus(context.Background(), account)
	require.NoError(t, err)
	require.Equal(t, CPACapacityStateStale, status.State)
	require.Equal(t, 2, status.AvailableCredentials)
	require.Equal(t, 20, status.EffectiveConcurrency)
}

func TestCPATestCapacityFailureDoesNotExposeAdministratorPassword(t *testing.T) {
	const administratorPassword = "do-not-leak-this-password"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	account := cpaTestAccount(server.URL, 1, 10)
	account.Credentials[CPAManagementKeyCredentialKey] = administratorPassword
	_, err := NewConcurrencyService(nil).TestCPACapacity(context.Background(), account, CPATestInput{})
	require.Error(t, err)
	require.NotContains(t, err.Error(), administratorPassword)
}

func TestNormalizeCPACredentialsRejectsIncompleteConfig(t *testing.T) {
	tests := []map[string]any{
		{CPAModeCredentialKey: true},
		{CPAModeCredentialKey: true, CPAManagementURLCredentialKey: "ftp://cpa.example.com", CPAManagementKeyCredentialKey: "secret"},
		{CPAModeCredentialKey: true, CPAManagementURLCredentialKey: "https://cpa.example.com", CPAManagementKeyCredentialKey: "secret", CPAConcurrencyPerCredentialCredentialKey: 0},
		{CPAModeCredentialKey: true, CPAManagementURLCredentialKey: "https://cpa.example.com", CPAManagementKeyCredentialKey: "secret", CPAExcludeAbnormalCredentialsCredentialKey: "sometimes"},
	}
	for _, credentials := range tests {
		require.Error(t, NormalizeCPACredentials(AccountTypeAPIKey, credentials))
	}
	require.Error(t, NormalizeCPACredentials(AccountTypeOAuth, map[string]any{
		CPAModeCredentialKey: true, CPAManagementURLCredentialKey: "https://cpa.example.com", CPAManagementKeyCredentialKey: "secret",
	}))
}

var cpaPoolCapacityBenchmarkAccountsSink []Account
var cpaPoolCapacityBenchmarkAccountSink *Account

func BenchmarkCPAPoolCapacityBatchNoCPA(b *testing.B) {
	for _, count := range []int{1, 64, 1024, 3131} {
		b.Run(fmt.Sprintf("accounts_%d", count), func(b *testing.B) {
			service := NewConcurrencyService(nil)
			accounts := make([]Account, count)
			for index := range accounts {
				accounts[index] = Account{
					ID:          int64(index + 1),
					Type:        AccountTypeAPIKey,
					Concurrency: 20,
				}
			}
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				cpaPoolCapacityBenchmarkAccountsSink = service.applyCPAPoolCapacityBatch(ctx, accounts)
			}
		})
	}
}

func BenchmarkCPAPoolCapacityCachedHit(b *testing.B) {
	service := NewConcurrencyService(nil)
	account := cpaTestAccount("https://cpa.example.com", 20, 1)
	config, enabled := cpaPoolConfigFromAccount(account)
	if !enabled {
		b.Fatal("CPA configuration was not enabled")
	}
	service.cpaPoolCapacity.cache.Store(service.cpaPoolCapacity.cacheKey(config), cpaCapacityCacheEntry{
		snapshot: &cpaCapacitySnapshot{totalCredentials: 20, enabledCredentials: 20, availableCredentials: 20, fetchedAt: time.Now()},
	})
	ctx := context.Background()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		result, available := service.applyCPAPoolCapacity(ctx, account)
		if !available || result == nil {
			b.Fatal("cached CPA capacity was unavailable")
		}
		cpaPoolCapacityBenchmarkAccountSink = result
	}
}

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type oauthModelSyncRepoStub struct {
	accounts []Account

	mu      sync.Mutex
	updates map[int64]map[string]any
}

func (r *oauthModelSyncRepoStub) ListByPlatform(_ context.Context, platform string) ([]Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	filtered := make([]Account, 0, len(r.accounts))
	for _, account := range r.accounts {
		if account.Platform == platform {
			filtered = append(filtered, account)
		}
	}
	return filtered, nil
}

func (r *oauthModelSyncRepoStub) UpdateExtra(_ context.Context, id int64, updates map[string]any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.updates == nil {
		r.updates = make(map[int64]map[string]any)
	}
	r.updates[id] = updates
	return nil
}

type oauthModelSyncFetcherStub struct {
	mu       sync.Mutex
	models   map[int64][]string
	errors   map[int64]error
	accounts []int64
}

func (f *oauthModelSyncFetcherStub) FetchUpstreamSupportedModels(_ context.Context, account *Account) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.accounts = append(f.accounts, account.ID)
	if err := f.errors[account.ID]; err != nil {
		return nil, err
	}
	return append([]string(nil), f.models[account.ID]...), nil
}

type oauthModelSyncCacheStub struct {
	mu    sync.Mutex
	calls int
}

func (c *oauthModelSyncCacheStub) InvalidateAvailableModelsCache(_ *int64, _ string) {
	c.mu.Lock()
	c.calls++
	c.mu.Unlock()
}

func TestOAuthModelSyncService_RunOnceUpdatesOnlyUnrestrictedOAuthAccounts(t *testing.T) {
	accounts := []Account{
		{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive},
		{ID: 6, Platform: PlatformAnthropic, Type: AccountTypeOAuth, Status: StatusActive},
		{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Credentials: map[string]any{
			"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"},
		}},
		{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Extra: map[string]any{
			"openai_passthrough": true,
		}},
		{ID: 4, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive},
		{ID: 5, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusDisabled},
	}
	repo := &oauthModelSyncRepoStub{accounts: accounts}
	fetcher := &oauthModelSyncFetcherStub{models: map[int64][]string{
		1: {"gpt-5.5", "gpt-5.5", " gpt-5.4 "},
		6: {"claude-sonnet-4-6"},
	}}
	cache := &oauthModelSyncCacheStub{}
	service := NewOAuthModelSyncService(repo, fetcher, OAuthModelSyncOptions{
		Enabled:        true,
		AccountTimeout: time.Second,
		MaxConcurrent:  2,
		CycleTimeout:   5 * time.Second,
	})
	service.SetModelsCacheInvalidator(cache)

	stats := service.RunOnce(context.Background())
	require.Equal(t, 2, stats.Considered)
	require.Equal(t, 2, stats.Updated)
	require.Equal(t, 2, stats.SkippedExplicit)
	require.Equal(t, 1, stats.SkippedInactive)
	require.Empty(t, stats.Failed)
	require.Contains(t, repo.updates[1], OAuthSupportedModelsExtraKey)
	require.Equal(t, []string{"gpt-5.4", "gpt-5.5"}, repo.updates[1][OAuthSupportedModelsExtraKey])
	require.Equal(t, []string{"claude-sonnet-4-6"}, repo.updates[6][OAuthSupportedModelsExtraKey])
	require.Equal(t, len(oauthModelSyncPlatforms), cache.calls)
}

func TestOAuthModelSyncService_RunOnceKeepsPreviousSnapshotAfterFailure(t *testing.T) {
	repo := &oauthModelSyncRepoStub{accounts: []Account{{
		ID: 9, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Extra: map[string]any{OpenAIOAuthSupportedModelsExtraKey: []any{"gpt-5.4"}},
	}}}
	fetcher := &oauthModelSyncFetcherStub{errors: map[int64]error{9: errors.New("429")}}
	service := NewOAuthModelSyncService(repo, fetcher, OAuthModelSyncOptions{Enabled: true, CycleTimeout: time.Second})

	stats := service.RunOnce(context.Background())
	require.Equal(t, 1, stats.Considered)
	require.Equal(t, 1, stats.Failed)
	require.Empty(t, repo.updates)
	require.Equal(t, []string{"gpt-5.4"}, normalizeOAuthSupportedModelValues(repo.accounts[0].Extra[OpenAIOAuthSupportedModelsExtraKey]))
}

func TestOAuthModelSyncService_StartAndStopAreIdempotent(t *testing.T) {
	repo := &oauthModelSyncRepoStub{accounts: []Account{{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive}}}
	fetcher := &oauthModelSyncFetcherStub{models: map[int64][]string{1: {"gpt-5.4"}}}
	service := NewOAuthModelSyncService(repo, fetcher, OAuthModelSyncOptions{
		Enabled:        true,
		Interval:       time.Hour,
		AccountTimeout: time.Second,
		CycleTimeout:   time.Second,
	})
	service.Start()
	service.Start()
	deadline := time.Now().Add(time.Second)
	for {
		fetcher.mu.Lock()
		calls := len(fetcher.accounts)
		fetcher.mu.Unlock()
		if calls > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	service.Stop()
	service.Stop()
	fetcher.mu.Lock()
	defer fetcher.mu.Unlock()
	require.Len(t, fetcher.accounts, 1)
}

func TestOAuthModelSyncService_RunOnceWithoutDependenciesIsNoop(t *testing.T) {
	service := NewOAuthModelSyncService(nil, nil, OAuthModelSyncOptions{Enabled: true})
	require.Equal(t, OAuthModelSyncStats{}, service.RunOnce(context.Background()))
}

func TestIsOAuthModelSyncEligibleAccount(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		want    bool
	}{
		{name: "openai oauth", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, want: true},
		{name: "openai setup token", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken}, want: false},
		{name: "anthropic setup token", account: &Account{Platform: PlatformAnthropic, Type: AccountTypeSetupToken}, want: true},
		{name: "gemini setup token", account: &Account{Platform: PlatformGemini, Type: AccountTypeSetupToken}, want: false},
		{name: "gemini code assist", account: &Account{Platform: PlatformGemini, Type: AccountTypeOAuth, Credentials: map[string]any{"project_id": "project"}}, want: false},
		{name: "api key", account: &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, isOAuthModelSyncEligibleAccount(tc.account))
		})
	}
}

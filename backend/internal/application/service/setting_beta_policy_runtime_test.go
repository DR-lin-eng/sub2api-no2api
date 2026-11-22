package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type betaPolicyRuntimeRepoStub struct {
	betaPolicySettingRepoStub
	mu     sync.Mutex
	value  string
	getErr error
	reads  int
}

func (s *betaPolicyRuntimeRepoStub) GetValue(_ context.Context, key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reads++
	if s.getErr != nil {
		return "", s.getErr
	}
	if key != SettingKeyBetaPolicySettings || s.value == "" {
		return "", ErrSettingNotFound
	}
	return s.value, nil
}

func (s *betaPolicyRuntimeRepoStub) Set(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key == SettingKeyBetaPolicySettings {
		s.value = value
	}
	return nil
}

func (s *betaPolicyRuntimeRepoStub) set(value string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.value, s.getErr = value, err
}

func (s *betaPolicyRuntimeRepoStub) readCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads
}

func betaPolicyRuntimeJSON(t *testing.T, token string) string {
	t.Helper()
	raw, err := json.Marshal(&BetaPolicySettings{Rules: []BetaPolicyRule{{
		BetaToken: token,
		Action:    BetaPolicyActionBlock,
		Scope:     BetaPolicyScopeAll,
	}}})
	require.NoError(t, err)
	return string(raw)
}

func expireBetaPolicyRuntimeCache(svc *SettingService) {
	svc.betaPolicyRuntimeLoaded.Store(time.Now().Add(-2 * betaPolicyRuntimeRefreshInterval).UnixNano())
}

func TestGetBetaPolicyRuntimeSettings_ReadsRepositoryOncePerInterval(t *testing.T) {
	repo := &betaPolicyRuntimeRepoStub{value: betaPolicyRuntimeJSON(t, "token-a")}
	svc := NewSettingService(repo, &config.Config{})

	for range 50 {
		settings, err := svc.GetBetaPolicyRuntimeSettings(context.Background())
		require.NoError(t, err)
		require.Len(t, settings.Rules, 1)
		require.Equal(t, "token-a", settings.Rules[0].BetaToken)
	}
	require.Equal(t, 1, repo.readCount())

	// A change written elsewhere is not visible before the interval elapses.
	repo.set(betaPolicyRuntimeJSON(t, "token-b"), nil)
	settings, err := svc.GetBetaPolicyRuntimeSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, "token-a", settings.Rules[0].BetaToken)
	require.Equal(t, 1, repo.readCount())

	expireBetaPolicyRuntimeCache(svc)
	settings, err = svc.GetBetaPolicyRuntimeSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, "token-b", settings.Rules[0].BetaToken)
	require.Equal(t, 2, repo.readCount())
}

func TestGetBetaPolicyRuntimeSettings_MissingSettingCachesDefault(t *testing.T) {
	repo := &betaPolicyRuntimeRepoStub{}
	svc := NewSettingService(repo, &config.Config{})

	for range 10 {
		settings, err := svc.GetBetaPolicyRuntimeSettings(context.Background())
		require.NoError(t, err)
		require.Equal(t, DefaultBetaPolicySettings(), settings)
	}
	require.Equal(t, 1, repo.readCount())
}

func TestSetBetaPolicySettings_PublishesToRuntimeCacheImmediately(t *testing.T) {
	repo := &betaPolicyRuntimeRepoStub{value: betaPolicyRuntimeJSON(t, "token-a")}
	svc := NewSettingService(repo, &config.Config{})

	_, err := svc.GetBetaPolicyRuntimeSettings(context.Background())
	require.NoError(t, err)

	update := &BetaPolicySettings{Rules: []BetaPolicyRule{{
		BetaToken: "token-b",
		Action:    BetaPolicyActionFilter,
		Scope:     BetaPolicyScopeOAuth,
	}}}
	require.NoError(t, svc.SetBetaPolicySettings(context.Background(), update))

	settings, err := svc.GetBetaPolicyRuntimeSettings(context.Background())
	require.NoError(t, err)
	require.Len(t, settings.Rules, 1)
	require.Equal(t, "token-b", settings.Rules[0].BetaToken)
	require.Equal(t, 1, repo.readCount())

	// The cached policy must not alias the struct the caller still holds.
	update.Rules[0].BetaToken = "mutated"
	settings, err = svc.GetBetaPolicyRuntimeSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, "token-b", settings.Rules[0].BetaToken)
}

func TestGetBetaPolicyRuntimeSettings_RefreshFailureKeepsLastPolicy(t *testing.T) {
	repo := &betaPolicyRuntimeRepoStub{value: betaPolicyRuntimeJSON(t, "token-a")}
	svc := NewSettingService(repo, &config.Config{})

	_, err := svc.GetBetaPolicyRuntimeSettings(context.Background())
	require.NoError(t, err)

	repo.set("", errors.New("db down"))
	expireBetaPolicyRuntimeCache(svc)
	for range 10 {
		settings, err := svc.GetBetaPolicyRuntimeSettings(context.Background())
		require.NoError(t, err)
		require.Equal(t, "token-a", settings.Rules[0].BetaToken)
	}
	// One failed refresh, then the interval applies again.
	require.Equal(t, 2, repo.readCount())
}

func TestGetBetaPolicyRuntimeSettings_FirstLoadFailureReturnsError(t *testing.T) {
	repo := &betaPolicyRuntimeRepoStub{getErr: errors.New("db down")}
	svc := NewSettingService(repo, &config.Config{})

	settings, err := svc.GetBetaPolicyRuntimeSettings(context.Background())
	require.Error(t, err)
	require.Nil(t, settings)

	// Nothing cached, so the next request retries and recovers.
	repo.set(betaPolicyRuntimeJSON(t, "token-a"), nil)
	settings, err = svc.GetBetaPolicyRuntimeSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, "token-a", settings.Rules[0].BetaToken)
}

func TestGetBetaPolicyRuntimeSettings_ConcurrentColdLoadSharesOneRead(t *testing.T) {
	repo := &betaPolicyRuntimeRepoStub{value: betaPolicyRuntimeJSON(t, "token-a")}
	svc := NewSettingService(repo, &config.Config{})

	var wg sync.WaitGroup
	for range 64 {
		wg.Go(func() {
			settings, err := svc.GetBetaPolicyRuntimeSettings(context.Background())
			if err != nil || len(settings.Rules) != 1 {
				t.Errorf("unexpected result: %v %v", settings, err)
			}
		})
	}
	wg.Wait()
	// Singleflight collapses overlapping loads; callers arriving after the first
	// load completes hit the cache.
	require.LessOrEqual(t, repo.readCount(), 2)
}

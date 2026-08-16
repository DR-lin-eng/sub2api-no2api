package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type supportChatSettingRepo struct {
	mu              sync.Mutex
	values          map[string]string
	errors          map[string]error
	getCalls        map[string]int
	getStarted      chan struct{}
	releaseGet      chan struct{}
	setMultipleDone chan struct{}
	getStartedOnce  sync.Once
	setDoneOnce     sync.Once
}

func newSupportChatSettingRepo(values map[string]string) *supportChatSettingRepo {
	return &supportChatSettingRepo{
		values:   values,
		errors:   make(map[string]error),
		getCalls: make(map[string]int),
	}
}

func (r *supportChatSettingRepo) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}

func (r *supportChatSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	r.getCalls[key]++
	err := r.errors[key]
	value, ok := r.values[key]
	getStarted := r.getStarted
	releaseGet := r.releaseGet
	r.mu.Unlock()
	if getStarted != nil {
		r.getStartedOnce.Do(func() { close(getStarted) })
	}
	if releaseGet != nil {
		<-releaseGet
	}
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *supportChatSettingRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	r.values[key] = value
	r.mu.Unlock()
	return nil
}

func (r *supportChatSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			values[key] = value
		}
	}
	return values, nil
}

func (r *supportChatSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	r.mu.Lock()
	for key, value := range settings {
		r.values[key] = value
	}
	r.mu.Unlock()
	if r.setMultipleDone != nil {
		r.setDoneOnce.Do(func() { close(r.setMultipleDone) })
	}
	return nil
}

func (r *supportChatSettingRepo) GetAll(context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	values := make(map[string]string, len(r.values))
	for key, value := range r.values {
		values[key] = value
	}
	return values, nil
}

func (r *supportChatSettingRepo) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	delete(r.values, key)
	r.mu.Unlock()
	return nil
}

func (r *supportChatSettingRepo) calls(key string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getCalls[key]
}

func TestIsSupportChatEnabledFailsClosedForMissingAndStorageErrors(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		repo := newSupportChatSettingRepo(map[string]string{})
		svc := NewSettingService(repo, &config.Config{})

		require.False(t, svc.IsSupportChatEnabled(context.Background()))
		require.Equal(t, 1, repo.calls(SettingKeySupportChatEnabled))
	})

	t.Run("storage error", func(t *testing.T) {
		repo := newSupportChatSettingRepo(map[string]string{})
		repo.errors[SettingKeySupportChatEnabled] = errors.New("database unavailable")
		svc := NewSettingService(repo, &config.Config{})

		require.False(t, svc.IsSupportChatEnabled(context.Background()))
		require.Equal(t, 1, repo.calls(SettingKeySupportChatEnabled))
	})
}

func TestIsSupportChatEnabledCachesExplicitTrue(t *testing.T) {
	repo := newSupportChatSettingRepo(map[string]string{SettingKeySupportChatEnabled: "true"})
	svc := NewSettingService(repo, &config.Config{})

	require.True(t, svc.IsSupportChatEnabled(context.Background()))
	require.True(t, svc.IsSupportChatEnabled(context.Background()))
	require.Equal(t, 1, repo.calls(SettingKeySupportChatEnabled))
}

func TestUpdateSettingsRefreshesSupportChatCacheImmediately(t *testing.T) {
	repo := newSupportChatSettingRepo(map[string]string{SettingKeySupportChatEnabled: "false"})
	svc := NewSettingService(repo, &config.Config{})

	require.False(t, svc.IsSupportChatEnabled(context.Background()))
	readsBeforeUpdate := repo.calls(SettingKeySupportChatEnabled)
	require.NoError(t, svc.UpdateSettings(context.Background(), &SystemSettings{SupportChatEnabled: true}))
	require.True(t, svc.IsSupportChatEnabled(context.Background()))
	require.Equal(t, readsBeforeUpdate, repo.calls(SettingKeySupportChatEnabled))
}

func TestUpdateSettingsCannotBeOverwrittenByStaleSupportChatRead(t *testing.T) {
	repo := newSupportChatSettingRepo(map[string]string{SettingKeySupportChatEnabled: "false"})
	repo.getStarted = make(chan struct{})
	repo.releaseGet = make(chan struct{})
	repo.setMultipleDone = make(chan struct{})
	svc := NewSettingService(repo, &config.Config{})

	staleRead := make(chan bool, 1)
	go func() {
		staleRead <- svc.IsSupportChatEnabled(context.Background())
	}()
	<-repo.getStarted

	updateDone := make(chan error, 1)
	go func() {
		updateDone <- svc.UpdateSettings(context.Background(), &SystemSettings{SupportChatEnabled: true})
	}()
	<-repo.setMultipleDone
	close(repo.releaseGet)

	require.False(t, <-staleRead)
	require.NoError(t, <-updateDone)
	require.True(t, svc.IsSupportChatEnabled(context.Background()))
}

func TestGetSupportChatRetentionDaysIsStrictAndUpgradeSafe(t *testing.T) {
	t.Run("missing means retain forever", func(t *testing.T) {
		svc := NewSettingService(newSupportChatSettingRepo(map[string]string{}), &config.Config{})
		days, err := svc.GetSupportChatRetentionDays(context.Background())
		require.NoError(t, err)
		require.Zero(t, days)
	})

	t.Run("valid persisted value", func(t *testing.T) {
		svc := NewSettingService(newSupportChatSettingRepo(map[string]string{
			SettingKeySupportChatRetentionDays: "30",
		}), &config.Config{})
		days, err := svc.GetSupportChatRetentionDays(context.Background())
		require.NoError(t, err)
		require.Equal(t, 30, days)
	})

	for _, raw := range []string{"invalid", "-1", "3651"} {
		t.Run(raw, func(t *testing.T) {
			svc := NewSettingService(newSupportChatSettingRepo(map[string]string{
				SettingKeySupportChatRetentionDays: raw,
			}), &config.Config{})
			_, err := svc.GetSupportChatRetentionDays(context.Background())
			require.Error(t, err, "a malformed destructive policy must fail without cleanup")
		})
	}
}

func TestUpdateSettingsNormalizesSupportChatRetentionDays(t *testing.T) {
	repo := newSupportChatSettingRepo(map[string]string{})
	svc := NewSettingService(repo, &config.Config{})
	settings := &SystemSettings{SupportChatRetentionDays: SupportChatRetentionDaysMax + 100}

	require.NoError(t, svc.UpdateSettings(context.Background(), settings))
	require.Equal(t, SupportChatRetentionDaysMax, settings.SupportChatRetentionDays)
	require.Equal(t, "3650", repo.values[SettingKeySupportChatRetentionDays])
}

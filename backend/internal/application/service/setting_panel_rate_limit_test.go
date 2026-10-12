package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type panelRateLimitSettingRepo struct {
	mu            sync.Mutex
	values        map[string]string
	getValueErr   error
	getValueCalls int
}

func (r *panelRateLimitSettingRepo) Get(_ context.Context, key string) (*Setting, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	value, ok := r.values[key]
	if !ok {
		return nil, ErrSettingNotFound
	}
	return &Setting{Key: key, Value: value}, nil
}

func (r *panelRateLimitSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.getValueCalls++
	if r.getValueErr != nil {
		return "", r.getValueErr
	}
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *panelRateLimitSettingRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *panelRateLimitSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (r *panelRateLimitSettingRepo) SetMultiple(_ context.Context, settings map[string]string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.values == nil {
		r.values = make(map[string]string)
	}
	for key, value := range settings {
		r.values[key] = value
	}
	return nil
}

func (r *panelRateLimitSettingRepo) GetAll(_ context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(r.values))
	for key, value := range r.values {
		out[key] = value
	}
	return out, nil
}

func (r *panelRateLimitSettingRepo) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.values, key)
	return nil
}

func newPanelRateLimitTestService(repo SettingRepository) *SettingService {
	return NewSettingService(repo, &config.Config{})
}

func TestGetPanelRateLimitSettingsDefaults(t *testing.T) {
	svc := newPanelRateLimitTestService(&panelRateLimitSettingRepo{})

	settings, err := svc.GetPanelRateLimitSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, DefaultPanelRateLimitSettings(), settings)
	require.True(t, settings.Enabled, "missing settings must enable abuse protection")
}

func TestGetPanelRateLimitSettingsInvalidJSONFallsBack(t *testing.T) {
	repo := &panelRateLimitSettingRepo{values: map[string]string{
		SettingKeyPanelRateLimitSettings: "{not-json",
	}}
	svc := newPanelRateLimitTestService(repo)

	settings, err := svc.GetPanelRateLimitSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, DefaultPanelRateLimitSettings(), settings)
}

func TestGetPanelRateLimitSettingsNormalizesValues(t *testing.T) {
	repo := &panelRateLimitSettingRepo{values: map[string]string{
		SettingKeyPanelRateLimitSettings: `{"enabled":true,"user_rpm":-5,"heavy_rpm":999999999,"exempt_admin":false,"public_ip_rpm":10}`,
	}}
	svc := newPanelRateLimitTestService(repo)

	settings, err := svc.GetPanelRateLimitSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 0, settings.UserRPM)
	require.Equal(t, panelRateLimitRPMMax, settings.HeavyRPM)
	require.Equal(t, 10, settings.PublicIPRPM)
	require.False(t, settings.ExemptAdmin)
}

func TestGetPanelRateLimitSettingsPartialJSONKeepsSafeDefaults(t *testing.T) {
	repo := &panelRateLimitSettingRepo{values: map[string]string{
		SettingKeyPanelRateLimitSettings: `{"enabled":true}`,
	}}
	svc := newPanelRateLimitTestService(repo)

	settings, err := svc.GetPanelRateLimitSettings(context.Background())
	require.NoError(t, err)
	require.True(t, settings.Enabled)
	require.Equal(t, 240, settings.UserRPM)
	require.Equal(t, 60, settings.HeavyRPM)
	require.True(t, settings.ExemptAdmin)
	require.Equal(t, 300, settings.PublicIPRPM)
}

func TestSetPanelRateLimitSettingsValidation(t *testing.T) {
	svc := newPanelRateLimitTestService(&panelRateLimitSettingRepo{})

	require.Error(t, svc.SetPanelRateLimitSettings(context.Background(), nil))
	require.Error(t, svc.SetPanelRateLimitSettings(context.Background(), &PanelRateLimitSettings{UserRPM: -1}))
	require.Error(t, svc.SetPanelRateLimitSettings(context.Background(), &PanelRateLimitSettings{HeavyRPM: panelRateLimitRPMMax + 1}))
}

func TestSetPanelRateLimitSettingsRoundTripAndCacheRefresh(t *testing.T) {
	repo := &panelRateLimitSettingRepo{}
	svc := newPanelRateLimitTestService(repo)

	// 先填充缓存（默认值）
	cached := svc.GetPanelRateLimitSettingsCached(context.Background())
	require.Equal(t, *DefaultPanelRateLimitSettings(), cached)

	want := &PanelRateLimitSettings{
		Enabled:     true,
		UserRPM:     120,
		HeavyRPM:    30,
		ExemptAdmin: false,
		PublicIPRPM: 60,
	}
	require.NoError(t, svc.SetPanelRateLimitSettings(context.Background(), want))

	// 写入后无需等待 TTL，缓存立即反映新值
	cached = svc.GetPanelRateLimitSettingsCached(context.Background())
	require.Equal(t, *want, cached)

	// DB 中持久化的值可直读
	stored, err := svc.GetPanelRateLimitSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, want, stored)
}

func TestGetPanelRateLimitSettingsCachedAvoidsRepeatedDBReads(t *testing.T) {
	repo := &panelRateLimitSettingRepo{values: map[string]string{
		SettingKeyPanelRateLimitSettings: `{"enabled":true,"user_rpm":100,"heavy_rpm":20,"exempt_admin":true,"public_ip_rpm":50}`,
	}}
	svc := newPanelRateLimitTestService(repo)

	first := svc.GetPanelRateLimitSettingsCached(context.Background())
	require.True(t, first.Enabled, "the first request serves the secure default")
	require.Eventually(t, func() bool {
		return svc.GetPanelRateLimitSettingsCached(context.Background()).UserRPM == 100
	}, time.Second, 10*time.Millisecond)
	for i := 0; i < 5; i++ {
		require.Equal(t, 100, svc.GetPanelRateLimitSettingsCached(context.Background()).UserRPM)
	}

	repo.mu.Lock()
	calls := repo.getValueCalls
	repo.mu.Unlock()
	require.Equal(t, 1, calls, "TTL 内应只读一次 DB")
}

func TestGetPanelRateLimitSettingsCachedServesStaleWhileRefreshing(t *testing.T) {
	repo := &panelRateLimitSettingRepo{values: map[string]string{
		SettingKeyPanelRateLimitSettings: `{"enabled":true,"user_rpm":200}`,
	}}
	svc := newPanelRateLimitTestService(repo)
	svc.storePanelRateLimitCache(PanelRateLimitSettings{Enabled: true, UserRPM: 100}, -time.Second)

	stale := svc.GetPanelRateLimitSettingsCached(context.Background())
	require.Equal(t, 100, stale.UserRPM)
	require.Eventually(t, func() bool {
		return svc.GetPanelRateLimitSettingsCached(context.Background()).UserRPM == 200
	}, time.Second, 10*time.Millisecond)
}

func TestGetPanelRateLimitSettingsCachedBacksOffAfterFailure(t *testing.T) {
	repo := &panelRateLimitSettingRepo{getValueErr: errors.New("database unavailable")}
	svc := newPanelRateLimitTestService(repo)
	svc.storePanelRateLimitCache(PanelRateLimitSettings{Enabled: true, UserRPM: 100}, -time.Second)

	require.Equal(t, 100, svc.GetPanelRateLimitSettingsCached(context.Background()).UserRPM)
	require.Eventually(t, func() bool {
		return !svc.panelRateLimitRefreshInFlight.Load()
	}, time.Second, 10*time.Millisecond)
	for i := 0; i < 10; i++ {
		require.Equal(t, 100, svc.GetPanelRateLimitSettingsCached(context.Background()).UserRPM)
	}

	repo.mu.Lock()
	calls := repo.getValueCalls
	repo.mu.Unlock()
	require.Equal(t, 1, calls, "failure backoff must suppress per-request DB retries")
}

package service

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type requestPrioritySettingRepo struct {
	mu     sync.RWMutex
	values map[string]string
}

func newRequestPrioritySettingRepo() *requestPrioritySettingRepo {
	return &requestPrioritySettingRepo{values: make(map[string]string)}
}

func (r *requestPrioritySettingRepo) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}

func (r *requestPrioritySettingRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *requestPrioritySettingRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	r.values[key] = value
	r.mu.Unlock()
	return nil
}

func (r *requestPrioritySettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			result[key] = value
		}
	}
	return result, nil
}

func (r *requestPrioritySettingRepo) SetMultiple(_ context.Context, values map[string]string) error {
	r.mu.Lock()
	for key, value := range values {
		r.values[key] = value
	}
	r.mu.Unlock()
	return nil
}

func (r *requestPrioritySettingRepo) GetAll(context.Context) (map[string]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make(map[string]string, len(r.values))
	for key, value := range r.values {
		result[key] = value
	}
	return result, nil
}

func (r *requestPrioritySettingRepo) Delete(_ context.Context, key string) error {
	r.mu.Lock()
	delete(r.values, key)
	r.mu.Unlock()
	return nil
}

func setRequestPriorityRepoValues(r *requestPrioritySettingRepo, enabled bool, limit, mib int) {
	values := map[string]string{
		SettingKeyRequestPriorityAdmissionEnabled:        "false",
		SettingKeyRequestPriorityPendingLimitPerInstance: "256",
		SettingKeyRequestPriorityPendingMiBPerInstance:   "256",
	}
	if enabled {
		values[SettingKeyRequestPriorityAdmissionEnabled] = "true"
	}
	if limit > 0 {
		values[SettingKeyRequestPriorityPendingLimitPerInstance] = strconv.Itoa(limit)
	}
	if mib > 0 {
		values[SettingKeyRequestPriorityPendingMiBPerInstance] = strconv.Itoa(mib)
	}
	_ = r.SetMultiple(context.Background(), values)
}

func waitForRequestPrioritySettings(t *testing.T, condition func() bool) {
	t.Helper()
	require.Eventually(t, condition, time.Second, time.Millisecond)
}

func TestRequestPriorityAdmissionSettingsDefaultsAndPersistence(t *testing.T) {
	repo := newRequestPrioritySettingRepo()
	svc := NewSettingService(repo, &config.Config{})

	require.Equal(t, RequestPriorityAdmissionSettings{
		PendingLimitPerInstance: 256,
		PendingMiBPerInstance:   256,
	}, svc.RequestPriorityAdmissionSettingsSnapshot())

	var snapshots []RequestPriorityAdmissionSettings
	svc.SetRequestPriorityAdmissionSettingsSink(func(settings RequestPriorityAdmissionSettings) {
		snapshots = append(snapshots, settings)
	})
	require.Len(t, snapshots, 1, "sink must receive the current snapshot immediately")

	require.NoError(t, svc.UpdateSettings(context.Background(), &SystemSettings{
		RequestPriorityAdmissionEnabled:        true,
		RequestPriorityPendingLimitPerInstance: 512,
		RequestPriorityPendingMiBPerInstance:   384,
	}))
	require.Equal(t, "true", mustRequestPriorityRepoValue(t, repo, SettingKeyRequestPriorityAdmissionEnabled))
	require.Equal(t, "512", mustRequestPriorityRepoValue(t, repo, SettingKeyRequestPriorityPendingLimitPerInstance))
	require.Equal(t, "384", mustRequestPriorityRepoValue(t, repo, SettingKeyRequestPriorityPendingMiBPerInstance))
	require.Equal(t, int64(384*1024*1024), svc.RequestPriorityAdmissionSettingsSnapshot().PendingBytesPerInstance())
	require.Equal(t, RequestPriorityAdmissionSettings{
		Enabled:                 true,
		PendingLimitPerInstance: 512,
		PendingMiBPerInstance:   384,
	}, snapshots[len(snapshots)-1])
}

func mustRequestPriorityRepoValue(t *testing.T, repo *requestPrioritySettingRepo, key string) string {
	t.Helper()
	value, err := repo.GetValue(context.Background(), key)
	require.NoError(t, err)
	return value
}

func TestRequestPriorityAdmissionSettingsPubSubAndPeriodicReconcile(t *testing.T) {
	repo := newRequestPrioritySettingRepo()
	setRequestPriorityRepoValues(repo, false, 256, 256)

	redisServer := miniredis.RunT(t)
	redisClient := redis.NewClient(&redis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { _ = redisClient.Close() })

	first := NewSettingService(repo, &config.Config{})
	second := NewSettingService(repo, &config.Config{})
	require.NoError(t, first.LoadRequestPriorityAdmissionSettings(context.Background()))
	require.NoError(t, second.LoadRequestPriorityAdmissionSettings(context.Background()))
	first.startRequestPriorityAdmissionSettingsSync(context.Background(), redisClient, time.Hour)
	second.startRequestPriorityAdmissionSettingsSync(context.Background(), redisClient, time.Hour)
	t.Cleanup(first.StopRequestPriorityAdmissionSettingsSync)
	t.Cleanup(second.StopRequestPriorityAdmissionSettingsSync)

	waitForRequestPrioritySettings(t, func() bool {
		return redisClient.PubSubNumSub(context.Background(), requestPriorityAdmissionSettingsChannel).Val()[requestPriorityAdmissionSettingsChannel] == 2
	})
	setRequestPriorityRepoValues(repo, true, 400, 320)
	first.publishRequestPriorityAdmissionSettingsUpdate(context.Background())
	waitForRequestPrioritySettings(t, func() bool {
		return second.RequestPriorityAdmissionSettingsSnapshot() == (RequestPriorityAdmissionSettings{
			Enabled: true, PendingLimitPerInstance: 400, PendingMiBPerInstance: 320,
		})
	})

	periodic := NewSettingService(repo, &config.Config{})
	require.NoError(t, periodic.LoadRequestPriorityAdmissionSettings(context.Background()))
	periodic.startRequestPriorityAdmissionSettingsSync(context.Background(), nil, 5*time.Millisecond)
	t.Cleanup(periodic.StopRequestPriorityAdmissionSettingsSync)
	setRequestPriorityRepoValues(repo, false, 600, 128)
	waitForRequestPrioritySettings(t, func() bool {
		return periodic.RequestPriorityAdmissionSettingsSnapshot() == (RequestPriorityAdmissionSettings{
			PendingLimitPerInstance: 600, PendingMiBPerInstance: 128,
		})
	})
}

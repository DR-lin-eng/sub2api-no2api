//go:build unit

package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type lastUsedSchedulerStub struct {
	mu        sync.Mutex
	scheduled map[int64]time.Time
	canceled  []int64
	accept    bool
}

func (s *lastUsedSchedulerStub) ScheduleAPIKeyLastUsedUpdate(apiKeyID int64, usedAt time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.accept {
		return false
	}
	if s.scheduled == nil {
		s.scheduled = make(map[int64]time.Time)
	}
	s.scheduled[apiKeyID] = usedAt
	return true
}

func (s *lastUsedSchedulerStub) CancelAPIKeyLastUsedUpdate(apiKeyID int64) {
	s.mu.Lock()
	s.canceled = append(s.canceled, apiKeyID)
	delete(s.scheduled, apiKeyID)
	s.mu.Unlock()
}

func TestAPIKeyLastUsedDebounceCacheBoundedAbove20KKeyScale(t *testing.T) {
	var cache apiKeyLastUsedDebounceCache
	expiresAt := time.Now().Add(time.Hour)
	for id := int64(1); id <= 40000; id++ {
		cache.Store(id, expiresAt)
	}
	require.Positive(t, cache.Len())
	require.LessOrEqual(t, cache.Len(), apiKeyLastUsedCacheCapacity)
}

func TestAPIKeyServiceTouchLastUsedUsesDeferredScheduler(t *testing.T) {
	scheduler := &lastUsedSchedulerStub{accept: true}
	svc := &APIKeyService{lastUsedScheduler: scheduler}

	require.NoError(t, svc.TouchLastUsed(context.Background(), 42))
	require.NoError(t, svc.TouchLastUsed(context.Background(), 42))

	scheduler.mu.Lock()
	defer scheduler.mu.Unlock()
	require.Len(t, scheduler.scheduled, 1)
	require.Contains(t, scheduler.scheduled, int64(42))
}

type deferredBatchAPIKeyRepo struct {
	*apiKeyRepoStub
	mu      sync.Mutex
	batches []map[int64]time.Time
	err     error
}

func (r *deferredBatchAPIKeyRepo) BatchUpdateLastUsed(_ context.Context, updates map[int64]time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copyOfUpdates := make(map[int64]time.Time, len(updates))
	for id, ts := range updates {
		copyOfUpdates[id] = ts
	}
	r.batches = append(r.batches, copyOfUpdates)
	return r.err
}

func TestDeferredServiceFlushesCoalescedAPIKeyLastUsedBatch(t *testing.T) {
	repo := &deferredBatchAPIKeyRepo{apiKeyRepoStub: &apiKeyRepoStub{}}
	svc := &DeferredService{apiKeyRepo: repo}
	first := time.Now().Add(-time.Second)
	latest := time.Now()

	require.True(t, svc.ScheduleAPIKeyLastUsedUpdate(1, first))
	require.True(t, svc.ScheduleAPIKeyLastUsedUpdate(1, latest))
	require.True(t, svc.ScheduleAPIKeyLastUsedUpdate(2, latest))
	svc.flushAPIKeyLastUsed()

	repo.mu.Lock()
	defer repo.mu.Unlock()
	require.Len(t, repo.batches, 1)
	require.Len(t, repo.batches[0], 2)
	require.Equal(t, latest, repo.batches[0][1])
	require.Zero(t, svc.apiKeyLastUsedPending.Load())
}

func TestDeferredServiceRequeuesFailedAPIKeyLastUsedBatch(t *testing.T) {
	repo := &deferredBatchAPIKeyRepo{
		apiKeyRepoStub: &apiKeyRepoStub{},
		err:            errors.New("db unavailable"),
	}
	svc := &DeferredService{apiKeyRepo: repo}
	require.True(t, svc.ScheduleAPIKeyLastUsedUpdate(1, time.Now()))

	svc.flushAPIKeyLastUsed()

	require.Equal(t, int64(1), svc.apiKeyLastUsedPending.Load())
	_, exists := svc.apiKeyLastUsedUpdates.Load(int64(1))
	require.True(t, exists)
}

type benchmarkLastUsedScheduler struct{}

func (benchmarkLastUsedScheduler) ScheduleAPIKeyLastUsedUpdate(int64, time.Time) bool { return true }
func (benchmarkLastUsedScheduler) CancelAPIKeyLastUsedUpdate(int64)                   {}

func BenchmarkAPIKeyServiceTouchLastUsed(b *testing.B) {
	b.Run("parallel_debounce_hit", func(b *testing.B) {
		svc := &APIKeyService{lastUsedScheduler: benchmarkLastUsedScheduler{}}
		svc.lastUsedTouchL1.Store(42, time.Now().Add(time.Hour))
		ctx := context.Background()
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = svc.TouchLastUsed(ctx, 42)
			}
		})
	})

	b.Run("distinct_keys", func(b *testing.B) {
		svc := &APIKeyService{lastUsedScheduler: benchmarkLastUsedScheduler{}}
		ctx := context.Background()
		var id atomic.Int64
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				_ = svc.TouchLastUsed(ctx, id.Add(1))
			}
		})
	})
}

var benchmarkAPIKeyAuthSink *APIKey

func BenchmarkAPIKeyAuthCacheHitMaterialization(b *testing.B) {
	svc := &APIKeyService{}
	entry := &APIKeyAuthCacheEntry{Snapshot: &APIKeyAuthSnapshot{
		Version:  apiKeyAuthSnapshotVersion,
		APIKeyID: 42,
		UserID:   7,
		Status:   StatusActive,
		User: APIKeyAuthUserSnapshot{
			ID:          7,
			Status:      StatusActive,
			Concurrency: 200000,
		},
		Group: &APIKeyAuthGroupSnapshot{
			ID:       9,
			Status:   StatusActive,
			Platform: PlatformOpenAI,
		},
	}}
	b.ReportAllocs()
	for b.Loop() {
		apiKey, used, err := svc.applyAuthCacheEntry("sk-single-hot-key", entry)
		if err != nil || !used {
			b.Fatalf("applyAuthCacheEntry() = (%v, %v, %v)", apiKey, used, err)
		}
		benchmarkAPIKeyAuthSink = apiKey
	}
}

func BenchmarkAPIKeyAuthSingleHotKeyLookup(b *testing.B) {
	svc := &APIKeyService{authCfg: apiKeyAuthCacheConfig{l1Size: 1, l1TTL: time.Hour}}
	entry := &APIKeyAuthCacheEntry{Snapshot: &APIKeyAuthSnapshot{
		Version:  apiKeyAuthSnapshotVersion,
		APIKeyID: 42,
		UserID:   7,
		Status:   StatusActive,
		User:     APIKeyAuthUserSnapshot{ID: 7, Status: StatusActive, Concurrency: 200000},
		Group:    &APIKeyAuthGroupSnapshot{ID: 9, Status: StatusActive, Platform: PlatformOpenAI},
	}}
	digest := authCacheDigest("sk-single-hot-key")
	svc.setAuthHotCacheEntry(digest, "sk-single-hot-key", entry)
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			apiKey, err := svc.GetByKey(context.Background(), "sk-single-hot-key")
			if err != nil {
				b.Fatal(err)
			}
			benchmarkAPIKeyAuthSink = apiKey
		}
	})
}

func BenchmarkStandaloneSingleHotAPIKeyAdmission(b *testing.B) {
	auth := &APIKeyService{
		authCfg:           apiKeyAuthCacheConfig{l1Size: 1, l1TTL: time.Hour},
		lastUsedScheduler: benchmarkLastUsedScheduler{},
	}
	entry := &APIKeyAuthCacheEntry{Snapshot: &APIKeyAuthSnapshot{
		Version:          apiKeyAuthSnapshotVersion,
		APIKeyID:         42,
		UserID:           7,
		Status:           StatusActive,
		ConcurrencyLimit: 200000,
		User:             APIKeyAuthUserSnapshot{ID: 7, Status: StatusActive, Concurrency: 200000},
		Group:            &APIKeyAuthGroupSnapshot{ID: 9, Status: StatusActive, Platform: PlatformOpenAI},
	}}
	digest := authCacheDigest("sk-single-hot-key")
	auth.setAuthHotCacheEntry(digest, "sk-single-hot-key", entry)
	auth.lastUsedTouchL1.Store(42, time.Now().Add(time.Hour))
	concurrency := NewConcurrencyService(nil)
	concurrency.SetStandaloneRequestSlots(true)
	ctx := context.Background()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			apiKey, err := auth.GetByKey(ctx, "sk-single-hot-key")
			if err != nil {
				b.Fatal(err)
			}
			_ = auth.TouchLastUsed(ctx, apiKey.ID)
			userResult, err := concurrency.AcquireUserSlot(ctx, apiKey.UserID, apiKey.User.Concurrency)
			if err != nil || !userResult.Acquired {
				b.Fatalf("AcquireUserSlot() = (%v, %v)", userResult, err)
			}
			result, err := concurrency.AcquireAPIKeySlot(ctx, apiKey.ID, apiKey.ConcurrencyLimit)
			if err != nil || !result.Acquired {
				b.Fatalf("AcquireAPIKeySlot() = (%v, %v)", result, err)
			}
			result.ReleaseFunc()
			userResult.ReleaseFunc()
		}
	})
}

func BenchmarkStandaloneSingleUserBurstRejection(b *testing.B) {
	svc := NewConcurrencyService(nil)
	svc.SetStandaloneRequestSlots(true)
	held, err := svc.AcquireUserSlot(context.Background(), 7, 1)
	if err != nil || !held.Acquired {
		b.Fatal(err)
	}
	defer held.ReleaseFunc()
	for i := 0; i < 20; i++ {
		allowed, err := svc.IncrementWaitCount(context.Background(), 7, 20)
		if err != nil || !allowed {
			b.Fatal("failed to prefill local wait queue")
		}
	}
	ctx := context.Background()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			result, err := svc.AcquireUserSlot(ctx, 7, 1)
			if err != nil || result.Acquired {
				b.Fatalf("expected local user slot rejection: result=%v err=%v", result, err)
			}
			allowed, err := svc.IncrementWaitCount(ctx, 7, 20)
			if err != nil || allowed {
				b.Fatalf("expected local wait queue rejection: allowed=%v err=%v", allowed, err)
			}
		}
	})
}

func TestAPIKeyAuthCacheRuntimePrototypeClonesOnlyRequestState(t *testing.T) {
	svc := &APIKeyService{}
	entry := &APIKeyAuthCacheEntry{Snapshot: &APIKeyAuthSnapshot{
		Version:  apiKeyAuthSnapshotVersion,
		APIKeyID: 42,
		UserID:   7,
		Status:   StatusActive,
		User:     APIKeyAuthUserSnapshot{ID: 7, Status: StatusActive},
		Group:    &APIKeyAuthGroupSnapshot{ID: 9, Status: StatusActive, Platform: PlatformOpenAI},
	}}

	first, used, err := svc.applyAuthCacheEntry("sk-first", entry)
	require.NoError(t, err)
	require.True(t, used)
	second, used, err := svc.applyAuthCacheEntry("sk-second", entry)
	require.NoError(t, err)
	require.True(t, used)
	require.NotSame(t, first, second)
	require.Same(t, first.User, second.User)
	require.Same(t, first.Group, second.Group)
	require.Equal(t, "sk-first", first.Key)
	require.Equal(t, "sk-second", second.Key)
	first.Status = StatusDisabled
	require.Equal(t, StatusActive, second.Status)
}

func TestAPIKeyAuthHotCacheInvalidation(t *testing.T) {
	svc := &APIKeyService{authCfg: apiKeyAuthCacheConfig{l1Size: 1, l1TTL: time.Hour}}
	digest := authCacheDigest("sk-hot")
	entry := &APIKeyAuthCacheEntry{Snapshot: &APIKeyAuthSnapshot{Version: apiKeyAuthSnapshotVersion}}
	svc.setAuthHotCacheEntry(digest, "sk-hot", entry)
	_, ok := svc.getAuthHotCacheEntry(digest, time.Now())
	require.True(t, ok)
	first, err := svc.GetByKey(context.Background(), "sk-hot")
	require.NoError(t, err)
	second, err := svc.GetByKey(context.Background(), "sk-hot")
	require.NoError(t, err)
	require.Same(t, first, second)

	svc.clearAuthHotCacheEntry(authCacheKeyFromDigest(digest))
	_, ok = svc.getAuthHotCacheEntry(digest, time.Now())
	require.False(t, ok)
}

func TestAPIKeyAuthHotCacheConcurrentSingleKey(t *testing.T) {
	svc := &APIKeyService{authCfg: apiKeyAuthCacheConfig{l1Size: 1, l1TTL: time.Hour}}
	digest := authCacheDigest("sk-hot")
	entry := &APIKeyAuthCacheEntry{Snapshot: &APIKeyAuthSnapshot{
		Version: apiKeyAuthSnapshotVersion,
		Status:  StatusActive,
		User:    APIKeyAuthUserSnapshot{Status: StatusActive},
	}}
	svc.setAuthHotCacheEntry(digest, "sk-hot", entry)
	expected, err := svc.GetByKey(context.Background(), "sk-hot")
	require.NoError(t, err)

	const workers = 64
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				apiKey, err := svc.GetByKey(context.Background(), "sk-hot")
				if err != nil || apiKey != expected {
					errs <- errors.New("hot cache returned an invalid API key")
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

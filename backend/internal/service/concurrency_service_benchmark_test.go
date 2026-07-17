package service

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type benchmarkAccountLoadCache struct {
	ConcurrencyCache
	loadMap map[int64]*AccountLoadInfo
}

type benchmarkAPIKeyConcurrencyCache struct {
	ConcurrencyCache
}

func (benchmarkAPIKeyConcurrencyCache) AcquireAPIKeySlot(context.Context, int64, int, string) (bool, error) {
	return true, nil
}

func (benchmarkAPIKeyConcurrencyCache) TrackAPIKeySlot(context.Context, int64, string) error {
	return nil
}

func (benchmarkAPIKeyConcurrencyCache) ReleaseAPIKeySlot(context.Context, int64, string) error {
	return nil
}

func (benchmarkAPIKeyConcurrencyCache) GetAPIKeyConcurrencyBatch(context.Context, []int64) (map[int64]int, error) {
	return nil, nil
}

func (c benchmarkAccountLoadCache) GetAccountsLoadBatch(_ context.Context, _ []AccountWithConcurrency) (map[int64]*AccountLoadInfo, error) {
	return c.loadMap, nil
}

func BenchmarkConcurrencyServiceAccountLoadCache(b *testing.B) {
	svc := NewConcurrencyService(nil)
	now := time.Now()
	loadMap := map[int64]*AccountLoadInfo{
		1: {AccountID: 1, CurrentConcurrency: 3, WaitingCount: 2, LoadRate: 50},
	}

	b.Run("parallel_hit", func(b *testing.B) {
		key := accountLoadBatchKey{count: 1, hashA: 1, hashB: 1}
		svc.storeCachedAccountLoadBatch(key, loadMap, now.Add(time.Hour))
		var invalid atomic.Bool
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			var result map[int64]*AccountLoadInfo
			ran := false
			for pb.Next() {
				ran = true
				result, _ = svc.getCachedAccountLoadBatch(key, now)
			}
			if ran && result == nil {
				invalid.Store(true)
			}
		})
		if invalid.Load() {
			b.Fatal("cache hit returned nil")
		}
	})

	b.Run("parallel_mixed_keys", func(b *testing.B) {
		const keyCount = 64
		var keys [keyCount]accountLoadBatchKey
		for i := 0; i < keyCount; i++ {
			keys[i] = accountLoadBatchKey{count: 1, hashA: uint64(i + 1), hashB: uint64(i + 1)}
			svc.storeCachedAccountLoadBatch(keys[i], loadMap, now.Add(time.Hour))
		}
		var sequence atomic.Uint64
		var invalid atomic.Bool
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			var result map[int64]*AccountLoadInfo
			ran := false
			for pb.Next() {
				ran = true
				key := keys[sequence.Add(1)%keyCount]
				result, _ = svc.getCachedAccountLoadBatch(key, now)
			}
			if ran && result == nil {
				invalid.Store(true)
			}
		})
		if invalid.Load() {
			b.Fatal("cache hit returned nil")
		}
	})

	b.Run("parallel_service_hit_64_accounts", func(b *testing.B) {
		accounts := make([]AccountWithConcurrency, 64)
		serviceLoadMap := make(map[int64]*AccountLoadInfo, len(accounts))
		for i := range accounts {
			accountID := int64(i + 1)
			accounts[i] = AccountWithConcurrency{ID: accountID, MaxConcurrency: 32}
			serviceLoadMap[accountID] = &AccountLoadInfo{AccountID: accountID, LoadRate: i % 100}
		}
		serviceCache := NewConcurrencyService(benchmarkAccountLoadCache{loadMap: serviceLoadMap})
		serviceCache.SetAccountLoadBatchCacheTTL(time.Hour)
		if _, err := serviceCache.GetAccountsLoadBatch(context.Background(), accounts); err != nil {
			b.Fatal(err)
		}

		var invalid atomic.Bool
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			var result map[int64]*AccountLoadInfo
			ran := false
			for pb.Next() {
				ran = true
				result, _ = serviceCache.GetAccountsLoadBatch(context.Background(), accounts)
			}
			if ran && len(result) != len(accounts) {
				invalid.Store(true)
			}
		})
		if invalid.Load() {
			b.Fatal("cached service result has the wrong size")
		}
	})
}

func BenchmarkConcurrencyServiceAPIKeySlot(b *testing.B) {
	svc := NewConcurrencyService(benchmarkAPIKeyConcurrencyCache{})
	ctx := context.Background()

	for _, tc := range []struct {
		name  string
		limit int
	}{
		{name: "unlimited_tracking", limit: 0},
		{name: "limited_atomic_acquire", limit: 8},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				result, err := svc.AcquireAPIKeySlot(ctx, 42, tc.limit)
				if err != nil || !result.Acquired {
					b.Fatalf("AcquireAPIKeySlot() = (%v, %v)", result, err)
				}
				result.ReleaseFunc()
			}
		})
	}
}

//go:build unit

package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func BenchmarkSchedulerCacheAccountSchedulingState(b *testing.B) {
	ctx := context.Background()
	mr := miniredis.RunT(b)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	b.Cleanup(func() { _ = rdb.Close() })
	cache, ok := newSchedulerCacheWithChunkSizes(rdb, defaultSchedulerSnapshotMGetChunkSize, defaultSchedulerSnapshotWriteChunkSize).(*schedulerCache)
	if !ok {
		b.Fatal("unexpected scheduler cache implementation")
	}
	account := service.Account{
		ID:          9401,
		Name:        "wait-revalidation-benchmark",
		Platform:    service.PlatformOpenAI,
		Type:        service.AccountTypeAPIKey,
		Status:      service.StatusActive,
		Schedulable: true,
		Credentials: map[string]any{
			"api_key":       strings.Repeat("k", 4096),
			"access_token":  strings.Repeat("a", 8192),
			"refresh_token": strings.Repeat("r", 8192),
		},
		Extra: map[string]any{
			"quota_limit":        100.0,
			"quota_used":         10.0,
			"unused_large_field": strings.Repeat("x", 8192),
		},
	}
	if err := cache.SetAccount(ctx, &account); err != nil {
		b.Fatal(err)
	}

	b.Run("scheduling_state", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			state, found, err := cache.GetCachedAccountSchedulingState(ctx, account.ID)
			if err != nil || !found || !state.Schedulable {
				b.Fatalf("unexpected state: state=%+v found=%t err=%v", state, found, err)
			}
		}
	})

	b.Run("full_account", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			cached, err := cache.GetAccount(ctx, account.ID)
			if err != nil || cached == nil || !cached.IsSchedulable() {
				b.Fatalf("unexpected account: account=%+v err=%v", cached, err)
			}
		}
	})
}

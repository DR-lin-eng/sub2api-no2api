package repository

import (
	"context"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestOpenAIIntegrityRedisCASAcrossReplicas(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := &gatewayCache{rdb: client}
	ctx := context.Background()
	ok, err := cache.CompareAndSwapOpenAIIntegrityState(ctx, "fixture-key", "", "first", time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	var wg sync.WaitGroup
	var accepted atomic.Int64
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			otherReplica := &gatewayCache{rdb: client}
			ok, err := otherReplica.CompareAndSwapOpenAIIntegrityState(ctx, "fixture-key", "first", "next", time.Minute)
			if err == nil && ok {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, accepted.Load())
	value, found, err := cache.GetOpenAIIntegrityState(ctx, "fixture-key")
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, "next", value)
	server.FastForward(2 * time.Minute)
	_, found, err = cache.GetOpenAIIntegrityState(ctx, "fixture-key")
	require.NoError(t, err)
	require.False(t, found)
}

func TestOpenAIIntegrityRealRedis(t *testing.T) {
	addr := os.Getenv("SUB2API_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("explicit isolated Redis required")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, client.Ping(ctx).Err())
	key := "isolated-integrity-test"
	t.Cleanup(func() { _ = client.Del(context.Background(), buildOpenAIWSSharedStateKey(key)).Err() })
	first := &gatewayCache{rdb: client}
	ok, err := first.CompareAndSwapOpenAIIntegrityState(ctx, key, "", "ois1.a.b.c", time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
	var accepted atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			replica := &gatewayCache{rdb: client}
			ok, err := replica.CompareAndSwapOpenAIIntegrityState(ctx, key, "ois1.a.b.c", "ois1.new.server.state", time.Minute)
			if err == nil && ok {
				accepted.Add(1)
			}
		}()
	}
	wg.Wait()
	require.EqualValues(t, 1, accepted.Load())
	t.Log("REAL_REDIS_CAS_CONTENDERS=32 ACCEPTED=1")
}

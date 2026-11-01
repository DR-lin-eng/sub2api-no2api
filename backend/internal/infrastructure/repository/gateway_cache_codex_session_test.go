package repository

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestCodexSessionIDCacheConcurrentReplicasAndTTL(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	key, first := requireCodexSessionIDCacheConcurrentReplicas(t, client)
	cache := &gatewayCache{rdb: client}
	server.FastForward(30 * time.Second)
	value, err := cache.GetOrCreateCodexSessionID(context.Background(), key, uuid.Must(uuid.NewV7()).String(), time.Minute)
	require.NoError(t, err)
	require.Equal(t, first, value)
	require.Equal(t, time.Minute, server.TTL(buildOpenAIWSSharedStateKey(key)), "a live conversation refreshes its retention")
	server.FastForward(time.Minute + time.Second)
	next := uuid.Must(uuid.NewV7()).String()
	value, err = cache.GetOrCreateCodexSessionID(context.Background(), key, next, time.Minute)
	require.NoError(t, err)
	require.Equal(t, next, value)
}

func TestCodexSessionIDCacheRealRedis(t *testing.T) {
	addr := os.Getenv("SUB2API_TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("explicit isolated Redis required")
	}
	client := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	requireCodexSessionIDCacheConcurrentReplicas(t, client)
}

func requireCodexSessionIDCacheConcurrentReplicas(t *testing.T, client *redis.Client) (string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, client.Ping(ctx).Err())
	key := "codex:simulation:v1:session:test:" + uuid.NewString()
	t.Cleanup(func() { _ = client.Del(context.Background(), buildOpenAIWSSharedStateKey(key)).Err() })
	const workers = 32
	type outcome struct {
		id  string
		err error
	}
	results := make(chan outcome, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			replica := &gatewayCache{rdb: client}
			id, err := replica.GetOrCreateCodexSessionID(ctx, key, uuid.Must(uuid.NewV7()).String(), time.Minute)
			results <- outcome{id: id, err: err}
		})
	}
	wg.Wait()
	close(results)
	var first string
	for result := range results {
		require.NoError(t, result.err)
		require.Equal(t, uuid.Version(7), uuid.MustParse(result.id).Version())
		if first == "" {
			first = result.id
		}
		require.Equal(t, first, result.id, "replicas must use one winner when creating the session concurrently")
	}
	t.Log("CODEX_SESSION_CONCURRENT_REPLICAS=32 DISTINCT_SESSION_IDS=1")
	return key, first
}

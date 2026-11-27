package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestGatewayCache_CompareAndSetSessionAccountID(t *testing.T) {
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	cache := &gatewayCache{rdb: client}
	ctx := context.Background()

	// 1. Unbound session -> binds atomically.
	ok, err := cache.CompareAndSetSessionAccountID(ctx, 7, "sess-1", 101, nil, time.Hour)
	require.NoError(t, err)
	require.True(t, ok)
	got, err := cache.GetSessionAccountID(ctx, 7, "sess-1")
	require.NoError(t, err)
	require.Equal(t, int64(101), got)

	// 2. Same account -> refreshes TTL.
	server.FastForward(30 * time.Minute)
	ok, err = cache.CompareAndSetSessionAccountID(ctx, 7, "sess-1", 101, nil, time.Hour)
	require.NoError(t, err)
	require.True(t, ok)
	require.Greater(t, server.TTL(buildSessionKey(7, "sess-1")), 55*time.Minute)

	// 3. Failed-over account still bound -> atomically rebinds to new account.
	ok, err = cache.CompareAndSetSessionAccountID(ctx, 7, "sess-1", 202, []int64{101}, time.Hour)
	require.NoError(t, err)
	require.True(t, ok)
	got, err = cache.GetSessionAccountID(ctx, 7, "sess-1")
	require.NoError(t, err)
	require.Equal(t, int64(202), got)

	// 4. Concurrent request rebound session to healthy account 303 -> stale caller with
	// target=101 or failed=[101] must NOT overwrite 303.
	require.NoError(t, cache.SetSessionAccountID(ctx, 7, "sess-1", 303, time.Hour))
	ok, err = cache.CompareAndSetSessionAccountID(ctx, 7, "sess-1", 101, nil, time.Hour)
	require.NoError(t, err)
	require.False(t, ok)
	ok, err = cache.CompareAndSetSessionAccountID(ctx, 7, "sess-1", 202, []int64{101}, time.Hour)
	require.NoError(t, err)
	require.False(t, ok)
	got, err = cache.GetSessionAccountID(ctx, 7, "sess-1")
	require.NoError(t, err)
	require.Equal(t, int64(303), got)
}

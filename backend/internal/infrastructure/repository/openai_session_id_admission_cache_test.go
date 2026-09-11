package repository

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestOpenAISessionIDAdmissionCacheEnforcesPerMinuteAndExistingSession(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	cache := &gatewayCache{rdb: rdb}

	first, err := cache.AdmitOpenAISessionID(context.Background(), 7, "s1", 1)
	require.NoError(t, err)
	require.True(t, first.Allowed)

	existing, err := cache.AdmitOpenAISessionID(context.Background(), 7, "s1", 1)
	require.NoError(t, err)
	require.True(t, existing.Allowed)
	require.True(t, existing.Existing)

	second, err := cache.AdmitOpenAISessionID(context.Background(), 7, "s2", 1)
	require.NoError(t, err)
	require.False(t, second.Allowed)
	require.GreaterOrEqual(t, second.RetryAfterSeconds, 1)
}

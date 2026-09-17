package repository

import (
	"context"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestOpenAIOAuthGatewayRateLimitCacheIsPerAccountAcrossClusterAndDeduplicatesRequests(t *testing.T) {
	firstBucket, firstSeen := openAIOAuthGatewayRateLimitKeys(101)
	secondBucket, secondSeen := openAIOAuthGatewayRateLimitKeys(202)
	require.Equal(t, "openai:oauth:{account_rate_limit:v2:101}:bucket", firstBucket)
	require.Equal(t, "openai:oauth:{account_rate_limit:v2:101}:seen", firstSeen)
	require.Equal(t, "openai:oauth:{account_rate_limit:v2:202}:bucket", secondBucket)
	require.Equal(t, "openai:oauth:{account_rate_limit:v2:202}:seen", secondSeen)
	require.NotEqual(t, firstBucket, secondBucket)

	mr := miniredis.RunT(t)
	firstRedis := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	secondRedis := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		_ = firstRedis.Close()
		_ = secondRedis.Close()
	})
	firstNode := &gatewayCache{rdb: firstRedis}
	secondNode := &gatewayCache{rdb: secondRedis}
	ctx := context.Background()

	first, err := firstNode.AdmitOpenAIOAuthGatewayRequest(ctx, 101, "request-1", 60, 2)
	require.NoError(t, err)
	require.True(t, first.Allowed)

	second, err := secondNode.AdmitOpenAIOAuthGatewayRequest(ctx, 101, "request-2", 60, 2)
	require.NoError(t, err)
	require.True(t, second.Allowed)

	limited, err := firstNode.AdmitOpenAIOAuthGatewayRequest(ctx, 101, "request-3", 60, 2)
	require.NoError(t, err)
	require.False(t, limited.Allowed)
	require.GreaterOrEqual(t, limited.RetryAfterSeconds, 1)

	retry, err := secondNode.AdmitOpenAIOAuthGatewayRequest(ctx, 101, "request-1", 60, 2)
	require.NoError(t, err)
	require.True(t, retry.Allowed)
	require.True(t, retry.Existing)

	independent, err := secondNode.AdmitOpenAIOAuthGatewayRequest(ctx, 202, "request-3", 60, 2)
	require.NoError(t, err)
	require.True(t, independent.Allowed)
	require.False(t, independent.Existing)
}

func TestOpenAIOAuthGatewayRateLimitCacheAgainstRedis(t *testing.T) {
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR is not set")
	}
	firstRedis := redis.NewClient(&redis.Options{Addr: addr})
	secondRedis := redis.NewClient(&redis.Options{Addr: addr})
	t.Cleanup(func() {
		_ = firstRedis.Close()
		_ = secondRedis.Close()
	})
	require.NoError(t, firstRedis.FlushDB(context.Background()).Err())
	firstNode := &gatewayCache{rdb: firstRedis}
	secondNode := &gatewayCache{rdb: secondRedis}

	decision, err := firstNode.AdmitOpenAIOAuthGatewayRequest(context.Background(), 101, "node-a", 60, 2)
	require.NoError(t, err)
	require.True(t, decision.Allowed)
	decision, err = secondNode.AdmitOpenAIOAuthGatewayRequest(context.Background(), 101, "node-b", 60, 2)
	require.NoError(t, err)
	require.True(t, decision.Allowed)

	decision, err = firstNode.AdmitOpenAIOAuthGatewayRequest(context.Background(), 101, "node-c", 60, 2)
	require.NoError(t, err)
	require.False(t, decision.Allowed)
	require.GreaterOrEqual(t, decision.RetryAfterSeconds, 1)

	decision, err = secondNode.AdmitOpenAIOAuthGatewayRequest(context.Background(), 202, "node-c", 60, 2)
	require.NoError(t, err)
	require.True(t, decision.Allowed)
}

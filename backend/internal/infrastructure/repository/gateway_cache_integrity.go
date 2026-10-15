package repository

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

// The request's expected value is compared inside Redis so two replicas cannot
// both advance a state that was issued for one concurrent request generation.
var openAIIntegrityCAS = redis.NewScript(`
local current = redis.call("GET", KEYS[1]) or ""
if current ~= ARGV[1] then return 0 end
redis.call("SET", KEYS[1], ARGV[2], "PX", ARGV[3])
return 1
`)

func (c *gatewayCache) GetOpenAIIntegrityState(ctx context.Context, key string) (string, bool, error) {
	value, err := c.rdb.Get(ctx, buildOpenAIWSSharedStateKey(key)).Result()
	if errors.Is(err, redis.Nil) {
		return "", false, nil
	}
	return value, err == nil, err
}

func (c *gatewayCache) CompareAndSwapOpenAIIntegrityState(ctx context.Context, key, expected, next string, ttl time.Duration) (bool, error) {
	if key == "" || next == "" || ttl <= 0 {
		return false, nil
	}
	value, err := openAIIntegrityCAS.Run(ctx, c.rdb, []string{buildOpenAIWSSharedStateKey(key)}, expected, next, ttl.Milliseconds()).Int()
	return err == nil && value == 1, err
}

var _ service.OpenAIIntegrityStateCache = (*gatewayCache)(nil)

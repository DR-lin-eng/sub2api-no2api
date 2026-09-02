package repository

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

const (
	openAIFailureCounterPrefix     = "openai_failure_count:account:"
	openAIFailureCounterTTLSeconds = 30 * 60
)

var openAIFailureCounterIncrScript = redis.NewScript(`
	local count = redis.call('INCR', KEYS[1])
	redis.call('EXPIRE', KEYS[1], ARGV[1])
	return count
`)

type openAIFailureCounterCache struct {
	rdb *redis.Client
}

func NewOpenAIFailureCounterCache(rdb *redis.Client) service.OpenAIFailureCounterCache {
	return &openAIFailureCounterCache{rdb: rdb}
}

func (c *openAIFailureCounterCache) IncrementOpenAIFailureCount(ctx context.Context, accountID int64) (int64, error) {
	key := fmt.Sprintf("%s%d", openAIFailureCounterPrefix, accountID)
	count, err := openAIFailureCounterIncrScript.Run(ctx, c.rdb, []string{key}, openAIFailureCounterTTLSeconds).Int64()
	if err != nil {
		return 0, fmt.Errorf("increment OpenAI failure count: %w", err)
	}
	return count, nil
}

func (c *openAIFailureCounterCache) ResetOpenAIFailureCount(ctx context.Context, accountID int64) error {
	key := fmt.Sprintf("%s%d", openAIFailureCounterPrefix, accountID)
	return c.rdb.Del(ctx, key).Err()
}

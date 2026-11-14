package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

var openAILocationSampleScript = redis.NewScript(`
local current = redis.call("GET", KEYS[1])
if current then
  return {current, redis.call("PTTL", KEYS[1])}
end
redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
return {ARGV[1], tonumber(ARGV[2])}
`)

// Reads do not extend the TTL: active accounts must refresh after ten minutes.
func (c *gatewayCache) GetOrCreateOpenAILocation(ctx context.Context, key, candidate string, ttl time.Duration) (string, time.Duration, error) {
	values, err := openAILocationSampleScript.Run(ctx, c.rdb, []string{"openai:location:" + key}, candidate, max(1, ttl.Milliseconds())).Slice()
	if err != nil {
		return "", 0, err
	}
	if len(values) != 2 {
		return "", 0, fmt.Errorf("invalid location cache result")
	}
	value, valueOK := values[0].(string)
	remaining, ttlOK := values[1].(int64)
	if !valueOK || !ttlOK || remaining <= 0 {
		return "", 0, fmt.Errorf("invalid location cache TTL")
	}
	return value, time.Duration(remaining) * time.Millisecond, nil
}

var _ service.OpenAILocationCache = (*gatewayCache)(nil)

package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

var codexSessionGetOrCreateScript = redis.NewScript(`
local current = redis.call("GET", KEYS[1])
if current and current ~= "" then
  redis.call("PEXPIRE", KEYS[1], ARGV[2])
  return current
end
redis.call("SET", KEYS[1], ARGV[1], "PX", ARGV[2])
return ARGV[1]
`)

func (c *gatewayCache) GetOrCreateCodexSessionID(ctx context.Context, key, candidate string, ttl time.Duration) (string, error) {
	key = buildOpenAIWSSharedStateKey(key)
	if key == "" || candidate == "" {
		return candidate, nil
	}
	ttlMillis := max(ttl.Milliseconds(), 1)
	return codexSessionGetOrCreateScript.Run(ctx, c.rdb, []string{key}, candidate, ttlMillis).Text()
}

var _ service.OpenAICodexSessionCache = (*gatewayCache)(nil)

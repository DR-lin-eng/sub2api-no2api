package repository

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

// Each account owns one cluster-wide bucket. Both keys use the same account
// hash tag so the Lua script remains Redis Cluster compatible. Redis TIME keeps
// refill calculations consistent across application nodes. A bounded seen set
// makes retries on the same account free while failover to another account
// consumes that account's independent bucket.
var openAIOAuthGatewayRateLimitScript = redis.NewScript(`
local bucket = KEYS[1]
local seen = KEYS[2]
local request_key = ARGV[1]
local rpm = tonumber(ARGV[2])
local burst = tonumber(ARGV[3])
local now_parts = redis.call("TIME")
local now_ms = tonumber(now_parts[1]) * 1000 + math.floor(tonumber(now_parts[2]) / 1000)

redis.call("ZREMRANGEBYSCORE", seen, "-inf", now_ms - 600000)
if redis.call("ZSCORE", seen, request_key) then
  redis.call("ZADD", seen, now_ms, request_key)
  redis.call("EXPIRE", seen, 660)
  return {1, 0, 1}
end

local state = redis.call("HMGET", bucket, "tokens", "updated_ms")
local tokens = tonumber(state[1])
local updated_ms = tonumber(state[2])
if not tokens or not updated_ms then
  tokens = burst
  updated_ms = now_ms
end
if tokens > burst then tokens = burst end
if now_ms > updated_ms then
  tokens = math.min(burst, tokens + ((now_ms - updated_ms) * rpm / 60000))
end

if tokens < 1 then
  local retry_ms = math.ceil((1 - tokens) * 60000 / rpm)
  redis.call("HSET", bucket, "tokens", tokens, "updated_ms", now_ms)
  return {0, math.max(1, math.ceil(retry_ms / 1000)), 0}
end

tokens = tokens - 1
redis.call("HSET", bucket, "tokens", tokens, "updated_ms", now_ms)
redis.call("ZADD", seen, now_ms, request_key)
local max_seen = math.min(100000, math.max(burst, rpm * 10))
local seen_count = redis.call("ZCARD", seen)
if seen_count > max_seen then
  redis.call("ZREMRANGEBYRANK", seen, 0, seen_count - max_seen - 1)
end
redis.call("EXPIRE", seen, 660)
return {1, 0, 0}
`)

func openAIOAuthGatewayRateLimitKeys(accountID int64) (string, string) {
	hashTag := fmt.Sprintf("account_rate_limit:v2:%d", accountID)
	prefix := fmt.Sprintf("openai:oauth:{%s}", hashTag)
	return prefix + ":bucket", prefix + ":seen"
}

func (c *gatewayCache) AdmitOpenAIOAuthGatewayRequest(ctx context.Context, accountID int64, requestKey string, rpm, burst int) (service.OpenAIOAuthGatewayRateLimitDecision, error) {
	if c == nil || c.rdb == nil {
		return service.OpenAIOAuthGatewayRateLimitDecision{}, fmt.Errorf("openai oauth gateway limiter cache is unavailable")
	}
	if accountID <= 0 || requestKey == "" || rpm <= 0 || burst <= 0 {
		return service.OpenAIOAuthGatewayRateLimitDecision{}, fmt.Errorf("invalid openai oauth gateway limiter request")
	}
	bucketKey, seenKey := openAIOAuthGatewayRateLimitKeys(accountID)
	values, err := openAIOAuthGatewayRateLimitScript.Run(
		ctx,
		c.rdb,
		[]string{bucketKey, seenKey},
		requestKey,
		rpm,
		burst,
	).Int64Slice()
	if err != nil {
		return service.OpenAIOAuthGatewayRateLimitDecision{}, err
	}
	if len(values) != 3 {
		return service.OpenAIOAuthGatewayRateLimitDecision{}, fmt.Errorf("invalid openai oauth gateway limiter response")
	}
	return service.OpenAIOAuthGatewayRateLimitDecision{
		Allowed:           values[0] == 1,
		RetryAfterSeconds: int(values[1]),
		Existing:          values[2] == 1,
	}, nil
}

var _ service.OpenAIOAuthGatewayRateLimitCache = (*gatewayCache)(nil)

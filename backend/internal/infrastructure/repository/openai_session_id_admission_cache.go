package repository

import (
	"context"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

// openAISessionIDAdmissionScript uses Redis TIME so all instances share the
// same one-minute window. The seen set makes repeated turns for one explicit
// session free; the rate set contains only newly observed sessions.
var openAISessionIDAdmissionScript = redis.NewScript(`
local seen = KEYS[1]
local rate = KEYS[2]
local session = ARGV[1]
local limit = tonumber(ARGV[2])
local now = tonumber(redis.call("TIME")[1])
local seenTTL = 3600
redis.call("ZREMRANGEBYSCORE", seen, "-inf", now - seenTTL)
if redis.call("ZSCORE", seen, session) then
  redis.call("ZADD", seen, now, session)
  redis.call("EXPIRE", seen, seenTTL + 60)
  return 2
end
if limit > 0 then
  redis.call("ZREMRANGEBYSCORE", rate, "-inf", now - 60)
  local count = redis.call("ZCARD", rate)
  if count >= limit then
    local oldest = redis.call("ZRANGE", rate, 0, 0, "WITHSCORES")
    local retry = 1
    if oldest[2] then retry = math.max(1, 61 - (now - tonumber(oldest[2]))) end
    redis.call("EXPIRE", rate, 120)
    return -retry
  end
end
redis.call("ZADD", seen, now, session)
redis.call("EXPIRE", seen, seenTTL + 60)
if limit > 0 then
  redis.call("ZADD", rate, now, session .. ":" .. now)
  redis.call("EXPIRE", rate, 120)
end
return 1
`)

func (c *gatewayCache) AdmitOpenAISessionID(ctx context.Context, accountID int64, sessionHash string, maxPerMinute int) (service.OpenAISessionIDAdmissionDecision, error) {
	if c == nil || c.rdb == nil || accountID <= 0 || sessionHash == "" {
		return service.OpenAISessionIDAdmissionDecision{Allowed: true}, nil
	}
	seenKey := fmt.Sprintf("openai:session_id:seen:%d", accountID)
	rateKey := fmt.Sprintf("openai:session_id:rate:%d", accountID)
	code, err := openAISessionIDAdmissionScript.Run(ctx, c.rdb, []string{seenKey, rateKey}, sessionHash, maxPerMinute).Int()
	if err != nil {
		return service.OpenAISessionIDAdmissionDecision{}, err
	}
	if code < 0 {
		return service.OpenAISessionIDAdmissionDecision{RetryAfterSeconds: -code}, nil
	}
	return service.OpenAISessionIDAdmissionDecision{Allowed: true, Existing: code == 2}, nil
}

var _ service.OpenAISessionIDAdmissionCache = (*gatewayCache)(nil)

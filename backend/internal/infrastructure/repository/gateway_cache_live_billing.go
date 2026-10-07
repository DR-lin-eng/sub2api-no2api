package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

const liveBillingPendingKey = "live:billing:pending"

var prepareLiveSettlementScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 0 then return -1 end
if redis.call("HGET", KEYS[1], "billing_state") == "billed" then return -1 end
if not redis.call("HGET", KEYS[1], "billing_snapshot") then return -2 end
if not redis.call("HGET", KEYS[1], "billing_duration_ms") then
  local created = tonumber(redis.call("HGET", KEYS[1], "created_at"))
  local expires = tonumber(redis.call("HGET", KEYS[1], "expires_at"))
  local ended = math.min(tonumber(ARGV[1]), expires)
  local duration = math.max(0, ended - created)
  redis.call("HSET", KEYS[1], "billing_duration_ms", duration)
end
redis.call("HSET", KEYS[1], "controller", "closed", "controller_owner", "", "billing_state", "pending")
redis.call("PERSIST", KEYS[1])
redis.call("ZADD", KEYS[2], ARGV[1], ARGV[2])
return 1
`)

var markLiveBilledScript = redis.NewScript(`
if redis.call("EXISTS", KEYS[1]) == 0 then return 0 end
redis.call("HSET", KEYS[1], "billing_state", "billed")
redis.call("ZREM", KEYS[2], ARGV[1])
redis.call("EXPIRE", KEYS[1], ARGV[2])
return 1
`)

func encodeLiveBillingSnapshot(values map[string]any, record *service.LiveCallRecord) error {
	if record.Billing == nil {
		return nil
	}
	encoded, err := json.Marshal(record.Billing)
	if err != nil {
		return fmt.Errorf("encode Live billing snapshot: %w", err)
	}
	values["billing_snapshot"] = string(encoded)
	values["billing_state"] = "active"
	return nil
}

func decodeLiveBillingSnapshot(values map[string]string, record *service.LiveCallRecord) error {
	if encoded := values["billing_snapshot"]; encoded != "" {
		if err := json.Unmarshal([]byte(encoded), &record.Billing); err != nil {
			return fmt.Errorf("decode Live billing snapshot: %w", err)
		}
	}
	if duration, ok := values["billing_duration_ms"]; ok {
		parsed, err := strconv.ParseInt(duration, 10, 64)
		if err != nil || parsed < 0 {
			return fmt.Errorf("invalid Live billing duration: %q", duration)
		}
		record.BillingDurationMs = parsed
	}
	return nil
}

func (c *gatewayCache) PrepareLiveCallSettlement(ctx context.Context, callHash string, endedAt time.Time) (*service.LiveCallRecord, error) {
	state, err := prepareLiveSettlementScript.Run(ctx, c.rdb,
		[]string{liveCallKey(callHash), liveBillingPendingKey}, endedAt.UnixMilli(), callHash).Int()
	if err != nil {
		return nil, err
	}
	if state == -1 {
		return nil, service.ErrLiveCallNotFound
	}
	if state != 1 {
		return nil, service.ErrLiveUnavailable
	}
	return c.GetLiveCall(ctx, callHash)
}

func (c *gatewayCache) ListDueLiveCalls(ctx context.Context, now time.Time, limit int) ([]*service.LiveCallRecord, error) {
	if limit <= 0 || limit > 32 {
		limit = 32
	}
	hashes, err := c.rdb.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key: liveBillingPendingKey, Start: "-inf", Stop: strconv.FormatInt(now.UnixMilli(), 10),
		ByScore: true, Offset: 0, Count: int64(limit),
	}).Result()
	if err != nil {
		return nil, err
	}
	records := make([]*service.LiveCallRecord, 0, len(hashes))
	for _, hash := range hashes {
		record, err := c.GetLiveCall(ctx, hash)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func (c *gatewayCache) MarkLiveCallBilled(ctx context.Context, callHash string, ttl time.Duration) error {
	state, err := markLiveBilledScript.Run(ctx, c.rdb,
		[]string{liveCallKey(callHash), liveBillingPendingKey}, callHash, int64(ttl.Seconds())).Int()
	if err != nil {
		return err
	}
	if state != 1 {
		return service.ErrLiveCallNotFound
	}
	return nil
}

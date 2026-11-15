package repository

import (
	"context"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

var acquireLiveLeaseScript = redis.NewScript(`
	redis.replicate_commands()
	local accountKey = KEYS[1]
	local userKey = KEYS[2]
	local apiKey = KEYS[3]
	local accountMax = tonumber(ARGV[1])
	local userMax = tonumber(ARGV[2])
	local apiMax = tonumber(ARGV[3])
	local slotTTL = tonumber(ARGV[4])
	local liveTTL = tonumber(ARGV[5])
	local leaseID = ARGV[6]
	local accountAllowance = tonumber(ARGV[7])
	local userAllowance = tonumber(ARGV[8])
	local apiAllowance = tonumber(ARGV[9])
	local now = tonumber(redis.call('TIME')[1])
	local expireBefore = now - slotTTL
	redis.call('ZREMRANGEBYSCORE', accountKey, '-inf', expireBefore)
	redis.call('ZREMRANGEBYSCORE', userKey, '-inf', expireBefore)
	redis.call('ZREMRANGEBYSCORE', apiKey, '-inf', expireBefore)
	local accountExists = redis.call('ZSCORE', accountKey, leaseID) ~= false
	local userExists = redis.call('ZSCORE', userKey, leaseID) ~= false
	local apiExists = redis.call('ZSCORE', apiKey, leaseID) ~= false
	if accountExists and userExists and apiExists then
		return 1
	end
	-- A partial lease can only result from manual eviction/corruption. Remove it
	-- before re-evaluating limits so retries cannot retain a split reservation.
	redis.call('ZREM', accountKey, leaseID)
	redis.call('ZREM', userKey, leaseID)
	redis.call('ZREM', apiKey, leaseID)
	local accountCount = redis.call('ZCARD', accountKey)
	local userCount = redis.call('ZCARD', userKey)
	local apiCount = redis.call('ZCARD', apiKey)
	if accountMax > 0 and accountCount >= accountMax + accountAllowance then return 0 end
	if userMax > 0 and userCount >= userMax + userAllowance then return 0 end
	if apiMax > 0 and apiCount >= apiMax + apiAllowance then return 0 end
	-- Encode the shorter Live expiry in the ordinary score domain. Existing
	-- readers use now-slotTTL as their cutoff, so this member expires at
	-- now+liveTTL without adding work to ordinary request paths.
	local liveScore = now + liveTTL - slotTTL
	redis.call('ZADD', accountKey, liveScore, leaseID)
	redis.call('ZADD', userKey, liveScore, leaseID)
	redis.call('ZADD', apiKey, liveScore, leaseID)
	redis.call('EXPIRE', accountKey, slotTTL)
	redis.call('EXPIRE', userKey, slotTTL)
	redis.call('EXPIRE', apiKey, slotTTL)
	return 1
`)

var refreshLiveLeaseScript = redis.NewScript(`
	redis.replicate_commands()
	local slotTTL = tonumber(ARGV[1])
	local liveTTL = tonumber(ARGV[2])
	local leaseID = ARGV[3]
	local now = tonumber(redis.call('TIME')[1])
	local expireBefore = now - slotTTL
	for _, key in ipairs(KEYS) do
		redis.call('ZREMRANGEBYSCORE', key, '-inf', expireBefore)
		if redis.call('ZSCORE', key, leaseID) == false then return 0 end
	end
	local liveScore = now + liveTTL - slotTTL
	for _, key in ipairs(KEYS) do
		redis.call('ZADD', key, liveScore, leaseID)
		redis.call('EXPIRE', key, slotTTL)
	end
	return 1
`)

func (c *concurrencyCache) AcquireLiveLease(
	ctx context.Context,
	accountID int64,
	accountMax int,
	userID int64,
	userMax int,
	apiKeyID int64,
	apiKeyMax int,
	leaseID string,
	replacements service.LiveConcurrencyReplacements,
) (bool, error) {
	if c == nil || c.rdb == nil || accountID <= 0 || userID <= 0 || apiKeyID <= 0 || leaseID == "" {
		return false, nil
	}
	accountAllowance := boolInt(replacements.Account)
	userAllowance := boolInt(replacements.User)
	apiKeyAllowance := boolInt(replacements.APIKey)
	result, err := acquireLiveLeaseScript.Run(ctx, c.rdb, []string{
		accountSlotKey(accountID),
		userSlotKey(userID),
		apiKeySlotKey(apiKeyID),
	},
		accountMax,
		userMax,
		apiKeyMax,
		c.slotTTLSeconds,
		liveLeaseTTLSeconds,
		leaseID,
		accountAllowance,
		userAllowance,
		apiKeyAllowance,
	).Int()
	return result == 1, err
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (c *concurrencyCache) RefreshLiveLease(ctx context.Context, accountID, userID, apiKeyID int64, leaseID string) (bool, error) {
	if c == nil || c.rdb == nil || leaseID == "" {
		return false, nil
	}
	result, err := refreshLiveLeaseScript.Run(ctx, c.rdb, []string{
		accountSlotKey(accountID),
		userSlotKey(userID),
		apiKeySlotKey(apiKeyID),
	}, c.slotTTLSeconds, liveLeaseTTLSeconds, leaseID).Int()
	return result == 1, err
}

func (c *concurrencyCache) ReleaseLiveLease(ctx context.Context, accountID, userID, apiKeyID int64, leaseID string) error {
	if c == nil || c.rdb == nil || leaseID == "" {
		return nil
	}
	pipe := c.rdb.TxPipeline()
	pipe.ZRem(ctx, accountSlotKey(accountID), leaseID)
	pipe.ZRem(ctx, userSlotKey(userID), leaseID)
	pipe.ZRem(ctx, apiKeySlotKey(apiKeyID), leaseID)
	_, err := pipe.Exec(ctx)
	if err == nil {
		c.refreshAccountActiveIndex(ctx, accountID)
		c.refreshUserActiveIndex(ctx, userID)
	}
	return err
}

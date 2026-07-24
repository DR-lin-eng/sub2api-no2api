package repository

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

const (
	priorityAccountWaitPrefix          = "concurrency:priority_admission:account:"
	priorityUserWaitPrefix             = "concurrency:priority_admission:user:"
	priorityAdmissionExpiredCleanLimit = 256
)

var priorityAccountAcquireScript = redis.NewScript(`
	redis.replicate_commands()
	local slotKey = KEYS[1]
	local priorityKey = KEYS[2]
	local normalKey = KEYS[3]
	local deadlineKey = KEYS[4]
		local stateKey = KEYS[5]
		local waitCountKey = KEYS[6]
		local activeIndexKey = KEYS[7]

	local maxConcurrency = tonumber(ARGV[1])
	local slotTTL = tonumber(ARGV[2])
	local requestID = ARGV[3]
	local tier = tonumber(ARGV[4])
	local register = tonumber(ARGV[5])
	local maxWaiting = tonumber(ARGV[6])
	local waitTimeoutMillis = tonumber(ARGV[7])
		local queueTTL = tonumber(ARGV[8])
		local cleanupLimit = tonumber(ARGV[9])
		local entityID = ARGV[10]

	local redisTime = redis.call('TIME')
	local nowSeconds = tonumber(redisTime[1])
	local nowMicros = tonumber(redisTime[2])
	local nowMillis = nowSeconds * 1000 + math.floor(nowMicros / 1000)
	local enqueueScore = nowSeconds + nowMicros / 1000000

	redis.call('ZREMRANGEBYSCORE', slotKey, '-inf', nowSeconds - slotTTL)
	local function updateWaitCount()
		local count = redis.call('ZCARD', priorityKey) + redis.call('ZCARD', normalKey)
		if count > 0 then
			redis.call('SET', waitCountKey, count, 'EX', queueTTL)
		else
			redis.call('DEL', waitCountKey)
		end
		return count
	end

	local function touchActiveIndex(ttl)
		redis.call('ZADD', activeIndexKey, nowSeconds + ttl, entityID)
	end

	-- An idempotent retry after a lost response refreshes the existing active
	-- member and never consumes a second slot.
	if redis.call('ZSCORE', slotKey, requestID) ~= false then
		redis.call('ZADD', slotKey, nowSeconds, requestID)
		redis.call('EXPIRE', slotKey, slotTTL)
		redis.call('ZREM', priorityKey, requestID)
		redis.call('ZREM', normalKey, requestID)
			redis.call('ZREM', deadlineKey, requestID)
			updateWaitCount()
			touchActiveIndex(slotTTL)
		return {1, nowSeconds}
	end

	-- Queue keys do not exist on the unsaturated hot path. EXISTS avoids three
	-- sorted-set operations in that common case while preserving bounded expiry
	-- cleanup whenever either protected queue is present.
	local priorityCount = 0
	local normalCount = 0
	if redis.call('EXISTS', priorityKey, normalKey) > 0 then
		local expired = redis.call('ZRANGEBYSCORE', deadlineKey, '-inf', nowMillis, 'LIMIT', 0, cleanupLimit)
		for _, member in ipairs(expired) do
			redis.call('ZREM', priorityKey, member)
			redis.call('ZREM', normalKey, member)
			redis.call('ZREM', deadlineKey, member)
		end
		priorityCount = redis.call('ZCARD', priorityKey)
		normalCount = redis.call('ZCARD', normalKey)
		if #expired > 0 then
			updateWaitCount()
		end
	end
	local activeCount = redis.call('ZCARD', slotKey)

	-- Low-tier requests are immediate-only and may not consume capacity while
	-- either protected tier has queued work.
	if tier == 2 then
		if activeCount < maxConcurrency and priorityCount == 0 and normalCount == 0 then
				redis.call('ZADD', slotKey, nowSeconds, requestID)
				redis.call('EXPIRE', slotKey, slotTTL)
				touchActiveIndex(slotTTL)
				return {1, nowSeconds}
		end
		return {0, nowSeconds}
	end

	local queuedInPriority = redis.call('ZSCORE', priorityKey, requestID) ~= false
	local queuedInNormal = redis.call('ZSCORE', normalKey, requestID) ~= false
	local queued = queuedInPriority or queuedInNormal

	-- With no backlog, the first protected-tier caller takes a free slot in a
	-- single round trip. The scheduler fast path never registers a waiter.
	if not queued and priorityCount == 0 and normalCount == 0 and activeCount < maxConcurrency then
			redis.call('ZADD', slotKey, nowSeconds, requestID)
			redis.call('EXPIRE', slotKey, slotTTL)
			touchActiveIndex(slotTTL)
			return {1, nowSeconds}
	end

	if not queued then
		if register ~= 1 then
			return {0, nowSeconds}
		end
		local totalCount = priorityCount + normalCount
		local tierLimit = maxWaiting - math.ceil(maxWaiting / 4)
		local currentTierCount = normalCount
		if tier == 0 then
			currentTierCount = priorityCount
		end
		if maxWaiting <= 0 or totalCount >= maxWaiting or currentTierCount >= tierLimit then
			return {3, nowSeconds}
		end
		local targetKey = normalKey
		if tier == 0 then
			targetKey = priorityKey
		end
		redis.call('ZADD', targetKey, enqueueScore, requestID)
		redis.call('ZADD', deadlineKey, nowMillis + waitTimeoutMillis, requestID)
		redis.call('EXPIRE', priorityKey, queueTTL)
		redis.call('EXPIRE', normalKey, queueTTL)
		redis.call('EXPIRE', deadlineKey, queueTTL)
		priorityCount = redis.call('ZCARD', priorityKey)
		normalCount = redis.call('ZCARD', normalKey)
		queuedInPriority = tier == 0
		queuedInNormal = tier ~= 0
	end

	activeCount = redis.call('ZCARD', slotKey)
	if activeCount < maxConcurrency then
		local priorityHeadResult = redis.call('ZRANGE', priorityKey, 0, 0)
		local normalHeadResult = redis.call('ZRANGE', normalKey, 0, 0)
		local priorityHead = priorityHeadResult[1]
		local normalHead = normalHeadResult[1]
		local streak = tonumber(redis.call('HGET', stateKey, 'priority_streak')) or 0
		local selected = nil
		local selectedTier = 1
		if priorityHead ~= nil and normalHead ~= nil then
			if streak >= 4 then
				selected = normalHead
				selectedTier = 1
			else
				selected = priorityHead
				selectedTier = 0
			end
		elseif priorityHead ~= nil then
			selected = priorityHead
			selectedTier = 0
		elseif normalHead ~= nil then
			selected = normalHead
			selectedTier = 1
		end

		if selected == requestID then
			redis.call('ZREM', priorityKey, requestID)
			redis.call('ZREM', normalKey, requestID)
			redis.call('ZREM', deadlineKey, requestID)
			redis.call('ZADD', slotKey, nowSeconds, requestID)
			redis.call('EXPIRE', slotKey, slotTTL)
			if selectedTier == 0 and normalHead ~= nil then
				redis.call('HSET', stateKey, 'priority_streak', math.min(streak + 1, 4))
			else
				redis.call('HSET', stateKey, 'priority_streak', 0)
			end
				redis.call('EXPIRE', stateKey, queueTTL)
				updateWaitCount()
				touchActiveIndex(slotTTL)
				return {1, nowSeconds}
		end
	end

		updateWaitCount()
		touchActiveIndex(queueTTL)
		return {2, nowSeconds}
`)

var priorityAccountCancelScript = redis.NewScript(`
		local slotKey = KEYS[1]
		local priorityKey = KEYS[2]
		local normalKey = KEYS[3]
		local deadlineKey = KEYS[4]
		local waitCountKey = KEYS[5]
		local requestID = ARGV[1]
		local queueTTL = tonumber(ARGV[2])
		local removed = redis.call('ZREM', slotKey, requestID)
		removed = removed + redis.call('ZREM', priorityKey, requestID)
	removed = removed + redis.call('ZREM', normalKey, requestID)
	redis.call('ZREM', deadlineKey, requestID)
	local count = redis.call('ZCARD', priorityKey) + redis.call('ZCARD', normalKey)
	if count > 0 then
		redis.call('SET', waitCountKey, count, 'EX', queueTTL)
	else
		redis.call('DEL', waitCountKey)
	end
	return removed
`)

type priorityAccountKeys struct {
	priority string
	normal   string
	deadline string
	state    string
}

func priorityWaitKeys(prefix string, id int64) priorityAccountKeys {
	base := prefix + strconv.FormatInt(id, 10)
	return priorityAccountKeys{
		priority: base + ":priority",
		normal:   base + ":normal",
		deadline: base + ":deadline",
		state:    base + ":state",
	}
}

func priorityWaitCountKey(prefix string, id int64) string {
	return prefix + strconv.FormatInt(id, 10) + ":count"
}

func (c *concurrencyCache) AcquirePriorityAccountSlot(ctx context.Context, request service.PriorityAccountAdmissionRequest) (service.PriorityAccountAdmissionStatus, error) {
	return c.acquirePrioritySlot(ctx, prioritySlotAcquireParams{
		id:             request.AccountID,
		maxConcurrency: request.MaxConcurrency,
		maxWaiting:     request.MaxWaiting,
		tier:           request.Tier,
		requestID:      request.RequestID,
		waitTimeout:    request.WaitTimeout,
		register:       request.Register,
		slotKey:        accountSlotKey(request.AccountID),
		waitCountKey:   priorityWaitCountKey(priorityAccountWaitPrefix, request.AccountID),
		waitKeyPrefix:  priorityAccountWaitPrefix,
		activeIndexKey: accountActiveIndexKey,
	})
}

func (c *concurrencyCache) AcquirePriorityUserSlot(ctx context.Context, request service.PriorityUserAdmissionRequest) (service.PriorityAccountAdmissionStatus, error) {
	return c.acquirePrioritySlot(ctx, prioritySlotAcquireParams{
		id:             request.UserID,
		maxConcurrency: request.MaxConcurrency,
		maxWaiting:     request.MaxWaiting,
		tier:           request.Tier,
		requestID:      request.RequestID,
		waitTimeout:    request.WaitTimeout,
		register:       request.Register,
		slotKey:        userSlotKey(request.UserID),
		waitCountKey:   priorityWaitCountKey(priorityUserWaitPrefix, request.UserID),
		waitKeyPrefix:  priorityUserWaitPrefix,
		activeIndexKey: userActiveIndexKey,
	})
}

type prioritySlotAcquireParams struct {
	id             int64
	maxConcurrency int
	maxWaiting     int
	tier           service.RequestSchedulingTier
	requestID      string
	waitTimeout    time.Duration
	register       bool
	slotKey        string
	waitCountKey   string
	waitKeyPrefix  string
	activeIndexKey string
}

func (c *concurrencyCache) acquirePrioritySlot(ctx context.Context, params prioritySlotAcquireParams) (service.PriorityAccountAdmissionStatus, error) {
	if c == nil || c.rdb == nil {
		return service.PriorityAccountAdmissionRejected, fmt.Errorf("redis client is unavailable")
	}
	if params.id <= 0 || params.maxConcurrency <= 0 || params.requestID == "" {
		return service.PriorityAccountAdmissionRejected, nil
	}
	tier := params.tier
	if !tier.Valid() {
		tier = service.RequestSchedulingTierNormal
	}
	waitTimeoutMillis := params.waitTimeout.Milliseconds()
	if waitTimeoutMillis <= 0 {
		waitTimeoutMillis = 1
	}
	register := 0
	if params.register {
		register = 1
	}
	keys := priorityWaitKeys(params.waitKeyPrefix, params.id)
	status, _, err := runScriptInt64Pair(
		ctx,
		c.rdb,
		priorityAccountAcquireScript,
		[]string{
			params.slotKey,
			keys.priority,
			keys.normal,
			keys.deadline,
			keys.state,
			params.waitCountKey,
			params.activeIndexKey,
		},
		params.maxConcurrency,
		c.slotTTLSeconds,
		params.requestID,
		int(tier),
		register,
		params.maxWaiting,
		waitTimeoutMillis,
		c.waitQueueTTLSeconds,
		priorityAdmissionExpiredCleanLimit,
		params.id,
	)
	if err != nil {
		return service.PriorityAccountAdmissionRejected, err
	}
	return service.PriorityAccountAdmissionStatus(status), nil
}

func (c *concurrencyCache) CancelPriorityAccountWait(ctx context.Context, accountID int64, requestID string) error {
	if c == nil || c.rdb == nil || accountID <= 0 || requestID == "" {
		return nil
	}
	keys := priorityWaitKeys(priorityAccountWaitPrefix, accountID)
	if _, err := priorityAccountCancelScript.Run(
		ctx,
		c.rdb,
		[]string{accountSlotKey(accountID), keys.priority, keys.normal, keys.deadline, priorityWaitCountKey(priorityAccountWaitPrefix, accountID)},
		requestID,
		c.waitQueueTTLSeconds,
	).Result(); err != nil {
		return err
	}
	c.refreshAccountActiveIndex(ctx, accountID)
	return nil
}

func (c *concurrencyCache) CancelPriorityUserWait(ctx context.Context, userID int64, requestID string) error {
	if c == nil || c.rdb == nil || userID <= 0 || requestID == "" {
		return nil
	}
	keys := priorityWaitKeys(priorityUserWaitPrefix, userID)
	if _, err := priorityAccountCancelScript.Run(
		ctx,
		c.rdb,
		[]string{userSlotKey(userID), keys.priority, keys.normal, keys.deadline, priorityWaitCountKey(priorityUserWaitPrefix, userID)},
		requestID,
		c.waitQueueTTLSeconds,
	).Result(); err != nil {
		return err
	}
	c.refreshUserActiveIndex(ctx, userID)
	return nil
}

var _ service.PriorityAdmissionCache = (*concurrencyCache)(nil)

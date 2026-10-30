package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	clientip "github.com/Wei-Shaw/sub2api/internal/shared/ip"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimitFailureMode Redis 故障策略
type RateLimitFailureMode int

const (
	RateLimitFailOpen RateLimitFailureMode = iota
	RateLimitFailClose
)

// RateLimitOptions 限流可选配置
type RateLimitOptions struct {
	FailureMode RateLimitFailureMode
	// IPv6PrefixBits 按网段（如 64）合并 IPv6 客户端计数，避免轮换同一分配内的地址绕过限流；0 表示按单个地址计数。
	IPv6PrefixBits int
}

var rateLimitScript = redis.NewScript(`
local current = redis.call('INCR', KEYS[1])
local ttl = redis.call('PTTL', KEYS[1])
local repaired = 0
if current == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
  ttl = tonumber(ARGV[1])
elseif ttl == -1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
  ttl = tonumber(ARGV[1])
  repaired = 1
end
return {current, repaired, ttl}
`)

var rateLimitManyScript = redis.NewScript(`
local window = tonumber(ARGV[1])
local allowed = 1
local retry_after = 0
local repaired = 0
local counts = {}
local blocked = false

for i, key in ipairs(KEYS) do
  if blocked then
    counts[i] = 0
  else
    local current = redis.call('INCR', key)
    local ttl = redis.call('PTTL', key)
    if current == 1 then
      redis.call('PEXPIRE', key, window)
      ttl = window
    elseif ttl == -1 then
      redis.call('PEXPIRE', key, window)
      ttl = window
      repaired = repaired + 1
    end

    counts[i] = current
    local limit = tonumber(ARGV[i + 1])
    if limit > 0 and current > limit then
      allowed = 0
      blocked = true
      retry_after = ttl
    end
  end
end

local result = {allowed, retry_after, repaired}
for i, count in ipairs(counts) do
  result[i + 3] = count
end
return result
`)

// rateLimitRun 允许测试覆写脚本执行逻辑
var rateLimitRun = func(ctx context.Context, client *redis.Client, key string, windowMillis int64) (int64, bool, int64, error) {
	values, err := rateLimitScript.Run(ctx, client, []string{key}, windowMillis).Slice()
	if err != nil {
		return 0, false, 0, err
	}
	if len(values) < 3 {
		return 0, false, 0, fmt.Errorf("rate limit script returned %d values", len(values))
	}
	count, err := parseInt64(values[0])
	if err != nil {
		return 0, false, 0, err
	}
	repaired, err := parseInt64(values[1])
	if err != nil {
		return 0, false, 0, err
	}
	ttlMillis, err := parseInt64(values[2])
	if err != nil {
		return 0, false, 0, err
	}
	return count, repaired == 1, ttlMillis, nil
}

type rateLimitManyScriptResult struct {
	allowed         bool
	retryAfterMilli int64
	repaired        int64
	counts          []int64
}

// rateLimitManyRun allows tests to replace the Redis script execution while
// keeping the public batching contract stable.
var rateLimitManyRun = func(ctx context.Context, client *redis.Client, keys []string, limits []int, windowMillis int64) (rateLimitManyScriptResult, error) {
	args := make([]any, 0, len(limits)+1)
	args = append(args, windowMillis)
	for _, limit := range limits {
		args = append(args, limit)
	}
	values, err := rateLimitManyScript.Run(ctx, client, keys, args...).Slice()
	if err != nil {
		return rateLimitManyScriptResult{}, err
	}
	if len(values) != len(keys)+3 {
		return rateLimitManyScriptResult{}, fmt.Errorf("rate limit batch script returned %d values for %d keys", len(values), len(keys))
	}
	allowed, err := parseInt64(values[0])
	if err != nil {
		return rateLimitManyScriptResult{}, err
	}
	retryAfterMilli, err := parseInt64(values[1])
	if err != nil {
		return rateLimitManyScriptResult{}, err
	}
	repaired, err := parseInt64(values[2])
	if err != nil {
		return rateLimitManyScriptResult{}, err
	}
	result := rateLimitManyScriptResult{
		allowed:         allowed == 1,
		retryAfterMilli: retryAfterMilli,
		repaired:        repaired,
		counts:          make([]int64, len(keys)),
	}
	for i := range keys {
		result.counts[i], err = parseInt64(values[i+3])
		if err != nil {
			return rateLimitManyScriptResult{}, err
		}
	}
	return result, nil
}

// RateLimiter Redis 速率限制器
type RateLimiter struct {
	redis  *redis.Client
	prefix string
}

// NewRateLimiter 创建速率限制器实例
func NewRateLimiter(redisClient *redis.Client) *RateLimiter {
	return &RateLimiter{
		redis:  redisClient,
		prefix: "rate_limit:",
	}
}

// AllowResult 单次固定窗口限流判定结果。
type AllowResult struct {
	// Allowed 是否放行
	Allowed bool
	// Count 当前窗口内累计请求数（含本次）
	Count int64
	// RetryAfter 超限时距窗口重置的剩余时间（尽力而为；PTTL 不可用时回退为完整窗口）
	RetryAfter time.Duration
}

// RateLimitRule describes one fixed-window bucket checked by AllowMany.
type RateLimitRule struct {
	Key   string
	Limit int
}

// AllowManyResult is the aggregate result of one atomic multi-bucket check.
// Counts has the same order as the supplied rules.
type AllowManyResult struct {
	Allowed    bool
	Counts     []int64
	RetryAfter time.Duration
}

// Allow 对给定 key（不含 "rate_limit:" 前缀）执行一次固定窗口计数判定。
// 供需要自定义限流维度（如按用户 ID）的调用方使用；Redis 错误由调用方决定 fail-open/close。
func (r *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (AllowResult, error) {
	if r == nil || r.redis == nil {
		return AllowResult{}, fmt.Errorf("rate limiter redis client is nil")
	}
	redisKey := r.prefix + key
	windowMillis := windowTTLMillis(window)

	count, repaired, ttlMillis, err := rateLimitRun(ctx, r.redis, redisKey, windowMillis)
	if err != nil {
		return AllowResult{}, err
	}
	if repaired {
		log.Printf("[RateLimit] ttl repaired: key=%s window_ms=%d", redisKey, windowMillis)
	}

	result := AllowResult{Allowed: count <= int64(limit), Count: count}
	if !result.Allowed {
		result.RetryAfter = window
		if ttlMillis > 0 {
			result.RetryAfter = time.Duration(ttlMillis) * time.Millisecond
		}
	}
	return result, nil
}

// AllowMany checks ordered buckets in one Redis EVAL. It stops incrementing
// after the first rejection, matching stacked middleware semantics (for
// example, a rejected global bucket never consumes the downstream heavy
// bucket). Counts for skipped rules are zero.
func (r *RateLimiter) AllowMany(ctx context.Context, rules []RateLimitRule, window time.Duration) (AllowManyResult, error) {
	result := AllowManyResult{Allowed: true, Counts: make([]int64, len(rules))}
	if len(rules) == 0 {
		return result, nil
	}
	if r == nil || r.redis == nil {
		return AllowManyResult{}, fmt.Errorf("rate limiter redis client is nil")
	}

	keys := make([]string, 0, len(rules))
	limits := make([]int, 0, len(rules))
	positions := make([]int, 0, len(rules))
	for i, rule := range rules {
		if rule.Limit <= 0 {
			continue
		}
		keys = append(keys, r.prefix+rule.Key)
		limits = append(limits, rule.Limit)
		positions = append(positions, i)
	}
	if len(keys) == 0 {
		return result, nil
	}

	scriptResult, err := rateLimitManyRun(ctx, r.redis, keys, limits, windowTTLMillis(window))
	if err != nil {
		return AllowManyResult{}, err
	}
	if scriptResult.repaired > 0 {
		log.Printf("[RateLimit] ttl repaired: keys=%d repaired=%d window_ms=%d", len(keys), scriptResult.repaired, windowTTLMillis(window))
	}
	result.Allowed = scriptResult.allowed
	for i, count := range scriptResult.counts {
		result.Counts[positions[i]] = count
	}
	if !result.Allowed {
		result.RetryAfter = time.Duration(scriptResult.retryAfterMilli) * time.Millisecond
		if result.RetryAfter <= 0 {
			result.RetryAfter = window
		}
	}
	return result, nil
}

// rateLimitClientKey 把 IPv6 地址归并到指定前缀的网段；IPv4 与无法解析的值保持原样。
func rateLimitClientKey(ip string, ipv6PrefixBits int) string {
	if ipv6PrefixBits <= 0 || ipv6PrefixBits >= 128 {
		return ip
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil || !addr.Is6() || addr.Is4In6() {
		return ip
	}
	prefix, err := addr.WithZone("").Prefix(ipv6PrefixBits)
	if err != nil {
		return ip
	}
	return prefix.String()
}

// Limit 返回速率限制中间件
// key: 限制类型标识
// limit: 时间窗口内最大请求数
// window: 时间窗口
func (r *RateLimiter) Limit(key string, limit int, window time.Duration) gin.HandlerFunc {
	return r.LimitWithOptions(key, limit, window, RateLimitOptions{})
}

// LimitWithOptions 返回速率限制中间件（带可选配置）
func (r *RateLimiter) LimitWithOptions(key string, limit int, window time.Duration, opts RateLimitOptions) gin.HandlerFunc {
	failureMode := opts.FailureMode
	if failureMode != RateLimitFailClose {
		failureMode = RateLimitFailOpen
	}

	return func(c *gin.Context) {
		result, err := r.Allow(c.Request.Context(), key+":"+rateLimitClientKey(clientip.GetClientIP(c), opts.IPv6PrefixBits), limit, window)
		if err != nil {
			log.Printf("[RateLimit] redis error: key=%s mode=%s err=%v", r.prefix+key, failureModeLabel(failureMode), err)
			if failureMode == RateLimitFailClose {
				abortRateLimit(c, window)
				return
			}
			// Redis 错误时放行，避免影响正常服务
			c.Next()
			return
		}

		// 超过限制
		if !result.Allowed {
			abortRateLimit(c, result.RetryAfter)
			return
		}

		c.Next()
	}
}

func windowTTLMillis(window time.Duration) int64 {
	ttl := window.Milliseconds()
	if ttl < 1 {
		return 1
	}
	return ttl
}

func abortRateLimit(c *gin.Context, retryAfter time.Duration) {
	if retryAfter > 0 {
		seconds := int64(retryAfter / time.Second)
		if retryAfter%time.Second > 0 {
			seconds++
		}
		c.Header("Retry-After", strconv.FormatInt(seconds, 10))
	}
	c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
		"error":   "rate limit exceeded",
		"message": "Too many requests, please try again later",
	})
}

func failureModeLabel(mode RateLimitFailureMode) string {
	if mode == RateLimitFailClose {
		return "fail-close"
	}
	return "fail-open"
}

func parseInt64(value any) (int64, error) {
	switch v := value.(type) {
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	case string:
		parsed, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, err
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("unexpected value type %T", value)
	}
}

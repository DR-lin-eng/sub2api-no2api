package service

import (
	"context"
	"math"
	"strings"
)

// OpenAIFailureCounterCache tracks consecutive OpenAI OAuth account-level
// 429/502 failures across application instances. A successful response resets it.
type OpenAIFailureCounterCache interface {
	IncrementOpenAIFailureCount(ctx context.Context, accountID int64) (int64, error)
	ResetOpenAIFailureCount(ctx context.Context, accountID int64) error
}

// OpenAIQuotaLimitChecker performs the live quota check used by the optional
// failure-circuit guard. Unknown or auxiliary buckets must not disable an
// otherwise usable account.
type OpenAIQuotaLimitChecker interface {
	IsQuotaLimitReached(ctx context.Context, accountID int64) (bool, error)
}

func isOpenAIQuotaUsageLimitReached(usage *OpenAIQuotaUsage) bool {
	if usage == nil {
		return false
	}
	if len(usage.RateLimitsByLimitID) > 0 {
		bucket, ok := usage.RateLimitsByLimitID["codex"]
		if !ok {
			for _, candidate := range usage.RateLimitsByLimitID {
				if strings.EqualFold(strings.TrimSpace(candidate.LimitID), "codex") {
					bucket = candidate
					ok = true
					break
				}
			}
		}
		if ok {
			return isOpenAIQuotaBucketLimitReached(&bucket)
		}
		if usage.RateLimit != nil && strings.EqualFold(strings.TrimSpace(usage.RateLimit.LimitID), "codex") {
			return isOpenAIRateLimitReached(usage.RateLimit)
		}
		return false
	}
	return isOpenAIRateLimitReached(usage.RateLimit)
}

func isOpenAIQuotaBucketLimitReached(bucket *OpenAIAppServerRateLimitBucket) bool {
	if bucket == nil {
		return false
	}
	if strings.TrimSpace(bucket.RateLimitReachedType) != "" {
		return true
	}
	if bucket.Primary != nil && isOpenAIQuotaPercentExhausted(bucket.Primary.UsedPercent) {
		return true
	}
	if bucket.Secondary != nil && isOpenAIQuotaPercentExhausted(bucket.Secondary.UsedPercent) {
		return true
	}
	return isOpenAIQuotaPercentExhausted(bucket.UsedPercent)
}

func isOpenAIRateLimitReached(rateLimit *OpenAIRateLimit) bool {
	if rateLimit == nil {
		return false
	}
	if rateLimit.LimitReached {
		return true
	}
	if rateLimit.PrimaryWindow != nil && isOpenAIQuotaPercentExhausted(rateLimit.PrimaryWindow.UsedPercent) {
		return true
	}
	return rateLimit.SecondaryWindow != nil && isOpenAIQuotaPercentExhausted(rateLimit.SecondaryWindow.UsedPercent)
}

func isOpenAIQuotaPercentExhausted(percent float64) bool {
	return !math.IsNaN(percent) && !math.IsInf(percent, 0) && percent >= 100
}

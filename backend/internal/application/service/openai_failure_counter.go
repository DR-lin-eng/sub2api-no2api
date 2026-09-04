package service

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
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

type OpenAIQuotaStatusChecker interface {
	QueryQuotaStatus(ctx context.Context, accountID int64) (limited bool, resetAt *time.Time, err error)
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

func isOpenAIQuotaUsageKnownAvailable(usage *OpenAIQuotaUsage) bool {
	if usage == nil || isOpenAIQuotaUsageLimitReached(usage) {
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
			return bucket.Primary != nil || bucket.Secondary != nil ||
				bucket.WindowDurationMins > 0 || bucket.ResetsAt > 0 || bucket.UsedPercent > 0
		}
		if usage.RateLimit == nil || !strings.EqualFold(strings.TrimSpace(usage.RateLimit.LimitID), "codex") {
			return false
		}
	}
	return usage.RateLimit != nil && (usage.RateLimit.Allowed ||
		usage.RateLimit.PrimaryWindow != nil || usage.RateLimit.SecondaryWindow != nil)
}

func openAIQuotaUsageMainResetAt(usage *OpenAIQuotaUsage, now time.Time) *time.Time {
	if usage == nil {
		return nil
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
			candidates := exhaustedOpenAIAppServerWindowResetTimes(&bucket)
			if len(candidates) == 0 && strings.TrimSpace(bucket.RateLimitReachedType) != "" {
				candidates = []*time.Time{
					openAIAppServerWindowResetAt(bucket.Primary),
					openAIAppServerWindowResetAt(bucket.Secondary),
					openAIQuotaUnixTime(bucket.ResetsAt),
				}
			}
			return latestOpenAIQuotaResetAt(now, candidates...)
		}
		if usage.RateLimit == nil || !strings.EqualFold(strings.TrimSpace(usage.RateLimit.LimitID), "codex") {
			return nil
		}
	}
	candidates := exhaustedOpenAILegacyWindowResetTimes(usage)
	if len(candidates) == 0 && usage.RateLimit != nil && usage.RateLimit.LimitReached {
		candidates = []*time.Time{
			openAILegacyWindowResetAt(usage, legacyOpenAIQuotaPrimaryWindow(usage)),
			openAILegacyWindowResetAt(usage, legacyOpenAIQuotaSecondaryWindow(usage)),
		}
	}
	return latestOpenAIQuotaResetAt(now, candidates...)
}

func exhaustedOpenAIAppServerWindowResetTimes(bucket *OpenAIAppServerRateLimitBucket) []*time.Time {
	if bucket == nil {
		return nil
	}
	candidates := make([]*time.Time, 0, 3)
	if bucket.Primary != nil && isOpenAIQuotaPercentExhausted(bucket.Primary.UsedPercent) {
		candidates = append(candidates, openAIAppServerWindowResetAt(bucket.Primary))
	}
	if bucket.Secondary != nil && isOpenAIQuotaPercentExhausted(bucket.Secondary.UsedPercent) {
		candidates = append(candidates, openAIAppServerWindowResetAt(bucket.Secondary))
	}
	if bucket.Primary == nil && bucket.Secondary == nil && isOpenAIQuotaPercentExhausted(bucket.UsedPercent) {
		candidates = append(candidates, openAIQuotaUnixTime(bucket.ResetsAt))
	}
	return candidates
}

func exhaustedOpenAILegacyWindowResetTimes(usage *OpenAIQuotaUsage) []*time.Time {
	if usage == nil || usage.RateLimit == nil {
		return nil
	}
	candidates := make([]*time.Time, 0, 2)
	if window := usage.RateLimit.PrimaryWindow; window != nil && isOpenAIQuotaPercentExhausted(window.UsedPercent) {
		candidates = append(candidates, openAILegacyWindowResetAt(usage, window))
	}
	if window := usage.RateLimit.SecondaryWindow; window != nil && isOpenAIQuotaPercentExhausted(window.UsedPercent) {
		candidates = append(candidates, openAILegacyWindowResetAt(usage, window))
	}
	return candidates
}

func legacyOpenAIQuotaPrimaryWindow(usage *OpenAIQuotaUsage) *OpenAIRateLimitWindow {
	if usage == nil || usage.RateLimit == nil {
		return nil
	}
	return usage.RateLimit.PrimaryWindow
}

func legacyOpenAIQuotaSecondaryWindow(usage *OpenAIQuotaUsage) *OpenAIRateLimitWindow {
	if usage == nil || usage.RateLimit == nil {
		return nil
	}
	return usage.RateLimit.SecondaryWindow
}

func openAIAppServerWindowResetAt(window *OpenAIAppServerRateLimitWindow) *time.Time {
	if window == nil {
		return nil
	}
	return openAIQuotaUnixTime(window.ResetsAt)
}

func openAILegacyWindowResetAt(usage *OpenAIQuotaUsage, window *OpenAIRateLimitWindow) *time.Time {
	if window == nil {
		return nil
	}
	if resetAt := openAIQuotaUnixTime(window.ResetAt); resetAt != nil {
		return resetAt
	}
	if window.ResetAfterSeconds <= 0 {
		return nil
	}
	base := time.Now()
	if usage != nil && usage.FetchedAt > 0 {
		base = time.Unix(usage.FetchedAt, 0)
	}
	resetAt := base.Add(time.Duration(window.ResetAfterSeconds) * time.Second)
	return &resetAt
}

func openAIQuotaUnixTime(unix int64) *time.Time {
	if unix <= 0 {
		return nil
	}
	resetAt := time.Unix(unix, 0)
	return &resetAt
}

func latestOpenAIQuotaResetAt(now time.Time, candidates ...*time.Time) *time.Time {
	var latest *time.Time
	for _, candidate := range candidates {
		if candidate == nil || !candidate.After(now) {
			continue
		}
		if latest == nil || candidate.After(*latest) {
			copy := *candidate
			latest = &copy
		}
	}
	return latest
}

func openAIFailureCircuitResetAt(account *Account, statusCode int, headers http.Header, responseBody []byte, now time.Time) *time.Time {
	candidates := make([]*time.Time, 0, 6)
	if statusCode == http.StatusTooManyRequests {
		if resetAt := calculateOpenAI429ResetTime(headers); resetAt != nil {
			candidates = append(candidates, resetAt)
		}
	}
	if resetUnix := parseOpenAIRateLimitResetTime(responseBody); resetUnix != nil {
		candidates = append(candidates, openAIQuotaUnixTime(*resetUnix))
	}
	if account == nil {
		return latestOpenAIQuotaResetAt(now, candidates...)
	}
	if account.RateLimitResetAt != nil {
		candidates = append(candidates, account.RateLimitResetAt)
	}
	for _, window := range []string{"5h", "7d", "primary", "secondary"} {
		used, ok := resolveAccountExtraNumber(account.Extra, "codex_"+window+"_used_percent")
		if !ok || !isOpenAIQuotaPercentExhausted(used) {
			continue
		}
		value, ok := account.Extra["codex_"+window+"_reset_at"]
		if !ok {
			continue
		}
		if resetAt, err := parseTime(strings.TrimSpace(valueString(value))); err == nil {
			candidates = append(candidates, &resetAt)
		}
	}
	return latestOpenAIQuotaResetAt(now, candidates...)
}

func valueString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return strings.TrimSpace(fmt.Sprint(value))
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

package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

type sub2APIUsageResponse struct {
	Mode         string               `json:"mode"`
	IsValid      *bool                `json:"isValid"`
	Status       string               `json:"status"`
	PlanName     string               `json:"planName"`
	Unit         string               `json:"unit"`
	Remaining    *float64             `json:"remaining"`
	Balance      *float64             `json:"balance"`
	Quota        *sub2APIQuota        `json:"quota"`
	RateLimits   []sub2APIRateLimit   `json:"rate_limits"`
	Subscription *sub2APISubscription `json:"subscription"`
	ExpiresAt    *time.Time           `json:"expires_at"`
}

type sub2APIQuota struct {
	Limit     *float64 `json:"limit"`
	Used      *float64 `json:"used"`
	Remaining *float64 `json:"remaining"`
	Unit      string   `json:"unit"`
}

type sub2APIRateLimit struct {
	Window      string          `json:"window"`
	Limit       *float64        `json:"limit"`
	Used        *float64        `json:"used"`
	Remaining   *float64        `json:"remaining"`
	WindowStart json.RawMessage `json:"window_start"`
	ResetAt     *time.Time      `json:"reset_at"`
}

type sub2APISubscription struct {
	DailyUsageUSD      *float64   `json:"daily_usage_usd"`
	WeeklyUsageUSD     *float64   `json:"weekly_usage_usd"`
	MonthlyUsageUSD    *float64   `json:"monthly_usage_usd"`
	DailyLimitUSD      *float64   `json:"daily_limit_usd"`
	WeeklyLimitUSD     *float64   `json:"weekly_limit_usd"`
	MonthlyLimitUSD    *float64   `json:"monthly_limit_usd"`
	DailyWindowStart   *time.Time `json:"daily_window_start"`
	WeeklyWindowStart  *time.Time `json:"weekly_window_start"`
	MonthlyWindowStart *time.Time `json:"monthly_window_start"`
	DailyResetAt       *time.Time `json:"daily_reset_at"`
	WeeklyResetAt      *time.Time `json:"weekly_reset_at"`
	MonthlyResetAt     *time.Time `json:"monthly_reset_at"`
	Unlimited          *bool      `json:"unlimited"`
	ExpiresAt          *time.Time `json:"expires_at"`
}

type newAPISubscriptionResponse struct {
	Object             string          `json:"object"`
	HasPaymentMethod   *bool           `json:"has_payment_method"`
	SoftLimitUSD       *float64        `json:"soft_limit_usd"`
	HardLimitUSD       *float64        `json:"hard_limit_usd"`
	SystemHardLimitUSD *float64        `json:"system_hard_limit_usd"`
	AccessUntil        *int64          `json:"access_until"`
	Error              json.RawMessage `json:"error"`
}

type newAPIUsageResponse struct {
	Object     string          `json:"object"`
	TotalUsage *float64        `json:"total_usage"`
	Error      json.RawMessage `json:"error"`
}

func parseSub2APIUsage(body []byte) (*UpstreamQuotaInfo, error) {
	var response sub2APIUsageResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	if response.IsValid == nil || !*response.IsValid {
		return nil, errors.New("invalid Sub2API key state")
	}
	switch response.Mode {
	case "quota_limited":
		return normalizeSub2APIQuotaLimited(&response)
	case "unrestricted":
		return normalizeSub2APIUnrestricted(&response)
	default:
		return nil, errors.New("unexpected Sub2API usage mode")
	}
}

func normalizeSub2APIQuotaLimited(response *sub2APIUsageResponse) (*UpstreamQuotaInfo, error) {
	if response.Status != StatusAPIKeyActive && response.Status != StatusAPIKeyQuotaExhausted && response.Status != StatusAPIKeyExpired {
		return nil, errors.New("invalid Sub2API key status")
	}
	windows, err := normalizeSub2APIRateLimits(response.RateLimits)
	if err != nil {
		return nil, err
	}
	subscription, err := normalizeSub2APISubscription(response.PlanName, response.Subscription, nil)
	if err != nil {
		return nil, err
	}
	if response.Quota == nil {
		if len(windows) == 0 || response.Remaining != nil || response.Unit != "" {
			return nil, errors.New("incomplete Sub2API rate limit response")
		}
		unit := ""
		if subscription != nil {
			unit = "USD"
		}
		return &UpstreamQuotaInfo{Provider: "sub2api", Mode: "rate_limits", Unit: unit, Windows: windows, Subscription: subscription}, nil
	}

	quota := response.Quota
	if quota.Unit != "USD" || response.Unit != quota.Unit || quota.Limit == nil || quota.Used == nil || quota.Remaining == nil || response.Remaining == nil || *quota.Limit <= 0 ||
		!validNonNegativeQuotaNumber(*quota.Limit) || !validNonNegativeQuotaNumber(*quota.Used) ||
		!validNonNegativeQuotaNumber(*quota.Remaining) || !validNonNegativeQuotaNumber(*response.Remaining) ||
		!equalBillingMultiplier(*quota.Remaining, math.Max(0, *quota.Limit-*quota.Used)) ||
		!equalBillingMultiplier(*quota.Remaining, *response.Remaining) {
		return nil, errors.New("invalid Sub2API quota response")
	}
	expiresAt, err := normalizedQuotaTime(response.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &UpstreamQuotaInfo{
		Provider: "sub2api", Mode: "quota", Unit: "USD", Remaining: quota.Remaining, Used: quota.Used, Total: quota.Limit,
		ExpiresAt: expiresAt, Windows: windows, Subscription: subscription,
	}, nil
}

func normalizeSub2APIUnrestricted(response *sub2APIUsageResponse) (*UpstreamQuotaInfo, error) {
	if response.Unit != "USD" || strings.TrimSpace(response.PlanName) == "" || response.Remaining == nil {
		return nil, errors.New("incomplete Sub2API unrestricted response")
	}
	if (response.Subscription == nil) == (response.Balance == nil) {
		return nil, errors.New("ambiguous Sub2API unrestricted response")
	}
	if response.Balance != nil {
		if !validQuotaNumber(*response.Balance) || !validQuotaNumber(*response.Remaining) || !equalBillingMultiplier(*response.Balance, *response.Remaining) {
			return nil, errors.New("invalid Sub2API balance response")
		}
		return &UpstreamQuotaInfo{Provider: "sub2api", Mode: "balance", Unit: "USD", Remaining: response.Balance}, nil
	}

	subscription, err := normalizeSub2APISubscription(response.PlanName, response.Subscription, response.Remaining)
	if err != nil || subscription == nil {
		return nil, errors.New("invalid Sub2API subscription")
	}
	if subscription.Unlimited && *response.Remaining != -1 {
		return nil, errors.New("inconsistent unlimited Sub2API subscription")
	}
	if !subscription.Unlimited && (!validNonNegativeQuotaNumber(*response.Remaining) || subscription.Remaining == nil || !equalBillingMultiplier(*response.Remaining, *subscription.Remaining)) {
		return nil, errors.New("inconsistent Sub2API subscription remaining")
	}
	return &UpstreamQuotaInfo{Provider: "sub2api", Mode: "subscription", Unit: "USD", Subscription: subscription}, nil
}

func normalizeSub2APIRateLimits(raw []sub2APIRateLimit) ([]UpstreamQuotaWindow, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(raw))
	windows := make([]UpstreamQuotaWindow, 0, len(raw))
	for _, item := range raw {
		if item.Window != "5h" && item.Window != "1d" && item.Window != "7d" {
			return nil, errors.New("unknown Sub2API rate limit window")
		}
		if _, exists := seen[item.Window]; exists {
			return nil, errors.New("duplicate Sub2API rate limit window")
		}
		seen[item.Window] = struct{}{}
		if item.Limit == nil || item.Used == nil || item.Remaining == nil || *item.Limit <= 0 ||
			!validNonNegativeQuotaNumber(*item.Limit) || !validNonNegativeQuotaNumber(*item.Used) ||
			!validNonNegativeQuotaNumber(*item.Remaining) || !equalBillingMultiplier(*item.Remaining, math.Max(0, *item.Limit-*item.Used)) {
			return nil, errors.New("invalid Sub2API rate limit window")
		}
		if err := validateSub2APIWindowStart(item.WindowStart); err != nil {
			return nil, err
		}
		resetAt, err := normalizedQuotaTime(item.ResetAt)
		if err != nil {
			return nil, err
		}
		windows = append(windows, UpstreamQuotaWindow{Name: item.Window, Used: item.Used, Limit: item.Limit, Remaining: item.Remaining, ResetAt: resetAt})
	}
	return windows, nil
}

func normalizeSub2APISubscription(planName string, subscription *sub2APISubscription, legacyRemaining *float64) (*UpstreamSubscriptionInfo, error) {
	if subscription == nil {
		return nil, nil
	}
	if strings.TrimSpace(planName) == "" || subscription.DailyUsageUSD == nil || subscription.WeeklyUsageUSD == nil || subscription.MonthlyUsageUSD == nil ||
		!validNonNegativeQuotaNumber(*subscription.DailyUsageUSD) || !validNonNegativeQuotaNumber(*subscription.WeeklyUsageUSD) || !validNonNegativeQuotaNumber(*subscription.MonthlyUsageUSD) {
		return nil, errors.New("invalid Sub2API subscription usage")
	}
	windows, err := normalizeSub2APISubscriptionWindows(subscription)
	if err != nil {
		return nil, err
	}
	expiresAt, err := normalizedQuotaTime(subscription.ExpiresAt)
	if err != nil || expiresAt == nil {
		return nil, errors.New("invalid Sub2API subscription expiry")
	}
	unlimited := subscription.Unlimited != nil && *subscription.Unlimited || subscription.Unlimited == nil && legacyRemaining != nil && *legacyRemaining == -1
	if unlimited {
		if len(windows) != 0 {
			return nil, errors.New("inconsistent unlimited Sub2API subscription")
		}
		return &UpstreamSubscriptionInfo{PlanName: strings.TrimSpace(planName), Unlimited: true, ExpiresAt: *expiresAt}, nil
	}
	if len(windows) == 0 {
		return nil, errors.New("missing Sub2API subscription limits")
	}
	minimumRemaining := *windows[0].Remaining
	for _, window := range windows[1:] {
		minimumRemaining = math.Min(minimumRemaining, *window.Remaining)
	}
	return &UpstreamSubscriptionInfo{PlanName: strings.TrimSpace(planName), Remaining: float64Ptr(minimumRemaining), ExpiresAt: *expiresAt, Windows: windows}, nil
}

func normalizeSub2APISubscriptionWindows(subscription *sub2APISubscription) ([]UpstreamQuotaWindow, error) {
	type windowInput struct {
		name                 string
		used, limit          *float64
		resetAt, legacyStart *time.Time
		duration             time.Duration
	}
	inputs := []windowInput{
		{name: "daily", used: subscription.DailyUsageUSD, limit: subscription.DailyLimitUSD, resetAt: subscription.DailyResetAt, legacyStart: subscription.DailyWindowStart, duration: 24 * time.Hour},
		{name: "weekly", used: subscription.WeeklyUsageUSD, limit: subscription.WeeklyLimitUSD, resetAt: subscription.WeeklyResetAt, legacyStart: subscription.WeeklyWindowStart, duration: 7 * 24 * time.Hour},
		{name: "monthly", used: subscription.MonthlyUsageUSD, limit: subscription.MonthlyLimitUSD, resetAt: subscription.MonthlyResetAt, legacyStart: subscription.MonthlyWindowStart, duration: 30 * 24 * time.Hour},
	}
	windows := make([]UpstreamQuotaWindow, 0, len(inputs))
	for _, input := range inputs {
		if input.limit == nil || *input.limit == 0 {
			continue
		}
		if *input.limit < 0 || !validNonNegativeQuotaNumber(*input.limit) {
			return nil, errors.New("invalid Sub2API subscription limit")
		}
		remaining := math.Max(0, *input.limit-*input.used)
		window := UpstreamQuotaWindow{Name: input.name, Used: input.used, Limit: input.limit, Remaining: float64Ptr(remaining)}
		resetAt, err := normalizedQuotaTime(input.resetAt)
		if err != nil {
			return nil, err
		}
		if resetAt == nil && input.legacyStart != nil {
			start, err := normalizedQuotaTime(input.legacyStart)
			if err != nil {
				return nil, err
			}
			legacyResetAt := start.Add(input.duration)
			resetAt = &legacyResetAt
		}
		window.ResetAt = resetAt
		windows = append(windows, window)
	}
	return windows, nil
}

func parseNewAPISubscription(body []byte) (*newAPISubscriptionResponse, error) {
	var response newAPISubscriptionResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	if hasUpstreamQuotaError(response.Error) || response.Object != "billing_subscription" || response.HasPaymentMethod == nil || response.SoftLimitUSD == nil || response.HardLimitUSD == nil || response.SystemHardLimitUSD == nil || response.AccessUntil == nil ||
		!validNonNegativeQuotaNumber(*response.SoftLimitUSD) || !validNonNegativeQuotaNumber(*response.HardLimitUSD) || !validNonNegativeQuotaNumber(*response.SystemHardLimitUSD) || *response.AccessUntil < 0 || *response.AccessUntil > 253402300799 {
		return nil, errors.New("invalid New API subscription response")
	}
	return &response, nil
}

func parseNewAPIUsage(body []byte) (*newAPIUsageResponse, error) {
	var response newAPIUsageResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	if hasUpstreamQuotaError(response.Error) || response.Object != "list" || response.TotalUsage == nil || !validNonNegativeQuotaNumber(*response.TotalUsage) {
		return nil, errors.New("invalid New API usage response")
	}
	return &response, nil
}

func hasUpstreamQuotaError(raw json.RawMessage) bool {
	trimmed := strings.TrimSpace(string(raw))
	return trimmed != "" && trimmed != "null"
}

func upstreamQuotaUnit(raw string) (string, bool) {
	switch unit := strings.TrimSpace(raw); unit {
	case "USD", "CNY", "TOKENS":
		return unit, true
	default:
		return "", false
	}
}

func validQuotaNumber(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func validNonNegativeQuotaNumber(value float64) bool { return value >= 0 && validQuotaNumber(value) }

func normalizedQuotaTime(value *time.Time) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	if value.IsZero() {
		return nil, fmt.Errorf("invalid quota timestamp")
	}
	normalized := value.UTC()
	return &normalized, nil
}

func validateSub2APIWindowStart(raw json.RawMessage) error {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return errors.New("missing Sub2API rate limit window_start")
	}
	if trimmed == "null" {
		return nil
	}
	var value time.Time
	if err := json.Unmarshal(raw, &value); err != nil || value.IsZero() {
		return errors.New("invalid Sub2API rate limit window_start")
	}
	return nil
}

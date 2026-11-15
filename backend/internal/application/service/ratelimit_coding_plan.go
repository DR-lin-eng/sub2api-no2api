package service

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

func isCodingPlanQuotaExhausted403(account *Account, body []byte, message string) bool {
	if account == nil {
		return false
	}
	switch account.GetCodingPlanProvider() {
	case PlatformKimi, PlatformZhipu, PlatformMiniMax:
	default:
		return false
	}
	message = strings.ToLower(message)
	return strings.EqualFold(strings.TrimSpace(gjson.GetBytes(body, "error.type").String()), "access_terminated_error") ||
		strings.Contains(message, "usage limit") || strings.Contains(message, "quota will reset")
}

// Reuse the already loaded quota snapshot. Only an exhausted future window
// proves a reset boundary; other windows must not shorten that cooldown.
func (s *RateLimitService) cooldownCodingPlanQuotaSnapshot(ctx context.Context, account *Account) bool {
	provider := account.GetCodingPlanProvider()
	now := time.Now()
	var resetAt time.Time
	for _, window := range [...]struct{ used, reset string }{
		{cnExtraSuffix5hUsed, cnExtraSuffix5hReset},
		{cnExtraSuffixWeeklyUsed, cnExtraSuffixWeeklyReset},
		{cnExtraSuffixMonthlyUsed, cnExtraSuffixMonthlyReset},
	} {
		if parseExtraFloat64(account.Extra[cnExtraKey(provider, window.used)]) < 100 {
			continue
		}
		raw, _ := account.Extra[cnExtraKey(provider, window.reset)].(string)
		boundary, err := time.Parse(time.RFC3339, raw)
		if err == nil && boundary.After(now) && (resetAt.IsZero() || boundary.After(resetAt)) {
			resetAt = boundary
		}
	}
	if resetAt.IsZero() {
		return false
	}
	if err := s.accountRepo.SetRateLimited(ctx, account.ID, resetAt); err != nil {
		slog.Warn("coding_plan_quota_cooldown_failed", "account_id", account.ID, "error", err)
		return false
	}
	s.notifyAccountSchedulingBlocked(account, resetAt, "cn_quota_exhausted")
	return true
}

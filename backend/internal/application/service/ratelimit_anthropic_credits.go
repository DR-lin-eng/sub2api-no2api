package service

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

const anthropicFableCreditsRequiredReason = "anthropic_fable_credits_required"

func (s *RateLimitService) persistAnthropicFableCreditsRequired(ctx context.Context, account *Account, headers http.Header, responseBody []byte, requestedModel string) bool {
	if s == nil || s.accountRepo == nil || account == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(gjson.GetBytes(responseBody, "error.details.error_code").String()), "credits_required") {
		return false
	}

	model := strings.TrimSpace(gjson.GetBytes(responseBody, "error.details.model").String())
	if model == "" {
		model = strings.TrimSpace(requestedModel)
	}
	if !isAnthropicFableModel(model) {
		return false
	}

	now := time.Now()
	resetAt, ok := parseAnthropicResetTimestamp(headers.Get("anthropic-ratelimit-unified-reset"), now, 366*24*time.Hour)
	if !ok {
		cooldown, enabled := s.get429FallbackCooldown(ctx, account)
		if !enabled {
			slog.Info("anthropic_fable_credits_required_cooldown_ignored", "account_id", account.ID)
			return true
		}
		resetAt = now.Add(cooldown)
	}

	if err := s.accountRepo.SetModelRateLimit(ctx, account.ID, anthropicFableRateLimitKey, resetAt, anthropicFableCreditsRequiredReason); err != nil {
		slog.Warn("anthropic_fable_credits_required_rate_limit_set_failed",
			"account_id", account.ID,
			"scope", anthropicFableRateLimitKey,
			"reset_at", resetAt,
			"error", err)
		// The response is still known to be Fable-specific. Do not widen a
		// persistence failure into an account-level rate limit.
		return true
	}
	slog.Info("anthropic_fable_credits_required_model_rate_limited",
		"account_id", account.ID,
		"scope", anthropicFableRateLimitKey,
		"reset_at", resetAt,
		"reset_in", time.Until(resetAt).Truncate(time.Second))
	return true
}

package service

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

func codexUsageUpdatesShowQuotaHeadroom(updates map[string]any) bool {
	if len(updates) == 0 {
		return false
	}

	seen := false
	for _, key := range []string{
		"codex_5h_used_percent",
		"codex_7d_used_percent",
		"codex_primary_used_percent",
		"codex_secondary_used_percent",
	} {
		value, ok := quotaPercentFromAny(updates[key])
		if !ok {
			continue
		}
		seen = true
		if value >= 100 {
			return false
		}
	}
	return seen
}

func clearStaleOpenAIQuotaModelRateLimits(ctx context.Context, repo AccountRepository, accountID int64) {
	if repo == nil || accountID <= 0 {
		return
	}

	account, err := repo.GetByID(ctx, accountID)
	if err != nil {
		slog.Warn("openai_codex_cooldown_recovery_get_account_failed", "account_id", accountID, "error", err)
		return
	}
	if account == nil || account.Platform != PlatformOpenAI || len(account.Extra) == 0 {
		return
	}

	cleaned, changed := filterOpenAIQuotaModelRateLimits(account.Extra[modelRateLimitsKey])
	if !changed {
		return
	}
	if err := repo.UpdateExtra(ctx, accountID, map[string]any{modelRateLimitsKey: cleaned}); err != nil {
		slog.Warn("openai_codex_cooldown_recovery_update_failed", "account_id", accountID, "error", err)
		return
	}
	slog.Info("openai_codex_cooldown_recovery_cleared_model_rate_limits", "account_id", accountID)
}

func maybeRecoverOpenAICodexQuotaModelRateLimits(ctx context.Context, repo AccountRepository, accountID int64, updates map[string]any) {
	if !codexUsageUpdatesShowQuotaHeadroom(updates) {
		return
	}

	go func() {
		recoveryCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		clearStaleOpenAIQuotaModelRateLimits(recoveryCtx, repo, accountID)
	}()
}

func filterOpenAIQuotaModelRateLimits(raw any) (map[string]any, bool) {
	limits, ok := raw.(map[string]any)
	if !ok || len(limits) == 0 {
		return nil, false
	}

	cleaned := make(map[string]any, len(limits))
	changed := false
	for model, rawLimit := range limits {
		limit, ok := rawLimit.(map[string]any)
		if ok && isOpenAIQuotaModelRateLimitReason(limit["reason"]) {
			changed = true
			continue
		}
		cleaned[model] = rawLimit
	}
	return cleaned, changed
}

func isOpenAIQuotaModelRateLimitReason(raw any) bool {
	reason := strings.TrimSpace(fmt.Sprint(raw))
	return reason == openAIModelRateLimitReason || strings.HasPrefix(reason, openAIModelRateLimitReason+":")
}

func quotaPercentFromAny(raw any) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	case int32:
		return float64(value), true
	case uint:
		return float64(value), true
	case uint64:
		return float64(value), true
	case uint32:
		return float64(value), true
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return 0, false
		}
		parsed, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

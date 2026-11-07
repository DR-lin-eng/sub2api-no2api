package service

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/tidwall/gjson"
)

// INTERNAL 500 渐进惩罚：连续多轮全部返回特定 500 错误时的惩罚时长
const (
	internal500PenaltyTier1Duration  = 30 * time.Minute // 第 1 轮：临时不可调度 30 分钟
	internal500PenaltyTier2Duration  = 2 * time.Hour    // 第 2 轮：临时不可调度 2 小时
	internal500PenaltyTier3Threshold = 3                // 第 3+ 轮：永久禁用
)

const defaultInternal500ImageCooldown = 2 * time.Minute

func internal500Duration(seconds int, fallback time.Duration) time.Duration {
	if seconds <= 0 || seconds > 604800 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}

func (s *AntigravityGatewayService) internal500ImageCooldown() time.Duration {
	if s == nil || s.settingService == nil || s.settingService.cfg == nil {
		return defaultInternal500ImageCooldown
	}
	return internal500Duration(s.settingService.cfg.Gateway.AntigravityInternal500ImageCooldownSeconds, defaultInternal500ImageCooldown)
}

func (s *AntigravityGatewayService) internal500PenaltyTiers() (time.Duration, time.Duration, bool) {
	tier1, tier2, disable := internal500PenaltyTier1Duration, internal500PenaltyTier2Duration, true
	if s != nil && s.settingService != nil && s.settingService.cfg != nil {
		tier1 = internal500Duration(s.settingService.cfg.Gateway.AntigravityInternal500Tier1Seconds, tier1)
		tier2 = internal500Duration(s.settingService.cfg.Gateway.AntigravityInternal500Tier2Seconds, tier2)
		if value := s.settingService.cfg.Gateway.AntigravityInternal500DisableAccount; value != nil {
			disable = *value
		}
	}
	return tier1, tier2, disable
}

// handleInternal500ForModel handles an INTERNAL 500 that survived every retry. Image models get a
// model-level cooldown only; anything else goes through the account penalty as upstream does.
func (s *AntigravityGatewayService) handleInternal500ForModel(
	ctx context.Context, prefix string, account *Account, modelKey string,
) {
	if account != nil && isImageGenerationModel(modelKey) {
		until := time.Now().Add(s.internal500ImageCooldown())
		if setModelRateLimitByModelName(ctx, s.accountRepo, account.ID, modelKey, prefix, http.StatusInternalServerError, until, false) {
			s.updateAccountModelRateLimitInCache(ctx, account, modelKey, until)
		}
		slog.Info("internal500_image_model_cooldown",
			"account_id", account.ID, "account_name", account.Name, "model", modelKey,
			"duration", time.Until(until).Truncate(time.Second))
		return
	}
	s.handleInternal500RetryExhausted(ctx, prefix, account)
}

// isAntigravityInternalServerError 检测特定的 INTERNAL 500 错误
// 必须同时匹配 error.code==500, error.message=="Internal error encountered.", error.status=="INTERNAL"
func isAntigravityInternalServerError(statusCode int, body []byte) bool {
	if statusCode != http.StatusInternalServerError {
		return false
	}
	return gjson.GetBytes(body, "error.code").Int() == 500 &&
		gjson.GetBytes(body, "error.message").String() == "Internal error encountered." &&
		gjson.GetBytes(body, "error.status").String() == "INTERNAL"
}

// applyInternal500Penalty 根据连续 INTERNAL 500 轮次数应用渐进惩罚
// count=1: temp_unschedulable 30 分钟
// count=2: temp_unschedulable 2 小时
// count>=3: SetError 永久禁用
func (s *AntigravityGatewayService) applyInternal500Penalty(
	ctx context.Context, prefix string, account *Account, count int64,
) {
	tier1, tier2, disable := s.internal500PenaltyTiers()
	if !disable && count > 2 {
		count = 2 // without the disable tier, later rounds repeat the second round
	}
	if count < int64(internal500PenaltyTier3Threshold) && !globalTempUnschedulableEnabled(ctx, s.settingService) {
		return
	}
	switch {
	case count >= int64(internal500PenaltyTier3Threshold):
		reason := fmt.Sprintf("INTERNAL 500 consecutive failures: %d rounds", count)
		if err := s.accountRepo.SetError(ctx, account.ID, reason); err != nil {
			slog.Error("internal500_set_error_failed", "account_id", account.ID, "error", err)
			return
		}
		slog.Warn("internal500_account_disabled",
			"account_id", account.ID, "account_name", account.Name, "consecutive_count", count)
	case count == 2:
		until := time.Now().Add(tier2)
		reason := fmt.Sprintf("INTERNAL 500 x%d (temp unsched %v)", count, tier2)
		if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, reason); err != nil {
			slog.Error("internal500_temp_unsched_failed", "account_id", account.ID, "error", err)
			return
		}
		slog.Warn("internal500_temp_unschedulable",
			"account_id", account.ID, "account_name", account.Name,
			"duration", tier2, "consecutive_count", count)
	case count == 1:
		until := time.Now().Add(tier1)
		reason := fmt.Sprintf("INTERNAL 500 x%d (temp unsched %v)", count, tier1)
		if err := s.accountRepo.SetTempUnschedulable(ctx, account.ID, until, reason); err != nil {
			slog.Error("internal500_temp_unsched_failed", "account_id", account.ID, "error", err)
			return
		}
		slog.Info("internal500_temp_unschedulable",
			"account_id", account.ID, "account_name", account.Name,
			"duration", tier1, "consecutive_count", count)
	}
}

// handleInternal500RetryExhausted 处理 INTERNAL 500 重试耗尽：递增计数器并应用惩罚
func (s *AntigravityGatewayService) handleInternal500RetryExhausted(
	ctx context.Context, prefix string, account *Account,
) {
	if s.internal500Cache == nil {
		return
	}
	count, err := s.internal500Cache.IncrementInternal500Count(ctx, account.ID)
	if err != nil {
		slog.Error("internal500_counter_increment_failed",
			"prefix", prefix, "account_id", account.ID, "error", err)
		return
	}
	s.applyInternal500Penalty(ctx, prefix, account, count)
}

// resetInternal500Counter 成功响应时清零 INTERNAL 500 计数器
func (s *AntigravityGatewayService) resetInternal500Counter(
	ctx context.Context, prefix string, accountID int64,
) {
	if s.internal500Cache == nil {
		return
	}
	if err := s.internal500Cache.ResetInternal500Count(ctx, accountID); err != nil {
		slog.Error("internal500_counter_reset_failed",
			"prefix", prefix, "account_id", accountID, "error", err)
	}
}

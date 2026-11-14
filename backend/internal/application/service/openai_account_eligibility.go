package service

import (
	"context"
	"log/slog"
)

// isOpenAICompatibleAccountEligibleForRequest 判断 OpenAI 兼容账号是否满足本次请求的调度条件。
// 检查内容包括：平台匹配、账号可用性、quota 自动暂停、spark 路由限制、模型支持及端点能力。
//
// 注意：对 spark 影子账号，调用方还须额外调用 parentHealthyForShadow(account, lookup)
// 检查母账号凭据可用性；该检查未内置于本函数，以避免注入 DB 依赖。
func isOpenAICompatibleAccountEligibleForRequest(ctx context.Context, account *Account, platform string, requestedModel string, requireCompact bool, requiredCapability OpenAIEndpointCapability) bool {
	platform = normalizeOpenAICompatiblePlatform(platform)
	if account == nil || !codexContinuationSchedulingMatches(ctx, account) || account.Platform != platform || !account.IsOpenAICompatible() || !account.IsSchedulableForModelWithContext(ctx, requestedModel) {
		return false
	}
	if !openAIPrivacyAllows(ctx, account) {
		return false
	}
	if vetoed, _ := openAIImageSizeVetoReason(ctx, account); vetoed {
		return false
	}
	if vetoed, _ := profitControlVetoReason(ctx, account); vetoed {
		return false
	}
	if account.IsOpenAI() {
		if paused, reason := shouldAutoPauseOpenAIAccountByQuota(ctx, account); paused {
			// Debug level: this fires per-candidate on the scheduling hot path, so Info
			// would amplify into log spam once several accounts cross the threshold.
			slog.Debug("account_auto_paused_by_quota",
				"account_id", account.ID,
				"window", reason.window,
				"threshold", reason.threshold,
				"utilization", reason.utilization,
			)
			return false
		}
	}
	if account.IsGrok() {
		if paused, reason := shouldAutoPauseGrokAccountByQuota(account); paused {
			slog.Debug("grok_account_auto_paused_by_quota",
				"account_id", account.ID,
				"window", reason.window,
				"threshold", reason.threshold,
				"utilization", reason.utilization,
			)
			return false
		}
	}
	if requestedModel != "" && !supportsOpenAIRequestModel(account, requestedModel, openAIRequestModelForSupport(ctx, requestedModel), requireCompact) {
		return false
	}
	if account.IsGrok() && requiredCapability == OpenAIEndpointCapabilityGrokMediaGeneration {
		eligible, reason := account.GrokMediaGenerationEligibility()
		if !eligible {
			slog.Debug("grok_media_account_ineligible", "account_id", account.ID, "reason", reason)
			return false
		}
	} else if !account.SupportsOpenAIEndpointCapability(requiredCapability) {
		return false
	}
	if requireCompact && openAICompactSupportTier(account) == 0 {
		return false
	}
	return true
}

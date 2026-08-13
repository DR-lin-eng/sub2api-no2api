package service

import (
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/shared/antigravity"
)

func writeGatewaySystemSettingUpdates(updates map[string]string, settings *SystemSettings) error {
	updates[SettingKeyAllowUngroupedKeyScheduling] = strconv.FormatBool(settings.AllowUngroupedKeyScheduling)
	updates[SettingKeySchedulerV2Enabled] = strconv.FormatBool(settings.SchedulerV2Enabled)
	updates[SettingKeySchedulerV2CandidateLimit] = strconv.Itoa(settings.SchedulerV2CandidateLimit)
	updates[SettingKeySchedulerV2ScanLimit] = strconv.Itoa(settings.SchedulerV2ScanLimit)
	updates[SettingKeyRequestPriorityAdmissionEnabled] = strconv.FormatBool(settings.RequestPriorityAdmissionEnabled)
	updates[SettingKeyRequestPriorityPendingLimitPerInstance] = strconv.Itoa(settings.RequestPriorityPendingLimitPerInstance)
	updates[SettingKeyRequestPriorityPendingMiBPerInstance] = strconv.Itoa(settings.RequestPriorityPendingMiBPerInstance)
	updates[SettingKeyBackendModeEnabled] = strconv.FormatBool(settings.BackendModeEnabled)
	updates[SettingKeyStreamModePerformanceEnabled] = strconv.FormatBool(settings.StreamModePerformanceEnabled)
	updates[SettingKeyOpenAIWSModeRouterV2Enabled] = strconv.FormatBool(settings.OpenAIWSModeRouterV2Enabled)
	updates[SettingKeyOpenAIVisibleOutputTTFTEnabled] = strconv.FormatBool(settings.OpenAIVisibleOutputTTFTEnabled)

	updates[SettingKeyEnableFingerprintUnification] = strconv.FormatBool(settings.EnableFingerprintUnification)
	updates[SettingKeyEnableMetadataPassthrough] = strconv.FormatBool(settings.EnableMetadataPassthrough)
	updates[SettingKeyEnableCCHSigning] = strconv.FormatBool(settings.EnableCCHSigning)
	updates[SettingKeyEnableClaudeOAuthSystemPromptInjection] = strconv.FormatBool(settings.EnableClaudeOAuthSystemPromptInjection)
	updates[SettingKeyClaudeOAuthSystemPrompt] = settings.ClaudeOAuthSystemPrompt
	if err := ValidateClaudeOAuthSystemPromptBlocksConfig(settings.ClaudeOAuthSystemPromptBlocks); err != nil {
		return err
	}
	updates[SettingKeyClaudeOAuthSystemPromptBlocks] = settings.ClaudeOAuthSystemPromptBlocks
	updates[SettingKeyEnableAnthropicCacheTTL1hInjection] = strconv.FormatBool(settings.EnableAnthropicCacheTTL1hInjection)
	updates[SettingKeyRewriteMessageCacheControl] = strconv.FormatBool(settings.RewriteMessageCacheControl)
	updates[SettingKeyEnableClientDatelineNormalization] = strconv.FormatBool(settings.EnableClientDatelineNormalization)
	updates[SettingKeyAntigravityUserAgentVersion] = antigravity.NormalizeUserAgentVersion(settings.AntigravityUserAgentVersion)
	updates[SettingKeyOpenAICodexUserAgent] = strings.TrimSpace(settings.OpenAICodexUserAgent)
	updates[SettingKeyOpenAICodexClientVersion] = NormalizeCodexClientVersion(settings.OpenAICodexClientVersion)
	updates[SettingKeyOpenAICodexVersionAutoSyncEnabled] = strconv.FormatBool(settings.OpenAICodexVersionAutoSyncEnabled)
	updates[SettingKeyMinCodexVersion] = strings.TrimSpace(settings.MinCodexVersion)
	updates[SettingKeyMaxCodexVersion] = strings.TrimSpace(settings.MaxCodexVersion)
	updates[SettingKeyCodexCLIOnlyBlacklist] = strings.TrimSpace(settings.CodexCLIOnlyBlacklist)
	updates[SettingKeyCodexCLIOnlyWhitelist] = strings.TrimSpace(settings.CodexCLIOnlyWhitelist)
	updates[SettingKeyCodexCLIOnlyAllowAppServerClients] = strconv.FormatBool(settings.CodexCLIOnlyAllowAppServerClients)
	updates[SettingKeyCodexCLIOnlyEngineFingerprintSignals] = strings.TrimSpace(settings.CodexCLIOnlyEngineFingerprintSignals)
	updates[SettingPaymentVisibleMethodAlipaySource] = settings.PaymentVisibleMethodAlipaySource
	updates[SettingPaymentVisibleMethodWxpaySource] = settings.PaymentVisibleMethodWxpaySource
	updates[SettingPaymentVisibleMethodAlipayEnabled] = strconv.FormatBool(settings.PaymentVisibleMethodAlipayEnabled)
	updates[SettingPaymentVisibleMethodWxpayEnabled] = strconv.FormatBool(settings.PaymentVisibleMethodWxpayEnabled)
	updates[SettingKeyOpenAILowUpstreamRatePriorityEnabled] = strconv.FormatBool(settings.OpenAILowUpstreamRatePriorityEnabled)
	updates[SettingKeyOpenAIOAuthSchedulingRateMultiplier] = strconv.FormatFloat(settings.OpenAIOAuthSchedulingRateMultiplier, 'f', -1, 64)
	updates[SettingKeyOpenAIContentSessionBurstBalanceEnabled] = strconv.FormatBool(settings.OpenAIContentSessionBurstBalanceEnabled)
	updates[openAIAdvancedSchedulerSettingKey] = strconv.FormatBool(settings.OpenAIAdvancedSchedulerEnabled)
	updates[SettingKeyOpenAIAdvancedSchedulerStickyWeightedEnabled] = strconv.FormatBool(settings.OpenAIAdvancedSchedulerStickyWeightedEnabled)
	updates[SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled] = strconv.FormatBool(settings.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled)
	updates[SettingKeyOpenAIAdvancedSchedulerLBTopK] = settings.OpenAIAdvancedSchedulerLBTopK
	updates[SettingKeyOpenAIAdvancedSchedulerWeightPriority] = settings.OpenAIAdvancedSchedulerWeightPriority
	updates[SettingKeyOpenAIAdvancedSchedulerWeightLoad] = settings.OpenAIAdvancedSchedulerWeightLoad
	updates[SettingKeyOpenAIAdvancedSchedulerWeightQueue] = settings.OpenAIAdvancedSchedulerWeightQueue
	updates[SettingKeyOpenAIAdvancedSchedulerWeightErrorRate] = settings.OpenAIAdvancedSchedulerWeightErrorRate
	updates[SettingKeyOpenAIAdvancedSchedulerWeightTTFT] = settings.OpenAIAdvancedSchedulerWeightTTFT
	updates[SettingKeyOpenAIAdvancedSchedulerWeightReset] = settings.OpenAIAdvancedSchedulerWeightReset
	updates[SettingKeyOpenAIAdvancedSchedulerWeightQuotaHeadroom] = settings.OpenAIAdvancedSchedulerWeightQuotaHeadroom
	updates[SettingKeyOpenAIAdvancedSchedulerWeightUpstreamCost] = settings.OpenAIAdvancedSchedulerWeightUpstreamCost
	updates[SettingKeyOpenAIAdvancedSchedulerWeightPreviousResponse] = settings.OpenAIAdvancedSchedulerWeightPreviousResponse
	updates[SettingKeyOpenAIAdvancedSchedulerWeightSessionSticky] = settings.OpenAIAdvancedSchedulerWeightSessionSticky
	return nil
}

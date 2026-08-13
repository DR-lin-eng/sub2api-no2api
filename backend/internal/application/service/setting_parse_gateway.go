package service

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/shared/antigravity"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
)

func (s *SettingService) applyGatewaySettings(result *SystemSettings, settings map[string]string) {
	if value, ok := settings[SettingKeyOpenAIWSModeRouterV2Enabled]; ok && strings.TrimSpace(value) != "" {
		result.OpenAIWSModeRouterV2Enabled = strings.EqualFold(strings.TrimSpace(value), "true")
	} else {
		result.OpenAIWSModeRouterV2Enabled = s.defaultOpenAIWSModeRouterV2Enabled()
	}
	if value, ok := settings[SettingKeyOpenAIVisibleOutputTTFTEnabled]; ok && strings.TrimSpace(value) != "" {
		result.OpenAIVisibleOutputTTFTEnabled = strings.EqualFold(strings.TrimSpace(value), "true")
	} else {
		result.OpenAIVisibleOutputTTFTEnabled = true
	}
	// Gateway forwarding behavior (defaults: fingerprint=true, metadata_passthrough=false,
	// cch_signing=false, claude_oauth_system_prompt_injection=true)
	if v, ok := settings[SettingKeyEnableFingerprintUnification]; ok && v != "" {
		result.EnableFingerprintUnification = v == "true"
	} else {
		result.EnableFingerprintUnification = true // default: enabled (current behavior)
	}
	result.EnableMetadataPassthrough = settings[SettingKeyEnableMetadataPassthrough] == "true"
	result.EnableCCHSigning = settings[SettingKeyEnableCCHSigning] == "true"
	if v, ok := settings[SettingKeyEnableClaudeOAuthSystemPromptInjection]; ok && v != "" {
		result.EnableClaudeOAuthSystemPromptInjection = v == "true"
	} else {
		result.EnableClaudeOAuthSystemPromptInjection = true
	}
	result.ClaudeOAuthSystemPrompt = settings[SettingKeyClaudeOAuthSystemPrompt]
	result.ClaudeOAuthSystemPromptBlocks = settings[SettingKeyClaudeOAuthSystemPromptBlocks]
	result.EnableAnthropicCacheTTL1hInjection = settings[SettingKeyEnableAnthropicCacheTTL1hInjection] == "true"
	if v, ok := settings[SettingKeyRewriteMessageCacheControl]; ok && v != "" {
		result.RewriteMessageCacheControl = v == "true"
	} else {
		result.RewriteMessageCacheControl = s.defaultRewriteMessageCacheControl()
	}
	if v, ok := settings[SettingKeyEnableClientDatelineNormalization]; ok && v != "" {
		result.EnableClientDatelineNormalization = v == "true"
	} else {
		result.EnableClientDatelineNormalization = true
	}
	result.AntigravityUserAgentVersion = antigravity.NormalizeUserAgentVersion(settings[SettingKeyAntigravityUserAgentVersion])
	result.OpenAICodexUserAgent = strings.TrimSpace(settings[SettingKeyOpenAICodexUserAgent])
	result.OpenAICodexClientVersion = NormalizeCodexClientVersion(settings[SettingKeyOpenAICodexClientVersion])
	result.OpenAICodexClientVersionSynced = NormalizeCodexClientVersion(settings[SettingKeyOpenAICodexClientVersionSynced])
	if value := strings.TrimSpace(settings[SettingKeyOpenAICodexVersionAutoSyncEnabled]); value != "" {
		result.OpenAICodexVersionAutoSyncEnabled = value == "true"
	} else {
		result.OpenAICodexVersionAutoSyncEnabled = true
	}
	// codex_cli_only 加固
	result.MinCodexVersion = settings[SettingKeyMinCodexVersion]
	result.MaxCodexVersion = settings[SettingKeyMaxCodexVersion]
	result.CodexCLIOnlyBlacklist = settings[SettingKeyCodexCLIOnlyBlacklist]
	result.CodexCLIOnlyWhitelist = settings[SettingKeyCodexCLIOnlyWhitelist]
	result.CodexCLIOnlyAllowAppServerClients = settings[SettingKeyCodexCLIOnlyAllowAppServerClients] == "true"
	if raw := strings.TrimSpace(settings[SettingKeyCodexCLIOnlyEngineFingerprintSignals]); raw != "" {
		result.CodexCLIOnlyEngineFingerprintSignals = raw
	} else {
		result.CodexCLIOnlyEngineFingerprintSignals = openai.DefaultEngineFingerprintSignalsJSON() // 缺失/空 → 展示默认种子
	}

	// Web search emulation: quick enabled check from the JSON config
	if raw := settings[SettingKeyWebSearchEmulationConfig]; raw != "" {
		var wsCfg WebSearchEmulationConfig
		if err := json.Unmarshal([]byte(raw), &wsCfg); err == nil {
			result.WebSearchEmulationEnabled = wsCfg.Enabled && len(wsCfg.Providers) > 0
		}
	}
	result.PaymentVisibleMethodAlipaySource = NormalizeVisibleMethodSource("alipay", settings[SettingPaymentVisibleMethodAlipaySource])
	result.PaymentVisibleMethodWxpaySource = NormalizeVisibleMethodSource("wxpay", settings[SettingPaymentVisibleMethodWxpaySource])
	result.PaymentVisibleMethodAlipayEnabled = settings[SettingPaymentVisibleMethodAlipayEnabled] == "true"
	result.PaymentVisibleMethodWxpayEnabled = settings[SettingPaymentVisibleMethodWxpayEnabled] == "true"
	result.OpenAILowUpstreamRatePriorityEnabled = settings[SettingKeyOpenAILowUpstreamRatePriorityEnabled] == "true"
	result.OpenAIOAuthSchedulingRateMultiplier = parseOpenAIOAuthSchedulingRateMultiplier(settings[SettingKeyOpenAIOAuthSchedulingRateMultiplier])
	result.OpenAIContentSessionBurstBalanceEnabled = settings[SettingKeyOpenAIContentSessionBurstBalanceEnabled] == "true"
	result.OpenAIAdvancedSchedulerEnabled = settings[openAIAdvancedSchedulerSettingKey] == "true"
	result.OpenAIAdvancedSchedulerStickyWeightedEnabled = settings[SettingKeyOpenAIAdvancedSchedulerStickyWeightedEnabled] == "true"
	result.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled = settings[SettingKeyOpenAIAdvancedSchedulerSubscriptionPriorityEnabled] == "true"
	result.OpenAIAdvancedSchedulerLBTopK = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerLBTopK])
	result.OpenAIAdvancedSchedulerWeightPriority = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightPriority])
	result.OpenAIAdvancedSchedulerWeightLoad = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightLoad])
	result.OpenAIAdvancedSchedulerWeightQueue = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightQueue])
	result.OpenAIAdvancedSchedulerWeightErrorRate = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightErrorRate])
	result.OpenAIAdvancedSchedulerWeightTTFT = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightTTFT])
	result.OpenAIAdvancedSchedulerWeightReset = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightReset])
	result.OpenAIAdvancedSchedulerWeightQuotaHeadroom = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightQuotaHeadroom])
	result.OpenAIAdvancedSchedulerWeightUpstreamCost = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightUpstreamCost])
	result.OpenAIAdvancedSchedulerWeightPreviousResponse = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightPreviousResponse])
	result.OpenAIAdvancedSchedulerWeightSessionSticky = strings.TrimSpace(settings[SettingKeyOpenAIAdvancedSchedulerWeightSessionSticky])
	result.OpenAIAdvancedSchedulerEffectiveLBTopK = s.openAIAdvancedSchedulerEffectiveLBTopK()
	effectiveWeights := s.openAIAdvancedSchedulerEffectiveWeights()
	result.OpenAIAdvancedSchedulerEffectiveWeightPriority = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.Priority)
	result.OpenAIAdvancedSchedulerEffectiveWeightLoad = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.Load)
	result.OpenAIAdvancedSchedulerEffectiveWeightQueue = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.Queue)
	result.OpenAIAdvancedSchedulerEffectiveWeightErrorRate = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.ErrorRate)
	result.OpenAIAdvancedSchedulerEffectiveWeightTTFT = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.TTFT)
	result.OpenAIAdvancedSchedulerEffectiveWeightReset = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.Reset)
	result.OpenAIAdvancedSchedulerEffectiveWeightQuotaHeadroom = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.QuotaHeadroom)
	result.OpenAIAdvancedSchedulerEffectiveWeightUpstreamCost = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.UpstreamCost)
	result.OpenAIAdvancedSchedulerEffectiveWeightPreviousResponse = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.PreviousResponse)
	result.OpenAIAdvancedSchedulerEffectiveWeightSessionSticky = formatOpenAIAdvancedSchedulerFloat(effectiveWeights.SessionSticky)
}

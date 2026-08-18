package service

import (
	"strconv"
	"strings"
)

func (s *SettingService) applyFeatureSettings(result *SystemSettings, settings map[string]string) {
	// Model fallback settings
	result.EnableModelFallback = settings[SettingKeyEnableModelFallback] == "true"
	result.FallbackModelAnthropic = s.getStringOrDefault(settings, SettingKeyFallbackModelAnthropic, "claude-3-5-sonnet-20241022")
	result.FallbackModelOpenAI = s.getStringOrDefault(settings, SettingKeyFallbackModelOpenAI, "gpt-4o")
	result.FallbackModelGemini = s.getStringOrDefault(settings, SettingKeyFallbackModelGemini, "gemini-2.5-pro")
	result.FallbackModelAntigravity = s.getStringOrDefault(settings, SettingKeyFallbackModelAntigravity, "gemini-2.5-pro")

	// Identity patch settings (default: enabled, to preserve existing behavior)
	if v, ok := settings[SettingKeyEnableIdentityPatch]; ok && v != "" {
		result.EnableIdentityPatch = v == "true"
	} else {
		result.EnableIdentityPatch = true
	}
	result.IdentityPatchPrompt = settings[SettingKeyIdentityPatchPrompt]

	// Ops monitoring settings (default: enabled, fail-open)
	result.OpsMonitoringEnabled = !isFalseSettingValue(settings[SettingKeyOpsMonitoringEnabled])
	result.OpsRealtimeMonitoringEnabled = !isFalseSettingValue(settings[SettingKeyOpsRealtimeMonitoringEnabled])
	result.OpsQueryModeDefault = string(ParseOpsQueryMode(settings[SettingKeyOpsQueryModeDefault]))
	result.OpsMetricsIntervalSeconds = 60
	if raw := strings.TrimSpace(settings[SettingKeyOpsMetricsIntervalSeconds]); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			if v < 60 {
				v = 60
			}
			if v > 3600 {
				v = 3600
			}
			result.OpsMetricsIntervalSeconds = v
		}
	}

	// Channel monitor feature (default: enabled, 60s)
	result.ChannelMonitorEnabled = !isFalseSettingValue(settings[SettingKeyChannelMonitorEnabled])
	result.ChannelMonitorDefaultIntervalSeconds = parseChannelMonitorInterval(
		settings[SettingKeyChannelMonitorDefaultIntervalSeconds],
	)
	result.ChannelMonitorLatencyUnit = normalizeChannelMonitorLatencyUnit(settings[SettingKeyChannelMonitorLatencyUnit])
	result.ChannelMonitorPublicShareEnabled = settings[SettingKeyChannelMonitorPublicShareEnabled] == "true"
	result.ChannelMonitorPublicShareRequireAuth = settings[SettingKeyChannelMonitorPublicShareRequireAuth] == "true"

	// Available channels feature (default: disabled; strict true)
	result.AvailableChannelsEnabled = settings[SettingKeyAvailableChannelsEnabled] == "true"

	// Support chat feature (default: disabled; explicit true enables)
	result.SupportChatEnabled = settings[SettingKeySupportChatEnabled] == "true"
	result.SupportChatRetentionDays = parseSupportChatRetentionDays(
		settings[SettingKeySupportChatRetentionDays],
	)

	// Model plaza feature (default: disabled and anonymously visible when enabled)
	result.ModelPlazaEnabled = settings[SettingKeyModelPlazaEnabled] == "true"
	result.ModelPlazaRequireAuth = settings[SettingKeyModelPlazaRequireAuth] == "true"
	result.ModelPlazaAutoPublicModels = settings[SettingKeyModelPlazaAutoPublicModels] == "true"
	result.ModelPlazaDescription = settings[SettingKeyModelPlazaDescription]

	// Media Studio feature (default: disabled; strict true)
	result.MediaStudioEnabled = settings[SettingKeyMediaStudioEnabled] == "true"

	// Custom model configuration feature (default: disabled; strict true)
	result.CustomModelConfigEnabled = settings[SettingKeyCustomModelConfigEnabled] == "true"

	// IPv6 egress is an administrator-controlled runtime switch. Databases
	// created before this setting existed have no row, so retain the legacy
	// deployment value until the administrator saves an explicit choice.
	ipv6EgressRaw, configured := settings[SettingKeyIPv6EgressUIEnabled]
	if configured {
		result.IPv6EgressUIEnabled = ipv6EgressRaw == "true"
	} else if s != nil && s.cfg != nil {
		result.IPv6EgressUIEnabled = s.cfg.IPv6Egress.IsEnabled()
	}
	if s != nil {
		s.syncIPv6EgressRuntime(result.IPv6EgressUIEnabled, false)
	}

	// Affiliate (邀请返利) feature (default: disabled; strict true)
	result.AffiliateEnabled = settings[SettingKeyAffiliateEnabled] == "true"

	// 风控中心功能（默认关闭，严格 true 才启用）
	result.RiskControlEnabled = settings[SettingKeyRiskControlEnabled] == "true"

	// cyber 会话屏蔽（默认关闭，TTL 默认 3600s）
	result.CyberSessionBlockEnabled = settings[SettingKeyCyberSessionBlockEnabled] == "true"
	if v, err := strconv.Atoi(strings.TrimSpace(settings[SettingKeyCyberSessionBlockTTLSeconds])); err == nil && v > 0 {
		result.CyberSessionBlockTTLSeconds = v
	} else {
		result.CyberSessionBlockTTLSeconds = 3600
	}

	// Claude Code version check
	result.MinClaudeCodeVersion = settings[SettingKeyMinClaudeCodeVersion]
	result.MaxClaudeCodeVersion = settings[SettingKeyMaxClaudeCodeVersion]

	// 分组隔离
	result.AllowUngroupedKeyScheduling = settings[SettingKeyAllowUngroupedKeyScheduling] == "true"
	result.SchedulerV2Enabled = settings[SettingKeySchedulerV2Enabled] == "true"
	result.SchedulerV2CandidateLimit, result.SchedulerV2ScanLimit = parseSchedulerV2Limits(
		settings[SettingKeySchedulerV2CandidateLimit],
		settings[SettingKeySchedulerV2ScanLimit],
	)
	requestPrioritySettings := parseRequestPriorityAdmissionSettings(settings)
	result.RequestPriorityAdmissionEnabled = requestPrioritySettings.Enabled
	result.RequestPriorityPendingLimitPerInstance = requestPrioritySettings.PendingLimitPerInstance
	result.RequestPriorityPendingMiBPerInstance = requestPrioritySettings.PendingMiBPerInstance
	if result.SchedulerV2Enabled {
		result.SchedulerV2Status = SchedulerEngineStatusBuilding
	} else {
		result.SchedulerV2Status = SchedulerEngineStatusDisabled
	}
}

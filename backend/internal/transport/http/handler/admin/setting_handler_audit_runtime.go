package admin

import "github.com/Wei-Shaw/sub2api/internal/application/service"

func appendRuntimeSettingChanges(changed []string, before, after *service.SystemSettings) []string {
	if before.OpsMonitoringEnabled != after.OpsMonitoringEnabled {
		changed = append(changed, "ops_monitoring_enabled")
	}
	if before.OpsRealtimeMonitoringEnabled != after.OpsRealtimeMonitoringEnabled {
		changed = append(changed, "ops_realtime_monitoring_enabled")
	}
	if before.OpsQueryModeDefault != after.OpsQueryModeDefault {
		changed = append(changed, "ops_query_mode_default")
	}
	if before.OpsMetricsIntervalSeconds != after.OpsMetricsIntervalSeconds {
		changed = append(changed, "ops_metrics_interval_seconds")
	}
	if before.MinClaudeCodeVersion != after.MinClaudeCodeVersion {
		changed = append(changed, "min_claude_code_version")
	}
	if before.MaxClaudeCodeVersion != after.MaxClaudeCodeVersion {
		changed = append(changed, "max_claude_code_version")
	}
	if before.MinCodexVersion != after.MinCodexVersion {
		changed = append(changed, "min_codex_version")
	}
	if before.MaxCodexVersion != after.MaxCodexVersion {
		changed = append(changed, "max_codex_version")
	}
	if before.CodexCLIOnlyAllowAppServerClients != after.CodexCLIOnlyAllowAppServerClients {
		changed = append(changed, "codex_cli_only_allow_app_server_clients")
	}
	if before.CodexCLIOnlyEngineFingerprintSignals != after.CodexCLIOnlyEngineFingerprintSignals {
		changed = append(changed, "codex_cli_only_engine_fingerprint_signals")
	}
	if before.CodexCLIOnlyBlacklist != after.CodexCLIOnlyBlacklist {
		changed = append(changed, "codex_cli_only_blacklist")
	}
	if before.CodexCLIOnlyWhitelist != after.CodexCLIOnlyWhitelist {
		changed = append(changed, "codex_cli_only_whitelist")
	}
	if before.AllowUngroupedKeyScheduling != after.AllowUngroupedKeyScheduling {
		changed = append(changed, "allow_ungrouped_key_scheduling")
	}
	if before.SchedulerV2Enabled != after.SchedulerV2Enabled {
		changed = append(changed, "scheduler_v2_enabled")
	}
	if before.SchedulerV2CandidateLimit != after.SchedulerV2CandidateLimit {
		changed = append(changed, "scheduler_v2_candidate_limit")
	}
	if before.SchedulerV2ScanLimit != after.SchedulerV2ScanLimit {
		changed = append(changed, "scheduler_v2_scan_limit")
	}
	if before.RequestPriorityAdmissionEnabled != after.RequestPriorityAdmissionEnabled {
		changed = append(changed, "request_priority_admission_enabled")
	}
	if before.RequestPriorityPendingLimitPerInstance != after.RequestPriorityPendingLimitPerInstance {
		changed = append(changed, "request_priority_pending_limit_per_instance")
	}
	if before.RequestPriorityPendingMiBPerInstance != after.RequestPriorityPendingMiBPerInstance {
		changed = append(changed, "request_priority_pending_mib_per_instance")
	}
	if before.BackendModeEnabled != after.BackendModeEnabled {
		changed = append(changed, "backend_mode_enabled")
	}
	if before.StreamModePerformanceEnabled != after.StreamModePerformanceEnabled {
		changed = append(changed, "stream_mode_performance_enabled")
	}
	if before.OpenAIWSModeRouterV2Enabled != after.OpenAIWSModeRouterV2Enabled {
		changed = append(changed, "openai_ws_mode_router_v2_enabled")
	}
	if before.OpenAIVisibleOutputTTFTEnabled != after.OpenAIVisibleOutputTTFTEnabled {
		changed = append(changed, "openai_visible_output_ttft_enabled")
	}
	if before.PurchaseSubscriptionEnabled != after.PurchaseSubscriptionEnabled {
		changed = append(changed, "purchase_subscription_enabled")
	}
	if before.PurchaseSubscriptionURL != after.PurchaseSubscriptionURL {
		changed = append(changed, "purchase_subscription_url")
	}
	if before.TableDefaultPageSize != after.TableDefaultPageSize {
		changed = append(changed, "table_default_page_size")
	}
	if !equalIntSlice(before.TablePageSizeOptions, after.TablePageSizeOptions) {
		changed = append(changed, "table_page_size_options")
	}
	if before.CustomMenuItems != after.CustomMenuItems {
		changed = append(changed, "custom_menu_items")
	}
	if before.CustomEndpoints != after.CustomEndpoints {
		changed = append(changed, "custom_endpoints")
	}
	if before.EnableFingerprintUnification != after.EnableFingerprintUnification {
		changed = append(changed, "enable_fingerprint_unification")
	}
	if before.EnableMetadataPassthrough != after.EnableMetadataPassthrough {
		changed = append(changed, "enable_metadata_passthrough")
	}
	if before.EnableCCHSigning != after.EnableCCHSigning {
		changed = append(changed, "enable_cch_signing")
	}
	if before.EnableClaudeOAuthSystemPromptInjection != after.EnableClaudeOAuthSystemPromptInjection {
		changed = append(changed, "enable_claude_oauth_system_prompt_injection")
	}
	if before.ClaudeOAuthSystemPrompt != after.ClaudeOAuthSystemPrompt {
		changed = append(changed, "claude_oauth_system_prompt")
	}
	if before.ClaudeOAuthSystemPromptBlocks != after.ClaudeOAuthSystemPromptBlocks {
		changed = append(changed, "claude_oauth_system_prompt_blocks")
	}
	if before.EnableAnthropicCacheTTL1hInjection != after.EnableAnthropicCacheTTL1hInjection {
		changed = append(changed, "enable_anthropic_cache_ttl_1h_injection")
	}
	if before.RewriteMessageCacheControl != after.RewriteMessageCacheControl {
		changed = append(changed, "rewrite_message_cache_control")
	}
	if before.EnableClientDatelineNormalization != after.EnableClientDatelineNormalization {
		changed = append(changed, "enable_client_dateline_normalization")
	}
	if before.AntigravityUserAgentVersion != after.AntigravityUserAgentVersion {
		changed = append(changed, "antigravity_user_agent_version")
	}
	if before.OpenAICodexUserAgent != after.OpenAICodexUserAgent {
		changed = append(changed, "openai_codex_user_agent")
	}
	if before.OpenAICodexClientVersion != after.OpenAICodexClientVersion {
		changed = append(changed, "openai_codex_client_version")
	}
	if before.OpenAICodexVersionAutoSyncEnabled != after.OpenAICodexVersionAutoSyncEnabled {
		changed = append(changed, "openai_codex_version_auto_sync_enabled")
	}
	return changed
}

func appendOpenAISchedulerSettingChanges(changed []string, before, after *service.SystemSettings) []string {
	if before.OpenAILowUpstreamRatePriorityEnabled != after.OpenAILowUpstreamRatePriorityEnabled {
		changed = append(changed, "openai_low_upstream_rate_priority_enabled")
	}
	if before.OpenAIOAuthSchedulingRateMultiplier != after.OpenAIOAuthSchedulingRateMultiplier {
		changed = append(changed, "openai_oauth_scheduling_rate_multiplier")
	}
	if before.OpenAIAdvancedSchedulerEnabled != after.OpenAIAdvancedSchedulerEnabled {
		changed = append(changed, "openai_advanced_scheduler_enabled")
	}
	if before.OpenAIAdvancedSchedulerStickyWeightedEnabled != after.OpenAIAdvancedSchedulerStickyWeightedEnabled {
		changed = append(changed, "openai_advanced_scheduler_sticky_weighted_enabled")
	}
	if before.OpenAIContentSessionBurstBalanceEnabled != after.OpenAIContentSessionBurstBalanceEnabled {
		changed = append(changed, "openai_content_session_burst_balance_enabled")
	}
	if before.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled != after.OpenAIAdvancedSchedulerSubscriptionPriorityEnabled {
		changed = append(changed, "openai_advanced_scheduler_subscription_priority_enabled")
	}
	if before.OpenAIAdvancedSchedulerLBTopK != after.OpenAIAdvancedSchedulerLBTopK {
		changed = append(changed, "openai_advanced_scheduler_lb_top_k")
	}
	if before.OpenAIAdvancedSchedulerWeightPriority != after.OpenAIAdvancedSchedulerWeightPriority {
		changed = append(changed, "openai_advanced_scheduler_weight_priority")
	}
	if before.OpenAIAdvancedSchedulerWeightLoad != after.OpenAIAdvancedSchedulerWeightLoad {
		changed = append(changed, "openai_advanced_scheduler_weight_load")
	}
	if before.OpenAIAdvancedSchedulerWeightQueue != after.OpenAIAdvancedSchedulerWeightQueue {
		changed = append(changed, "openai_advanced_scheduler_weight_queue")
	}
	if before.OpenAIAdvancedSchedulerWeightErrorRate != after.OpenAIAdvancedSchedulerWeightErrorRate {
		changed = append(changed, "openai_advanced_scheduler_weight_error_rate")
	}
	if before.OpenAIAdvancedSchedulerWeightTTFT != after.OpenAIAdvancedSchedulerWeightTTFT {
		changed = append(changed, "openai_advanced_scheduler_weight_ttft")
	}
	if before.OpenAIAdvancedSchedulerWeightReset != after.OpenAIAdvancedSchedulerWeightReset {
		changed = append(changed, "openai_advanced_scheduler_weight_reset")
	}
	if before.OpenAIAdvancedSchedulerWeightQuotaHeadroom != after.OpenAIAdvancedSchedulerWeightQuotaHeadroom {
		changed = append(changed, "openai_advanced_scheduler_weight_quota_headroom")
	}
	if before.OpenAIAdvancedSchedulerWeightUpstreamCost != after.OpenAIAdvancedSchedulerWeightUpstreamCost {
		changed = append(changed, "openai_advanced_scheduler_weight_upstream_cost")
	}
	if before.OpenAIAdvancedSchedulerWeightPreviousResponse != after.OpenAIAdvancedSchedulerWeightPreviousResponse {
		changed = append(changed, "openai_advanced_scheduler_weight_previous_response")
	}
	if before.OpenAIAdvancedSchedulerWeightSessionSticky != after.OpenAIAdvancedSchedulerWeightSessionSticky {
		changed = append(changed, "openai_advanced_scheduler_weight_session_sticky")
	}
	return changed
}

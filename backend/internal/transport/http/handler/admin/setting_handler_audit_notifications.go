package admin

import "github.com/Wei-Shaw/sub2api/internal/application/service"

func appendNotificationAndRiskSettingChanges(changed []string, before, after *service.SystemSettings) []string {
	if before.BalanceLowNotifyEnabled != after.BalanceLowNotifyEnabled {
		changed = append(changed, "balance_low_notify_enabled")
	}
	if before.BalanceLowNotifyThreshold != after.BalanceLowNotifyThreshold {
		changed = append(changed, "balance_low_notify_threshold")
	}
	if before.BalanceLowNotifyRechargeURL != after.BalanceLowNotifyRechargeURL {
		changed = append(changed, "balance_low_notify_recharge_url")
	}
	if before.SubscriptionExpiryNotifyEnabled != after.SubscriptionExpiryNotifyEnabled {
		changed = append(changed, "subscription_expiry_notify_enabled")
	}
	if before.AccountQuotaNotifyEnabled != after.AccountQuotaNotifyEnabled {
		changed = append(changed, "account_quota_notify_enabled")
	}
	if !equalNotifyEmailEntries(before.AccountQuotaNotifyEmails, after.AccountQuotaNotifyEmails) {
		changed = append(changed, "account_quota_notify_emails")
	}
	if before.ChannelMonitorEnabled != after.ChannelMonitorEnabled {
		changed = append(changed, "channel_monitor_enabled")
	}
	if before.ChannelMonitorDefaultIntervalSeconds != after.ChannelMonitorDefaultIntervalSeconds {
		changed = append(changed, "channel_monitor_default_interval_seconds")
	}
	if before.ChannelMonitorLatencyUnit != after.ChannelMonitorLatencyUnit {
		changed = append(changed, "channel_monitor_latency_unit")
	}
	if before.ChannelMonitorPublicShareEnabled != after.ChannelMonitorPublicShareEnabled {
		changed = append(changed, "channel_monitor_public_share_enabled")
	}
	if before.ChannelMonitorPublicShareRequireAuth != after.ChannelMonitorPublicShareRequireAuth {
		changed = append(changed, "channel_monitor_public_share_require_auth")
	}
	if before.AvailableChannelsEnabled != after.AvailableChannelsEnabled {
		changed = append(changed, "available_channels_enabled")
	}
	if before.SupportChatEnabled != after.SupportChatEnabled {
		changed = append(changed, "support_chat_enabled")
	}
	if before.SupportChatRetentionDays != after.SupportChatRetentionDays {
		changed = append(changed, "support_chat_retention_days")
	}
	if before.ModelPlazaEnabled != after.ModelPlazaEnabled {
		changed = append(changed, "model_plaza_enabled")
	}
	if before.ModelPlazaRequireAuth != after.ModelPlazaRequireAuth {
		changed = append(changed, "model_plaza_require_auth")
	}
	if before.ModelPlazaAutoPublicModels != after.ModelPlazaAutoPublicModels {
		changed = append(changed, "model_plaza_auto_public_models")
	}
	if before.ModelPlazaDescription != after.ModelPlazaDescription {
		changed = append(changed, "model_plaza_description")
	}
	if before.MediaStudioEnabled != after.MediaStudioEnabled {
		changed = append(changed, "media_studio_enabled")
	}
	if before.AffiliateEnabled != after.AffiliateEnabled {
		changed = append(changed, "affiliate_enabled")
	}
	if before.RiskControlEnabled != after.RiskControlEnabled {
		changed = append(changed, "risk_control_enabled")
	}
	if before.CyberSessionBlockEnabled != after.CyberSessionBlockEnabled {
		changed = append(changed, "cyber_session_block_enabled")
	}
	if before.CyberSessionBlockTTLSeconds != after.CyberSessionBlockTTLSeconds {
		changed = append(changed, "cyber_session_block_ttl_seconds")
	}
	return changed
}

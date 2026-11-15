package admin

import "github.com/DR-lin-eng/sub2api-no2api/internal/application/service"

func appendProductSettingChanges(changed []string, before, after *service.SystemSettings) []string {
	if before.SiteName != after.SiteName {
		changed = append(changed, "site_name")
	}
	if before.SiteLogo != after.SiteLogo {
		changed = append(changed, "site_logo")
	}
	if before.SiteSubtitle != after.SiteSubtitle {
		changed = append(changed, "site_subtitle")
	}
	if before.APIBaseURL != after.APIBaseURL {
		changed = append(changed, "api_base_url")
	}
	if before.ContactInfo != after.ContactInfo {
		changed = append(changed, "contact_info")
	}
	if before.DocURL != after.DocURL {
		changed = append(changed, "doc_url")
	}
	if before.HomeContent != after.HomeContent {
		changed = append(changed, "home_content")
	}
	if before.CompactHomeEnabled != after.CompactHomeEnabled {
		changed = append(changed, "compact_home_enabled")
	}
	if before.HideCcsImportButton != after.HideCcsImportButton {
		changed = append(changed, "hide_ccs_import_button")
	}
	if before.DefaultConcurrency != after.DefaultConcurrency {
		changed = append(changed, "default_concurrency")
	}
	if before.DefaultBalance != after.DefaultBalance {
		changed = append(changed, "default_balance")
	}
	if before.AffiliateRebateRate != after.AffiliateRebateRate {
		changed = append(changed, "affiliate_rebate_rate")
	}
	if before.AffiliateRebateFreezeHours != after.AffiliateRebateFreezeHours {
		changed = append(changed, "affiliate_rebate_freeze_hours")
	}
	if before.AffiliateRebateDurationDays != after.AffiliateRebateDurationDays {
		changed = append(changed, "affiliate_rebate_duration_days")
	}
	if before.AffiliateRebatePerInviteeCap != after.AffiliateRebatePerInviteeCap {
		changed = append(changed, "affiliate_rebate_per_invitee_cap")
	}
	if before.AdminRechargeRebateEnabled != after.AdminRechargeRebateEnabled {
		changed = append(changed, "affiliate_admin_recharge_enabled")
	}
	if !equalDefaultSubscriptions(before.DefaultSubscriptions, after.DefaultSubscriptions) {
		changed = append(changed, "default_subscriptions")
	}
	if before.EnableModelFallback != after.EnableModelFallback {
		changed = append(changed, "enable_model_fallback")
	}
	if before.FallbackModelAnthropic != after.FallbackModelAnthropic {
		changed = append(changed, "fallback_model_anthropic")
	}
	if before.FallbackModelOpenAI != after.FallbackModelOpenAI {
		changed = append(changed, "fallback_model_openai")
	}
	if before.FallbackModelGemini != after.FallbackModelGemini {
		changed = append(changed, "fallback_model_gemini")
	}
	if before.FallbackModelAntigravity != after.FallbackModelAntigravity {
		changed = append(changed, "fallback_model_antigravity")
	}
	if before.EnableIdentityPatch != after.EnableIdentityPatch {
		changed = append(changed, "enable_identity_patch")
	}
	if before.IdentityPatchPrompt != after.IdentityPatchPrompt {
		changed = append(changed, "identity_patch_prompt")
	}
	return changed
}

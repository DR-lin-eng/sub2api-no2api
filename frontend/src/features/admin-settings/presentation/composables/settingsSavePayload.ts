import {
  appendAuthSourceDefaultsToUpdateRequest,
  defaultWeChatConnectScopesForMode,
  sanitizePlatformQuotasMap,
  type AuthSourceDefaultsState,
  type DefaultSubscriptionSetting,
  type UpdateSettingsRequest,
  type WeChatConnectMode,
} from "@/features/admin-settings/data/dtos/systemSettingsDtos";
import type { OpenAIFastPolicyRule } from "@/features/admin-settings/data/dtos/adminSettingsDtos";
import type { SettingsForm } from "./settingsForm";

export interface SettingsSavePayloadContext {
  form: SettingsForm;
  normalizedDefaultSubscriptions: DefaultSubscriptionSetting[];
  registrationEmailSuffixWhitelistTags: readonly string[];
  clientIPTrustedProxies: string[];
  wechatStoredMode: WeChatConnectMode;
  claudeOAuthSystemPromptBlocksJSON: string;
  codexFingerprintSignalsJSON: string;
  codexBlacklistJSON: string;
  codexWhitelistJSON: string;
  currentOrigin: string;
  openaiFastPolicyLoaded: boolean;
  openaiFastPolicyRules: readonly OpenAIFastPolicyRule[];
  authSourceDefaults: AuthSourceDefaultsState;
}

function buildGeneralSettingsPayload({
  form,
  normalizedDefaultSubscriptions,
  registrationEmailSuffixWhitelistTags,
}: SettingsSavePayloadContext): UpdateSettingsRequest {
  return {
    registration_enabled: form.registration_enabled,
    email_verify_enabled: form.email_verify_enabled,
    registration_email_suffix_whitelist:
      registrationEmailSuffixWhitelistTags.map((suffix) =>
        suffix.startsWith("*.") ? suffix : `@${suffix}`,
      ),
    promo_code_enabled: form.promo_code_enabled,
    invitation_code_enabled: form.invitation_code_enabled,
    password_reset_enabled: form.password_reset_enabled,
    totp_enabled: form.totp_enabled,
    passkey_enabled: form.passkey_enabled,
    session_binding_enabled: form.session_binding_enabled,
    step_up_enabled: form.step_up_enabled,
    // Empty/invalid numeric input follows the backend's empty-value default.
    audit_log_retention_days: Number.isFinite(form.audit_log_retention_days)
      ? form.audit_log_retention_days
      : 180,
    login_agreement_enabled: form.login_agreement_enabled,
    login_agreement_mode: form.login_agreement_mode,
    login_agreement_updated_at: form.login_agreement_updated_at,
    login_agreement_documents: form.login_agreement_documents,
    default_balance: form.default_balance,
    affiliate_rebate_rate: Math.min(
      100,
      Math.max(0, Number(form.affiliate_rebate_rate) || 0),
    ),
    affiliate_rebate_freeze_hours: Math.max(
      0,
      Math.min(720, Number(form.affiliate_rebate_freeze_hours) || 0),
    ),
    affiliate_rebate_duration_days: Math.max(
      0,
      Math.min(
        3650,
        Math.floor(Number(form.affiliate_rebate_duration_days) || 0),
      ),
    ),
    affiliate_rebate_per_invitee_cap: Math.max(
      0,
      Number(form.affiliate_rebate_per_invitee_cap) || 0,
    ),
    affiliate_admin_recharge_enabled: form.affiliate_admin_recharge_enabled,
    default_concurrency: form.default_concurrency,
    default_subscriptions: normalizedDefaultSubscriptions,
    force_email_on_third_party_signup:
      form.force_email_on_third_party_signup,
    default_user_rpm_limit: form.default_user_rpm_limit,
    site_name: form.site_name,
    site_logo: form.site_logo,
    site_subtitle: form.site_subtitle,
    api_base_url: form.api_base_url,
    contact_info: form.contact_info,
    doc_url: form.doc_url,
	home_content: form.home_content,
	compact_home_enabled: form.compact_home_enabled,
	backend_mode_enabled: form.backend_mode_enabled,
    hide_ccs_import_button: form.hide_ccs_import_button,
    table_default_page_size: form.table_default_page_size,
    table_page_size_options: form.table_page_size_options,
    custom_menu_items: form.custom_menu_items,
    custom_endpoints: form.custom_endpoints,
    frontend_url: form.frontend_url,
  };
}

function buildIdentitySettingsPayload({
  form,
  clientIPTrustedProxies,
  wechatStoredMode,
}: SettingsSavePayloadContext): UpdateSettingsRequest {
  return {
    smtp_host: form.smtp_host,
    smtp_port: form.smtp_port,
    smtp_username: form.smtp_username,
    smtp_password: form.smtp_password || undefined,
    smtp_from_email: form.smtp_from_email,
    smtp_from_name: form.smtp_from_name,
    smtp_use_tls: form.smtp_use_tls,
    turnstile_enabled: form.turnstile_enabled,
    turnstile_site_key: form.turnstile_site_key,
    turnstile_secret_key: form.turnstile_secret_key || undefined,
    recaptcha_enabled: form.recaptcha_enabled,
    recaptcha_site_key: form.recaptcha_site_key,
    recaptcha_secret_key: form.recaptcha_secret_key || undefined,
    cap_enabled: form.cap_enabled,
    cap_api_endpoint: form.cap_api_endpoint,
    cap_secret_key: form.cap_secret_key || undefined,
    tencent_captcha_enabled: form.tencent_captcha_enabled,
    tencent_captcha_app_id: form.tencent_captcha_app_id,
    tencent_captcha_app_secret_key:
      form.tencent_captcha_app_secret_key || undefined,
    tencent_captcha_cloud_secret_id:
      form.tencent_captcha_cloud_secret_id || undefined,
    tencent_captcha_cloud_secret_key:
      form.tencent_captcha_cloud_secret_key || undefined,
    tencent_captcha_region: form.tencent_captcha_region,
    aliyun_captcha_enabled: form.aliyun_captcha_enabled,
    aliyun_captcha_access_key_id: form.aliyun_captcha_access_key_id,
    aliyun_captcha_access_key_secret:
      form.aliyun_captcha_access_key_secret || undefined,
    aliyun_captcha_scene_id: form.aliyun_captcha_scene_id,
    aliyun_captcha_prefix: form.aliyun_captcha_prefix,
    aliyun_captcha_region: form.aliyun_captcha_region,
    local_captcha_enabled: form.local_captcha_enabled,
    client_ip_resolution_mode: form.client_ip_resolution_mode,
    client_ip_trusted_proxies: clientIPTrustedProxies,
    linuxdo_connect_enabled: form.linuxdo_connect_enabled,
    linuxdo_connect_client_id: form.linuxdo_connect_client_id,
    linuxdo_connect_client_secret:
      form.linuxdo_connect_client_secret || undefined,
    linuxdo_connect_redirect_url: form.linuxdo_connect_redirect_url,
    dingtalk_connect_enabled: form.dingtalk_connect_enabled,
    dingtalk_connect_client_id: form.dingtalk_connect_client_id,
    dingtalk_connect_client_secret:
      form.dingtalk_connect_client_secret || undefined,
    dingtalk_connect_redirect_url: form.dingtalk_connect_redirect_url,
    dingtalk_connect_corp_restriction_policy:
      form.dingtalk_connect_corp_restriction_policy,
    dingtalk_connect_internal_corp_id:
      form.dingtalk_connect_internal_corp_id,
    dingtalk_connect_bypass_registration:
      form.dingtalk_connect_bypass_registration,
    dingtalk_connect_sync_corp_email:
      form.dingtalk_connect_sync_corp_email,
    dingtalk_connect_sync_display_name:
      form.dingtalk_connect_sync_display_name,
    dingtalk_connect_sync_dept: form.dingtalk_connect_sync_dept,
    dingtalk_connect_sync_corp_email_attr_key:
      form.dingtalk_connect_sync_corp_email_attr_key,
    dingtalk_connect_sync_display_name_attr_key:
      form.dingtalk_connect_sync_display_name_attr_key,
    dingtalk_connect_sync_dept_attr_key:
      form.dingtalk_connect_sync_dept_attr_key,
    dingtalk_connect_sync_corp_email_attr_name:
      form.dingtalk_connect_sync_corp_email_attr_name,
    dingtalk_connect_sync_display_name_attr_name:
      form.dingtalk_connect_sync_display_name_attr_name,
    dingtalk_connect_sync_dept_attr_name:
      form.dingtalk_connect_sync_dept_attr_name,
    wechat_connect_enabled: form.wechat_connect_enabled,
    wechat_connect_app_id:
      form.wechat_connect_open_app_id ||
      form.wechat_connect_mp_app_id ||
      form.wechat_connect_mobile_app_id ||
      form.wechat_connect_app_id,
    wechat_connect_app_secret: form.wechat_connect_app_secret || undefined,
    wechat_connect_open_app_id: form.wechat_connect_open_app_id,
    wechat_connect_open_app_secret:
      form.wechat_connect_open_app_secret || undefined,
    wechat_connect_mp_app_id: form.wechat_connect_mp_app_id,
    wechat_connect_mp_app_secret:
      form.wechat_connect_mp_app_secret || undefined,
    wechat_connect_mobile_app_id: form.wechat_connect_mobile_app_id,
    wechat_connect_mobile_app_secret:
      form.wechat_connect_mobile_app_secret || undefined,
    wechat_connect_open_enabled: form.wechat_connect_open_enabled,
    wechat_connect_mp_enabled: form.wechat_connect_mp_enabled,
    wechat_connect_mobile_enabled: form.wechat_connect_mobile_enabled,
    wechat_connect_mode: wechatStoredMode,
    wechat_connect_scopes: defaultWeChatConnectScopesForMode(wechatStoredMode),
    wechat_connect_redirect_url: form.wechat_connect_redirect_url,
    wechat_connect_frontend_redirect_url:
      form.wechat_connect_frontend_redirect_url,
    oidc_connect_enabled: form.oidc_connect_enabled,
    oidc_connect_provider_name: form.oidc_connect_provider_name,
    oidc_connect_client_id: form.oidc_connect_client_id,
    oidc_connect_client_secret: form.oidc_connect_client_secret || undefined,
    oidc_connect_issuer_url: form.oidc_connect_issuer_url,
    oidc_connect_discovery_url: form.oidc_connect_discovery_url,
    oidc_connect_authorize_url: form.oidc_connect_authorize_url,
    oidc_connect_token_url: form.oidc_connect_token_url,
    oidc_connect_userinfo_url: form.oidc_connect_userinfo_url,
    oidc_connect_jwks_url: form.oidc_connect_jwks_url,
    oidc_connect_scopes: form.oidc_connect_scopes,
    oidc_connect_redirect_url: form.oidc_connect_redirect_url,
    oidc_connect_frontend_redirect_url:
      form.oidc_connect_frontend_redirect_url,
    oidc_connect_token_auth_method: form.oidc_connect_token_auth_method,
    oidc_connect_use_pkce: form.oidc_connect_use_pkce,
    oidc_connect_validate_id_token: form.oidc_connect_validate_id_token,
    oidc_connect_allowed_signing_algs: form.oidc_connect_allowed_signing_algs,
    oidc_connect_clock_skew_seconds: form.oidc_connect_clock_skew_seconds,
    oidc_connect_require_email_verified:
      form.oidc_connect_require_email_verified,
    oidc_connect_userinfo_email_path: form.oidc_connect_userinfo_email_path,
    oidc_connect_userinfo_id_path: form.oidc_connect_userinfo_id_path,
    oidc_connect_userinfo_username_path:
      form.oidc_connect_userinfo_username_path,
    github_oauth_enabled: form.github_oauth_enabled,
    github_oauth_client_id: form.github_oauth_client_id,
    github_oauth_client_secret: form.github_oauth_client_secret || undefined,
    github_oauth_redirect_url: form.github_oauth_redirect_url,
    github_oauth_frontend_redirect_url:
      form.github_oauth_frontend_redirect_url,
    google_oauth_enabled: form.google_oauth_enabled,
    google_oauth_client_id: form.google_oauth_client_id,
    google_oauth_client_secret: form.google_oauth_client_secret || undefined,
    google_oauth_redirect_url: form.google_oauth_redirect_url,
    google_oauth_frontend_redirect_url:
      form.google_oauth_frontend_redirect_url,
  };
}

function buildGatewaySettingsPayload({
  form,
  claudeOAuthSystemPromptBlocksJSON,
  codexFingerprintSignalsJSON,
  codexBlacklistJSON,
  codexWhitelistJSON,
}: SettingsSavePayloadContext): UpdateSettingsRequest {
  return {
    enable_model_fallback: form.enable_model_fallback,
    fallback_model_anthropic: form.fallback_model_anthropic,
    fallback_model_openai: form.fallback_model_openai,
    fallback_model_gemini: form.fallback_model_gemini,
    fallback_model_antigravity: form.fallback_model_antigravity,
    enable_identity_patch: form.enable_identity_patch,
    identity_patch_prompt: form.identity_patch_prompt,
    min_claude_code_version: form.min_claude_code_version,
    max_claude_code_version: form.max_claude_code_version,
    allow_ungrouped_key_scheduling: form.allow_ungrouped_key_scheduling,
    stream_mode_performance_enabled: form.stream_mode_performance_enabled,
    openai_ws_mode_router_v2_enabled:
      form.openai_ws_mode_router_v2_enabled,
    openai_visible_output_ttft_enabled:
      form.openai_visible_output_ttft_enabled,
    scheduler_v2_enabled: form.scheduler_v2_enabled,
    scheduler_v2_candidate_limit: Number(form.scheduler_v2_candidate_limit),
    scheduler_v2_scan_limit: Number(form.scheduler_v2_scan_limit),
    request_priority_admission_enabled:
      form.request_priority_admission_enabled,
    request_priority_pending_limit_per_instance: Number(
      form.request_priority_pending_limit_per_instance,
    ),
    request_priority_pending_mib_per_instance: Number(
      form.request_priority_pending_mib_per_instance,
    ),
    enable_fingerprint_unification: form.enable_fingerprint_unification,
    enable_metadata_passthrough: form.enable_metadata_passthrough,
    enable_cch_signing: form.enable_cch_signing,
    enable_claude_oauth_system_prompt_injection:
      form.enable_claude_oauth_system_prompt_injection,
    claude_oauth_system_prompt: form.claude_oauth_system_prompt?.trim()
      ? form.claude_oauth_system_prompt
      : "",
    claude_oauth_system_prompt_blocks: claudeOAuthSystemPromptBlocksJSON,
    enable_anthropic_cache_ttl_1h_injection:
      form.enable_anthropic_cache_ttl_1h_injection,
    rewrite_message_cache_control: form.rewrite_message_cache_control,
    enable_client_dateline_normalization:
      form.enable_client_dateline_normalization,
    antigravity_user_agent_version:
      form.antigravity_user_agent_version?.trim() || "",
    openai_codex_user_agent: form.openai_codex_user_agent?.trim() || "",
    openai_codex_client_version:
      form.openai_codex_client_version?.trim() || "",
    openai_codex_version_auto_sync_enabled:
      form.openai_codex_version_auto_sync_enabled,
    min_codex_version: form.min_codex_version?.trim() || "",
    max_codex_version: form.max_codex_version?.trim() || "",
    codex_cli_only_allow_app_server_clients:
      form.codex_cli_only_allow_app_server_clients,
    codex_cli_only_engine_fingerprint_signals: codexFingerprintSignalsJSON,
    codex_cli_only_blacklist: codexBlacklistJSON,
    codex_cli_only_whitelist: codexWhitelistJSON,
  };
}

function buildPaymentSettingsPayload({
  form,
}: SettingsSavePayloadContext): UpdateSettingsRequest {
  return {
    payment_enabled: form.payment_enabled,
    risk_control_enabled: form.risk_control_enabled,
    cyber_session_block_enabled: form.cyber_session_block_enabled,
    cyber_session_block_ttl_seconds:
      Number(form.cyber_session_block_ttl_seconds) || 3600,
    payment_min_amount: Number(form.payment_min_amount) || 0,
    payment_max_amount: Number(form.payment_max_amount) || 0,
    payment_daily_limit: Number(form.payment_daily_limit) || 0,
    payment_max_pending_orders: Number(form.payment_max_pending_orders) || 0,
    payment_order_timeout_minutes:
      Number(form.payment_order_timeout_minutes) || 0,
    payment_balance_disabled: form.payment_balance_disabled,
    payment_balance_recharge_multiplier:
      Number(form.payment_balance_recharge_multiplier) || 1,
    payment_subscription_usd_to_cny_rate:
      Number(form.payment_subscription_usd_to_cny_rate) || 0,
    payment_recharge_fee_rate: Number(form.payment_recharge_fee_rate) || 0,
    payment_enabled_types: form.payment_enabled_types,
    payment_load_balance_strategy: form.payment_load_balance_strategy,
    payment_product_name_prefix: form.payment_product_name_prefix,
    payment_product_name_suffix: form.payment_product_name_suffix,
    payment_help_image_url: form.payment_help_image_url,
    payment_help_text: form.payment_help_text,
    payment_cancel_rate_limit_enabled:
      form.payment_cancel_rate_limit_enabled,
    payment_cancel_rate_limit_max:
      Number(form.payment_cancel_rate_limit_max) || 10,
    payment_cancel_rate_limit_window:
      Number(form.payment_cancel_rate_limit_window) || 1,
    payment_cancel_rate_limit_unit: form.payment_cancel_rate_limit_unit,
    payment_cancel_rate_limit_window_mode:
      form.payment_cancel_rate_limit_window_mode,
    payment_alipay_force_qrcode: form.payment_alipay_force_qrcode,
    payment_alipay_mobile_precreate_deep_link:
      form.payment_alipay_mobile_precreate_deep_link,
  };
}

function buildOpenAISchedulingSettingsPayload({
  form,
}: SettingsSavePayloadContext): UpdateSettingsRequest {
  return {
    openai_low_upstream_rate_priority_enabled:
      form.openai_low_upstream_rate_priority_enabled,
    openai_oauth_scheduling_rate_multiplier:
      form.openai_oauth_scheduling_rate_multiplier,
    openai_content_session_burst_balance_enabled:
      form.openai_content_session_burst_balance_enabled,
    openai_advanced_scheduler_enabled:
      form.openai_advanced_scheduler_enabled,
    openai_advanced_scheduler_sticky_weighted_enabled:
      form.openai_advanced_scheduler_sticky_weighted_enabled,
    openai_advanced_scheduler_subscription_priority_enabled:
      form.openai_advanced_scheduler_subscription_priority_enabled,
    openai_advanced_scheduler_lb_top_k:
      form.openai_advanced_scheduler_lb_top_k.trim(),
    openai_advanced_scheduler_weight_priority:
      form.openai_advanced_scheduler_weight_priority.trim(),
    openai_advanced_scheduler_weight_load:
      form.openai_advanced_scheduler_weight_load.trim(),
    openai_advanced_scheduler_weight_queue:
      form.openai_advanced_scheduler_weight_queue.trim(),
    openai_advanced_scheduler_weight_error_rate:
      form.openai_advanced_scheduler_weight_error_rate.trim(),
    openai_advanced_scheduler_weight_ttft:
      form.openai_advanced_scheduler_weight_ttft.trim(),
    openai_advanced_scheduler_weight_reset:
      form.openai_advanced_scheduler_weight_reset.trim(),
    openai_advanced_scheduler_weight_quota_headroom:
      form.openai_advanced_scheduler_weight_quota_headroom.trim(),
    openai_advanced_scheduler_weight_upstream_cost:
      form.openai_advanced_scheduler_weight_upstream_cost.trim(),
    openai_advanced_scheduler_weight_previous_response:
      form.openai_advanced_scheduler_weight_previous_response.trim(),
    openai_advanced_scheduler_weight_session_sticky:
      form.openai_advanced_scheduler_weight_session_sticky.trim(),
  };
}

function buildNotificationAndFeatureSettingsPayload({
  form,
  currentOrigin,
}: SettingsSavePayloadContext): UpdateSettingsRequest {
  return {
    balance_low_notify_enabled: form.balance_low_notify_enabled,
    balance_low_notify_threshold:
      Number(form.balance_low_notify_threshold) || 0,
    balance_low_notify_recharge_url: (form.balance_low_notify_recharge_url =
      form.balance_low_notify_recharge_url || currentOrigin),
    subscription_expiry_notify_enabled:
      form.subscription_expiry_notify_enabled,
    account_quota_notify_enabled: form.account_quota_notify_enabled,
    account_quota_notify_emails: (form.account_quota_notify_emails || []).filter(
      (entry) => entry.email.trim() !== "",
    ),
    channel_monitor_enabled: form.channel_monitor_enabled,
    channel_monitor_default_interval_seconds:
      Number(form.channel_monitor_default_interval_seconds) || 60,
    channel_monitor_latency_unit:
      form.channel_monitor_latency_unit === "s" ? "s" : "ms",
    channel_monitor_public_share_enabled:
      form.channel_monitor_public_share_enabled,
    channel_monitor_public_share_require_auth:
      form.channel_monitor_public_share_require_auth,
    available_channels_enabled: form.available_channels_enabled,
    support_chat_enabled: form.support_chat_enabled,
    support_chat_retention_days: Math.min(
      3650,
      Math.max(0, Math.trunc(Number(form.support_chat_retention_days) || 0)),
    ),
    model_plaza_enabled: form.model_plaza_enabled,
    model_plaza_require_auth: form.model_plaza_require_auth,
    model_plaza_auto_public_models: form.model_plaza_auto_public_models,
    model_plaza_description: form.model_plaza_description,
    media_studio_enabled: form.media_studio_enabled,
    custom_model_config_enabled: form.custom_model_config_enabled,
    ipv6_egress_ui_enabled: form.ipv6_egress_ui_enabled,
    affiliate_enabled: form.affiliate_enabled,
    allow_user_view_error_requests: form.allow_user_view_error_requests,
    allow_user_view_usage_details: form.allow_user_view_usage_details,
  };
}

function appendOpenAIFastPolicy(
  payload: UpdateSettingsRequest,
  rules: readonly OpenAIFastPolicyRule[],
): void {
  payload.openai_fast_policy_settings = {
    rules: rules.map((rule) => {
      const whitelist = (rule.model_whitelist || [])
        .map((pattern) => pattern.trim())
        .filter((pattern) => pattern !== "");
      const hasWhitelist = whitelist.length > 0;
      return {
        service_tier: rule.service_tier,
        action: rule.action,
        scope: rule.scope,
        user_ids:
          rule.user_ids && rule.user_ids.length > 0
            ? [...rule.user_ids]
            : undefined,
        error_message: rule.action === "block" ? rule.error_message : undefined,
        model_whitelist: hasWhitelist ? whitelist : undefined,
        fallback_action: hasWhitelist
          ? rule.fallback_action || "pass"
          : undefined,
        fallback_error_message:
          hasWhitelist && rule.fallback_action === "block"
            ? rule.fallback_error_message
            : undefined,
      };
    }),
  };
}

export function buildSettingsSavePayload(
  context: SettingsSavePayloadContext,
): UpdateSettingsRequest {
  const payload: UpdateSettingsRequest = {
    ...buildGeneralSettingsPayload(context),
    ...buildIdentitySettingsPayload(context),
    ...buildGatewaySettingsPayload(context),
    ...buildPaymentSettingsPayload(context),
    ...buildOpenAISchedulingSettingsPayload(context),
    ...buildNotificationAndFeatureSettingsPayload(context),
  };

  if (context.openaiFastPolicyLoaded) {
    appendOpenAIFastPolicy(payload, context.openaiFastPolicyRules);
  }

  payload.default_platform_quotas = sanitizePlatformQuotasMap(
    context.form.default_platform_quotas,
  );
  appendAuthSourceDefaultsToUpdateRequest(payload, context.authSourceDefaults);
  return payload;
}

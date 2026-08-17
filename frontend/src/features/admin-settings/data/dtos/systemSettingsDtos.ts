/** Main system settings DTOs and compatibility normalization rules. */

import type { OpenAIFastPolicySettings } from "@/features/admin-settings/data/dtos/adminSettingsDtos";
import type {
  CustomEndpoint,
  CustomMenuItem,
  LoginAgreementDocument,
  NotifyEmailEntry,
} from "@/types";

export type ClientIPResolutionMode =
  | "auto_compat"
  | "trusted_proxy"
  | "direct";

export interface ClientIPResolutionStatus {
  mode: ClientIPResolutionMode;
  custom_prefix_count: number;
  static_prefix_count: number;
  cloudflare_prefix_count: number;
  cloudflare_ranges_source: "embedded" | "refreshed";
  cloudflare_last_success_at: string | null;
}

export interface DefaultSubscriptionSetting {
  group_id: number;
  validity_days: number;
}

// ── 平台限额类型 ──────────────────────────────────────────────────
export type PlatformType = "anthropic" | "openai" | "gemini" | "antigravity" | "grok"
export type QuotaWindowType = "daily" | "weekly" | "monthly"

/** 单平台三档限额；null = 不限制，undefined = 未填（等价 null） */
export interface PlatformQuotaLimits {
  daily:   number | null
  weekly:  number | null
  monthly: number | null
}

/** 全平台默认限额 map（key = PlatformType） */
export type DefaultPlatformQuotasMap = Partial<Record<PlatformType, PlatformQuotaLimits>>

const PLATFORMS: PlatformType[] = ["anthropic", "openai", "gemini", "antigravity", "grok"]

/** 归一化为全 4 平台 × 3 窗口（缺失填 null），供模板非空绑定 */
export function normalizePlatformQuotasMap(input?: DefaultPlatformQuotasMap | null): DefaultPlatformQuotasMap {
  const result: DefaultPlatformQuotasMap = {}
  for (const p of PLATFORMS) {
    const src = input?.[p]
    result[p] = {
      daily:   typeof src?.daily === "number" ? src.daily : null,
      weekly:  typeof src?.weekly === "number" ? src.weekly : null,
      monthly: typeof src?.monthly === "number" ? src.monthly : null,
    }
  }
  return result
}

/** 提交前清洗：非有限数/负数/空字符串 → null（保留 0 = 显式禁用），返回全 4 平台嵌套 map */
export function sanitizePlatformQuotasMap(input?: DefaultPlatformQuotasMap | null): DefaultPlatformQuotasMap {
  const clean = (v: unknown): number | null => (typeof v === "number" && Number.isFinite(v) && v >= 0 ? v : null)
  const result: DefaultPlatformQuotasMap = {}
  for (const p of PLATFORMS) {
    const src = input?.[p]
    result[p] = { daily: clean(src?.daily), weekly: clean(src?.weekly), monthly: clean(src?.monthly) }
  }
  return result
}

export type AuthSourceType =
  | "email"
  | "linuxdo"
  | "oidc"
  | "wechat"
  | "github"
  | "google"
  | "dingtalk";

export interface AuthSourceDefaultsValue {
  balance: number;
  concurrency: number;
  subscriptions: DefaultSubscriptionSetting[];
  grant_on_signup: boolean;
  grant_on_first_bind: boolean;
  // ★ 新增：平台限额覆盖（key = PlatformType）
  platform_quotas: DefaultPlatformQuotasMap;
}

export type AuthSourceDefaultsState = Record<
  AuthSourceType,
  AuthSourceDefaultsValue
>;
export type PaymentVisibleMethod = "alipay" | "wxpay";
export type PaymentVisibleMethodSource =
  | ""
  | "official_alipay"
  | "easypay_alipay"
  | "official_wxpay"
  | "easypay_wxpay";
export type WeChatConnectMode = "open" | "mp" | "mobile";

export interface PaymentVisibleMethodSourceOption {
  value: PaymentVisibleMethodSource;
  labelZh: string;
  labelEn: string;
}

export interface WeChatConnectModeOption {
  value: WeChatConnectMode;
  labelZh: string;
  labelEn: string;
}

const AUTH_SOURCE_TYPES: AuthSourceType[] = [
  "email",
  "linuxdo",
  "oidc",
  "wechat",
  "github",
  "google",
  "dingtalk",
];
const AUTH_SOURCE_DEFAULT_BALANCE = 0;
const AUTH_SOURCE_DEFAULT_CONCURRENCY = 5;
const PAYMENT_VISIBLE_METHOD_SOURCE_OPTIONS: Record<
  PaymentVisibleMethod,
  PaymentVisibleMethodSourceOption[]
> = {
  alipay: [
    { value: "", labelZh: "未配置", labelEn: "Not configured" },
    {
      value: "official_alipay",
      labelZh: "支付宝官方",
      labelEn: "Official Alipay",
    },
    {
      value: "easypay_alipay",
      labelZh: "易支付支付宝",
      labelEn: "EasyPay Alipay",
    },
  ],
  wxpay: [
    { value: "", labelZh: "未配置", labelEn: "Not configured" },
    {
      value: "official_wxpay",
      labelZh: "微信官方",
      labelEn: "Official WeChat Pay",
    },
    {
      value: "easypay_wxpay",
      labelZh: "易支付微信",
      labelEn: "EasyPay WeChat Pay",
    },
  ],
};
const PAYMENT_VISIBLE_METHOD_SOURCE_ALIASES: Record<
  PaymentVisibleMethod,
  Record<string, PaymentVisibleMethodSource>
> = {
  alipay: {
    official_alipay: "official_alipay",
    alipay: "official_alipay",
    alipay_direct: "official_alipay",
    official: "official_alipay",
    easypay_alipay: "easypay_alipay",
    easypay: "easypay_alipay",
  },
  wxpay: {
    official_wxpay: "official_wxpay",
    wxpay: "official_wxpay",
    wxpay_direct: "official_wxpay",
    wechat: "official_wxpay",
    official: "official_wxpay",
    easypay_wxpay: "easypay_wxpay",
    easypay: "easypay_wxpay",
  },
};
const WECHAT_CONNECT_MODE_OPTIONS: WeChatConnectModeOption[] = [
  { value: "open", labelZh: "PC 应用", labelEn: "PC App" },
  {
    value: "mp",
    labelZh: "公众号",
    labelEn: "Official Account",
  },
  {
    value: "mobile",
    labelZh: "移动应用",
    labelEn: "Mobile App",
  },
];
const WECHAT_CONNECT_MODE_ALIASES: Record<string, WeChatConnectMode> = {
  open: "open",
  open_platform: "open",
  official: "open",
  wx_open: "open",
  mp: "mp",
  official_account: "mp",
  wechat_mp: "mp",
  mini_program: "mp",
  mobile: "mobile",
  mobile_app: "mobile",
  native_app: "mobile",
};

export function normalizeDefaultSubscriptionSettings(
  subscriptions: DefaultSubscriptionSetting[] | null | undefined,
): DefaultSubscriptionSetting[] {
  if (!Array.isArray(subscriptions)) return [];

  return subscriptions
    .filter((item) => item.group_id > 0 && item.validity_days > 0)
    .map((item) => ({
      group_id: Math.floor(item.group_id),
      validity_days: Math.min(
        36500,
        Math.max(1, Math.floor(item.validity_days)),
      ),
    }));
}

export function buildAuthSourceDefaultsState(
  settings: Partial<SystemSettings>,
): AuthSourceDefaultsState {
  const raw = settings as Record<string, unknown>;

  return AUTH_SOURCE_TYPES.reduce((acc, source) => {
    const subscriptions = raw[`auth_source_default_${source}_subscriptions`];
    acc[source] = {
      balance: Number(
        raw[`auth_source_default_${source}_balance`] ??
          AUTH_SOURCE_DEFAULT_BALANCE,
      ),
      concurrency: Math.max(
        1,
        Number(
          raw[`auth_source_default_${source}_concurrency`] ??
            AUTH_SOURCE_DEFAULT_CONCURRENCY,
        ),
      ),
      subscriptions: normalizeDefaultSubscriptionSettings(
        Array.isArray(subscriptions)
          ? (subscriptions as DefaultSubscriptionSetting[])
          : [],
      ),
      grant_on_signup:
        raw[`auth_source_default_${source}_grant_on_signup`] === true,
      grant_on_first_bind:
        raw[`auth_source_default_${source}_grant_on_first_bind`] === true,
      platform_quotas: normalizePlatformQuotasMap(raw[`auth_source_default_${source}_platform_quotas`] as DefaultPlatformQuotasMap | undefined),
    };
    return acc;
  }, {} as AuthSourceDefaultsState);
}

export function appendAuthSourceDefaultsToUpdateRequest(
  payload: UpdateSettingsRequest,
  authSourceDefaults: AuthSourceDefaultsState,
): UpdateSettingsRequest {
  const target = payload as Record<string, unknown>;

  for (const source of AUTH_SOURCE_TYPES) {
    const current = authSourceDefaults[source];
    target[`auth_source_default_${source}_balance`] =
      Number(current.balance) || 0;
    target[`auth_source_default_${source}_concurrency`] = Math.max(
      1,
      Math.floor(
        Number(current.concurrency) || AUTH_SOURCE_DEFAULT_CONCURRENCY,
      ),
    );
    target[`auth_source_default_${source}_subscriptions`] =
      normalizeDefaultSubscriptionSettings(current.subscriptions);
    target[`auth_source_default_${source}_grant_on_signup`] =
      current.grant_on_signup;
    target[`auth_source_default_${source}_grant_on_first_bind`] =
      current.grant_on_first_bind;
    target[`auth_source_default_${source}_platform_quotas`] = sanitizePlatformQuotasMap(current.platform_quotas)
  }

  return payload;
}

export function getPaymentVisibleMethodSourceOptions(
  method: PaymentVisibleMethod,
): PaymentVisibleMethodSourceOption[] {
  return PAYMENT_VISIBLE_METHOD_SOURCE_OPTIONS[method];
}

export function normalizePaymentVisibleMethodSource(
  method: PaymentVisibleMethod,
  source: unknown,
): PaymentVisibleMethodSource {
  if (typeof source !== "string") return "";

  const normalized = source.trim().toLowerCase();
  if (!normalized) return "";

  return PAYMENT_VISIBLE_METHOD_SOURCE_ALIASES[method][normalized] ?? "";
}

export function getWeChatConnectModeOptions(): WeChatConnectModeOption[] {
  return WECHAT_CONNECT_MODE_OPTIONS;
}

export function normalizeWeChatConnectMode(source: unknown): WeChatConnectMode {
  if (typeof source !== "string") return "open";

  const normalized = source.trim().toLowerCase();
  if (!normalized) return "open";

  return WECHAT_CONNECT_MODE_ALIASES[normalized] ?? "open";
}

export function defaultWeChatConnectScopesForMode(mode: unknown): string {
  switch (normalizeWeChatConnectMode(mode)) {
    case "mp":
      return "snsapi_userinfo";
    case "mobile":
      return "";
    default:
      return "snsapi_login";
  }
}

export function resolveWeChatConnectModeCapabilities(
  openEnabled: unknown,
  mpEnabled: unknown,
  mobileEnabled: unknown,
  legacyMode: unknown,
): { openEnabled: boolean; mpEnabled: boolean; mobileEnabled: boolean } {
  if (
    typeof openEnabled === "boolean" ||
    typeof mpEnabled === "boolean" ||
    typeof mobileEnabled === "boolean"
  ) {
    return {
      openEnabled: openEnabled === true,
      mpEnabled: mpEnabled === true,
      mobileEnabled: mobileEnabled === true,
    };
  }

  switch (normalizeWeChatConnectMode(legacyMode)) {
    case "mp":
      return { openEnabled: false, mpEnabled: true, mobileEnabled: false };
    case "mobile":
      return { openEnabled: false, mpEnabled: false, mobileEnabled: true };
    default:
      return { openEnabled: true, mpEnabled: false, mobileEnabled: false };
  }
}

export function deriveWeChatConnectStoredMode(
  openEnabled: boolean,
  mpEnabled: boolean,
  mobileEnabled: boolean,
  legacyMode: unknown,
): WeChatConnectMode {
  if (mpEnabled) return "mp";
  if (mobileEnabled) return "mobile";
  if (openEnabled) return "open";
  return normalizeWeChatConnectMode(legacyMode);
}

/**
 * System settings interface
 */
export interface SystemSettings {
  // Registration settings
  registration_enabled: boolean;
  email_verify_enabled: boolean;
  registration_email_suffix_whitelist: string[];
  promo_code_enabled: boolean;
  password_reset_enabled: boolean;
  frontend_url: string;
  invitation_code_enabled: boolean;
  totp_enabled: boolean; // TOTP 双因素认证
  totp_encryption_key_configured: boolean; // TOTP 加密密钥是否已配置
  passkey_enabled: boolean;
  passkey_configured: boolean;
  passkey_rp_id: string;
  passkey_rp_origins: string[];
  session_binding_enabled: boolean; // 会话 IP/UA 绑定
  step_up_enabled: boolean; // 敏感操作 step-up 2FA
  audit_log_retention_days: number; // 审计日志保留天数
  login_agreement_enabled: boolean;
  login_agreement_mode: "modal" | "checkbox" | string;
  login_agreement_updated_at: string;
  login_agreement_documents: LoginAgreementDocument[];
  // Default settings
  default_balance: number;
  affiliate_rebate_rate: number;
  affiliate_rebate_freeze_hours: number;
  affiliate_rebate_duration_days: number;
  affiliate_rebate_per_invitee_cap: number;
  affiliate_admin_recharge_enabled: boolean;
  default_concurrency: number;
  default_user_rpm_limit: number;
  default_subscriptions: DefaultSubscriptionSetting[];
  auth_source_default_email_balance?: number;
  auth_source_default_email_concurrency?: number;
  auth_source_default_email_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_email_grant_on_signup?: boolean;
  auth_source_default_email_grant_on_first_bind?: boolean;
  auth_source_default_linuxdo_balance?: number;
  auth_source_default_linuxdo_concurrency?: number;
  auth_source_default_linuxdo_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_linuxdo_grant_on_signup?: boolean;
  auth_source_default_linuxdo_grant_on_first_bind?: boolean;
  auth_source_default_oidc_balance?: number;
  auth_source_default_oidc_concurrency?: number;
  auth_source_default_oidc_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_oidc_grant_on_signup?: boolean;
  auth_source_default_oidc_grant_on_first_bind?: boolean;
  auth_source_default_wechat_balance?: number;
  auth_source_default_wechat_concurrency?: number;
  auth_source_default_wechat_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_wechat_grant_on_signup?: boolean;
  auth_source_default_wechat_grant_on_first_bind?: boolean;
  auth_source_default_dingtalk_balance?: number;
  auth_source_default_dingtalk_concurrency?: number;
  auth_source_default_dingtalk_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_dingtalk_grant_on_signup?: boolean;
  auth_source_default_dingtalk_grant_on_first_bind?: boolean;
  auth_source_default_github_balance?: number;
  auth_source_default_github_concurrency?: number;
  auth_source_default_github_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_github_grant_on_signup?: boolean;
  auth_source_default_github_grant_on_first_bind?: boolean;
  auth_source_default_google_balance?: number;
  auth_source_default_google_concurrency?: number;
  auth_source_default_google_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_google_grant_on_signup?: boolean;
  auth_source_default_google_grant_on_first_bind?: boolean;
  force_email_on_third_party_signup?: boolean;
  // ── 平台限额（嵌套 JSON，系统层 + 7 auth-source 层）────────────────────────────────
  default_platform_quotas?: DefaultPlatformQuotasMap;
  auth_source_default_email_platform_quotas?: DefaultPlatformQuotasMap;
  auth_source_default_linuxdo_platform_quotas?: DefaultPlatformQuotasMap;
  auth_source_default_oidc_platform_quotas?: DefaultPlatformQuotasMap;
  auth_source_default_wechat_platform_quotas?: DefaultPlatformQuotasMap;
  auth_source_default_github_platform_quotas?: DefaultPlatformQuotasMap;
  auth_source_default_google_platform_quotas?: DefaultPlatformQuotasMap;
  auth_source_default_dingtalk_platform_quotas?: DefaultPlatformQuotasMap;
  // OEM settings
  site_name: string;
  site_logo: string;
  site_subtitle: string;
  api_base_url: string;
  contact_info: string;
  doc_url: string;
	home_content: string;
	compact_home_enabled: boolean;
	hide_ccs_import_button: boolean;
  table_default_page_size: number;
  table_page_size_options: number[];
  backend_mode_enabled: boolean;
  stream_mode_performance_enabled: boolean;
  openai_ws_mode_router_v2_enabled: boolean;
  openai_visible_output_ttft_enabled: boolean;
  custom_menu_items: CustomMenuItem[];
  custom_endpoints: CustomEndpoint[];
  // SMTP settings
  smtp_host: string;
  smtp_port: number;
  smtp_username: string;
  smtp_password_configured: boolean;
  smtp_from_email: string;
  smtp_from_name: string;
  smtp_use_tls: boolean;
  // Human verification settings
  turnstile_enabled: boolean;
  turnstile_site_key: string;
  turnstile_secret_key_configured: boolean;
  recaptcha_enabled: boolean;
  recaptcha_site_key: string;
  recaptcha_secret_key_configured: boolean;
  cap_enabled: boolean;
  cap_api_endpoint: string;
  cap_secret_key_configured: boolean;
  tencent_captcha_enabled: boolean;
  tencent_captcha_app_id: string;
  tencent_captcha_app_secret_key_configured: boolean;
  tencent_captcha_cloud_secret_id_configured: boolean;
  tencent_captcha_cloud_secret_key_configured: boolean;
  tencent_captcha_region: "cn" | "intl" | string;
  aliyun_captcha_enabled: boolean;
  aliyun_captcha_access_key_id: string;
  aliyun_captcha_access_key_secret_configured: boolean;
  aliyun_captcha_scene_id: string;
  aliyun_captcha_prefix: string;
  aliyun_captcha_region: "cn" | "sgp" | string;
  local_captcha_enabled: boolean;
  api_key_acl_trust_forwarded_ip: boolean;
  client_ip_resolution_mode: ClientIPResolutionMode;
  client_ip_trusted_proxies: string[];
  client_ip_resolution_status: ClientIPResolutionStatus;

  // LinuxDo Connect OAuth settings
  linuxdo_connect_enabled: boolean;
  linuxdo_connect_client_id: string;
  linuxdo_connect_client_secret_configured: boolean;
  linuxdo_connect_redirect_url: string;

  // DingTalk Connect OAuth settings
  dingtalk_connect_enabled: boolean;
  dingtalk_connect_client_id: string;
  dingtalk_connect_client_secret_configured: boolean;
  dingtalk_connect_redirect_url: string;
  dingtalk_connect_corp_restriction_policy: string;
  dingtalk_connect_internal_corp_id: string;
  dingtalk_connect_bypass_registration: boolean;
  dingtalk_connect_sync_corp_email: boolean;
  dingtalk_connect_sync_display_name: boolean;
  dingtalk_connect_sync_dept: boolean;
  dingtalk_connect_sync_corp_email_attr_key: string;
  dingtalk_connect_sync_display_name_attr_key: string;
  dingtalk_connect_sync_dept_attr_key: string;
  dingtalk_connect_sync_corp_email_attr_name: string;
  dingtalk_connect_sync_display_name_attr_name: string;
  dingtalk_connect_sync_dept_attr_name: string;

  // WeChat Connect OAuth settings
  wechat_connect_enabled: boolean;
  wechat_connect_app_id: string;
  wechat_connect_app_secret_configured: boolean;
  wechat_connect_open_app_id?: string;
  wechat_connect_open_app_secret_configured?: boolean;
  wechat_connect_mp_app_id?: string;
  wechat_connect_mp_app_secret_configured?: boolean;
  wechat_connect_mobile_app_id?: string;
  wechat_connect_mobile_app_secret_configured?: boolean;
  wechat_connect_open_enabled?: boolean;
  wechat_connect_mp_enabled?: boolean;
  wechat_connect_mobile_enabled?: boolean;
  wechat_connect_mode: string;
  wechat_connect_scopes: string;
  wechat_connect_redirect_url: string;
  wechat_connect_frontend_redirect_url: string;

  // Generic OIDC OAuth settings
  oidc_connect_enabled: boolean;
  oidc_connect_provider_name: string;
  oidc_connect_client_id: string;
  oidc_connect_client_secret_configured: boolean;
  oidc_connect_issuer_url: string;
  oidc_connect_discovery_url: string;
  oidc_connect_authorize_url: string;
  oidc_connect_token_url: string;
  oidc_connect_userinfo_url: string;
  oidc_connect_jwks_url: string;
  oidc_connect_scopes: string;
  oidc_connect_redirect_url: string;
  oidc_connect_frontend_redirect_url: string;
  oidc_connect_token_auth_method: string;
  oidc_connect_use_pkce: boolean;
  oidc_connect_validate_id_token: boolean;
  oidc_connect_allowed_signing_algs: string;
  oidc_connect_clock_skew_seconds: number;
  oidc_connect_require_email_verified: boolean;
  oidc_connect_userinfo_email_path: string;
  oidc_connect_userinfo_id_path: string;
  oidc_connect_userinfo_username_path: string;
  github_oauth_enabled: boolean;
  github_oauth_client_id: string;
  github_oauth_client_secret_configured: boolean;
  github_oauth_redirect_url: string;
  github_oauth_frontend_redirect_url: string;
  google_oauth_enabled: boolean;
  google_oauth_client_id: string;
  google_oauth_client_secret_configured: boolean;
  google_oauth_redirect_url: string;
  google_oauth_frontend_redirect_url: string;

  // Model fallback configuration
  enable_model_fallback: boolean;
  fallback_model_anthropic: string;
  fallback_model_openai: string;
  fallback_model_gemini: string;
  fallback_model_antigravity: string;

  // Identity patch configuration (Claude -> Gemini)
  enable_identity_patch: boolean;
  identity_patch_prompt: string;

  // Ops Monitoring (vNext)
  ops_monitoring_enabled: boolean;
  ops_realtime_monitoring_enabled: boolean;
  ops_query_mode_default: "auto" | "raw" | "preagg" | string;
  ops_metrics_interval_seconds: number;

  // Claude Code version check
  min_claude_code_version: string;
  max_claude_code_version: string;

  // 分组隔离
  allow_ungrouped_key_scheduling: boolean;

  // 实验高性能调度引擎
  scheduler_v2_enabled: boolean;
  scheduler_v2_status: string;
  scheduler_v2_error: string;
  scheduler_v2_candidate_limit: number;
  scheduler_v2_scan_limit: number;
  request_priority_admission_enabled: boolean;
  request_priority_pending_limit_per_instance: number;
  request_priority_pending_mib_per_instance: number;

  // Gateway forwarding behavior
  enable_fingerprint_unification: boolean;
  enable_metadata_passthrough: boolean;
  enable_cch_signing: boolean;
  enable_claude_oauth_system_prompt_injection: boolean;
  claude_oauth_system_prompt: string;
  claude_oauth_system_prompt_blocks: string;
  enable_anthropic_cache_ttl_1h_injection: boolean;
  rewrite_message_cache_control: boolean;
  enable_client_dateline_normalization: boolean;
  antigravity_user_agent_version: string;
  openai_codex_user_agent: string;
  openai_codex_client_version: string;
  openai_codex_client_version_synced: string;
  openai_codex_version_auto_sync_enabled: boolean;
  // codex_cli_only 加固
  min_codex_version: string;
  max_codex_version: string;
  codex_cli_only_blacklist: string;
  codex_cli_only_whitelist: string;
  codex_cli_only_allow_app_server_clients: boolean;
  codex_cli_only_engine_fingerprint_signals: string;
  web_search_emulation_enabled?: boolean;

  // Payment configuration
  payment_enabled: boolean;
  risk_control_enabled: boolean;

  // Cyber session block
  cyber_session_block_enabled: boolean;
  cyber_session_block_ttl_seconds: number;

  payment_min_amount: number;
  payment_max_amount: number;
  payment_daily_limit: number;
  payment_order_timeout_minutes: number;
  payment_max_pending_orders: number;
  payment_enabled_types: string[];
  payment_balance_disabled: boolean;
  payment_balance_recharge_multiplier: number;
  payment_subscription_usd_to_cny_rate: number;
  payment_recharge_fee_rate: number;
  payment_load_balance_strategy: string;
  payment_product_name_prefix: string;
  payment_product_name_suffix: string;
  payment_help_image_url: string;
  payment_help_text: string;
  payment_cancel_rate_limit_enabled: boolean;
  payment_cancel_rate_limit_max: number;
  payment_cancel_rate_limit_window: number;
  payment_cancel_rate_limit_unit: string;
  payment_cancel_rate_limit_window_mode: string;
  payment_alipay_force_qrcode?: boolean;
  payment_alipay_mobile_precreate_deep_link?: boolean;
  payment_visible_method_alipay_source?: string;
  payment_visible_method_wxpay_source?: string;
  payment_visible_method_alipay_enabled?: boolean;
  payment_visible_method_wxpay_enabled?: boolean;
  openai_low_upstream_rate_priority_enabled?: boolean;
  openai_oauth_scheduling_rate_multiplier?: number;
  openai_content_session_burst_balance_enabled?: boolean;
  openai_advanced_scheduler_enabled?: boolean;
  openai_advanced_scheduler_sticky_weighted_enabled?: boolean;
  openai_advanced_scheduler_subscription_priority_enabled?: boolean;
  openai_advanced_scheduler_lb_top_k?: string;
  openai_advanced_scheduler_weight_priority?: string;
  openai_advanced_scheduler_weight_load?: string;
  openai_advanced_scheduler_weight_queue?: string;
  openai_advanced_scheduler_weight_error_rate?: string;
  openai_advanced_scheduler_weight_ttft?: string;
  openai_advanced_scheduler_weight_reset?: string;
  openai_advanced_scheduler_weight_quota_headroom?: string;
  openai_advanced_scheduler_weight_upstream_cost?: string;
  openai_advanced_scheduler_weight_previous_response?: string;
  openai_advanced_scheduler_weight_session_sticky?: string;
  openai_advanced_scheduler_effective_lb_top_k?: string;
  openai_advanced_scheduler_effective_weight_priority?: string;
  openai_advanced_scheduler_effective_weight_load?: string;
  openai_advanced_scheduler_effective_weight_queue?: string;
  openai_advanced_scheduler_effective_weight_error_rate?: string;
  openai_advanced_scheduler_effective_weight_ttft?: string;
  openai_advanced_scheduler_effective_weight_reset?: string;
  openai_advanced_scheduler_effective_weight_quota_headroom?: string;
  openai_advanced_scheduler_effective_weight_upstream_cost?: string;
  openai_advanced_scheduler_effective_weight_previous_response?: string;
  openai_advanced_scheduler_effective_weight_session_sticky?: string;

  // 余额、订阅到期与账号限额通知
  balance_low_notify_enabled: boolean;
  balance_low_notify_threshold: number;
  balance_low_notify_recharge_url: string;
  subscription_expiry_notify_enabled: boolean;
  account_quota_notify_enabled: boolean;
  account_quota_notify_emails: NotifyEmailEntry[];

  // Channel Monitor feature switch
  channel_monitor_enabled: boolean;
  channel_monitor_default_interval_seconds: number;
  channel_monitor_latency_unit: "ms" | "s" | string;
  channel_monitor_public_share_enabled: boolean;
  channel_monitor_public_share_require_auth: boolean;

  // Available Channels feature switch
  available_channels_enabled: boolean;

  // Support Chat feature switch
  support_chat_enabled: boolean;
  support_chat_retention_days: number;

  // Model Plaza feature switches + description
  model_plaza_enabled: boolean;
  model_plaza_require_auth: boolean;
  model_plaza_auto_public_models: boolean;
  model_plaza_description: string;

  // Media Studio feature switch
  media_studio_enabled: boolean;

  // IPv6 egress management UI feature switch
  ipv6_egress_ui_enabled: boolean;

  // Affiliate (邀请返利) feature switch
  affiliate_enabled: boolean;

  // OpenAI fast/flex policy
  openai_fast_policy_settings?: OpenAIFastPolicySettings;

  // Allow user view error requests
  allow_user_view_error_requests: boolean;
  // Allow user view usage details and billing calculations
  allow_user_view_usage_details: boolean;
}

export interface UpdateSettingsRequest {
  registration_enabled?: boolean;
  email_verify_enabled?: boolean;
  registration_email_suffix_whitelist?: string[];
  promo_code_enabled?: boolean;
  password_reset_enabled?: boolean;
  frontend_url?: string;
  invitation_code_enabled?: boolean;
  totp_enabled?: boolean; // TOTP 双因素认证
  passkey_enabled?: boolean;
  session_binding_enabled?: boolean; // 会话 IP/UA 绑定
  step_up_enabled?: boolean; // 敏感操作 step-up 2FA
  audit_log_retention_days?: number; // 审计日志保留天数
  login_agreement_enabled?: boolean;
  login_agreement_mode?: "modal" | "checkbox" | string;
  login_agreement_updated_at?: string;
  login_agreement_documents?: LoginAgreementDocument[];
  default_balance?: number;
  affiliate_rebate_rate?: number;
  affiliate_rebate_freeze_hours?: number;
  affiliate_rebate_duration_days?: number;
  affiliate_rebate_per_invitee_cap?: number;
  affiliate_admin_recharge_enabled?: boolean;
  default_concurrency?: number;
  default_user_rpm_limit?: number;
  default_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_email_balance?: number;
  auth_source_default_email_concurrency?: number;
  auth_source_default_email_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_email_grant_on_signup?: boolean;
  auth_source_default_email_grant_on_first_bind?: boolean;
  auth_source_default_linuxdo_balance?: number;
  auth_source_default_linuxdo_concurrency?: number;
  auth_source_default_linuxdo_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_linuxdo_grant_on_signup?: boolean;
  auth_source_default_linuxdo_grant_on_first_bind?: boolean;
  auth_source_default_oidc_balance?: number;
  auth_source_default_oidc_concurrency?: number;
  auth_source_default_oidc_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_oidc_grant_on_signup?: boolean;
  auth_source_default_oidc_grant_on_first_bind?: boolean;
  auth_source_default_wechat_balance?: number;
  auth_source_default_wechat_concurrency?: number;
  auth_source_default_wechat_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_wechat_grant_on_signup?: boolean;
  auth_source_default_wechat_grant_on_first_bind?: boolean;
  auth_source_default_dingtalk_balance?: number;
  auth_source_default_dingtalk_concurrency?: number;
  auth_source_default_dingtalk_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_dingtalk_grant_on_signup?: boolean;
  auth_source_default_dingtalk_grant_on_first_bind?: boolean;
  auth_source_default_github_balance?: number;
  auth_source_default_github_concurrency?: number;
  auth_source_default_github_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_github_grant_on_signup?: boolean;
  auth_source_default_github_grant_on_first_bind?: boolean;
  auth_source_default_google_balance?: number;
  auth_source_default_google_concurrency?: number;
  auth_source_default_google_subscriptions?: DefaultSubscriptionSetting[];
  auth_source_default_google_grant_on_signup?: boolean;
  auth_source_default_google_grant_on_first_bind?: boolean;
  force_email_on_third_party_signup?: boolean;
  // ── 平台限额（嵌套 JSON，系统层 + 7 auth-source 层）────────────────────────────────
  default_platform_quotas?: DefaultPlatformQuotasMap;
  auth_source_default_email_platform_quotas?: DefaultPlatformQuotasMap;
  auth_source_default_linuxdo_platform_quotas?: DefaultPlatformQuotasMap;
  auth_source_default_oidc_platform_quotas?: DefaultPlatformQuotasMap;
  auth_source_default_wechat_platform_quotas?: DefaultPlatformQuotasMap;
  auth_source_default_github_platform_quotas?: DefaultPlatformQuotasMap;
  auth_source_default_google_platform_quotas?: DefaultPlatformQuotasMap;
  auth_source_default_dingtalk_platform_quotas?: DefaultPlatformQuotasMap;
  site_name?: string;
  site_logo?: string;
  site_subtitle?: string;
  api_base_url?: string;
  contact_info?: string;
  doc_url?: string;
	home_content?: string;
	compact_home_enabled?: boolean;
	hide_ccs_import_button?: boolean;
  table_default_page_size?: number;
  table_page_size_options?: number[];
  backend_mode_enabled?: boolean;
  stream_mode_performance_enabled?: boolean;
  openai_ws_mode_router_v2_enabled?: boolean;
  openai_visible_output_ttft_enabled?: boolean;
  custom_menu_items?: CustomMenuItem[];
  custom_endpoints?: CustomEndpoint[];
  smtp_host?: string;
  smtp_port?: number;
  smtp_username?: string;
  smtp_password?: string;
  smtp_from_email?: string;
  smtp_from_name?: string;
  smtp_use_tls?: boolean;
  turnstile_enabled?: boolean;
  turnstile_site_key?: string;
  turnstile_secret_key?: string;
  recaptcha_enabled?: boolean;
  recaptcha_site_key?: string;
  recaptcha_secret_key?: string;
  cap_enabled?: boolean;
  cap_api_endpoint?: string;
  cap_secret_key?: string;
  tencent_captcha_enabled?: boolean;
  tencent_captcha_app_id?: string;
  tencent_captcha_app_secret_key?: string;
  tencent_captcha_cloud_secret_id?: string;
  tencent_captcha_cloud_secret_key?: string;
  tencent_captcha_region?: "cn" | "intl" | string;
  aliyun_captcha_enabled?: boolean;
  aliyun_captcha_access_key_id?: string;
  aliyun_captcha_access_key_secret?: string;
  aliyun_captcha_scene_id?: string;
  aliyun_captcha_prefix?: string;
  aliyun_captcha_region?: "cn" | "sgp" | string;
  local_captcha_enabled?: boolean;
  api_key_acl_trust_forwarded_ip?: boolean;
  client_ip_resolution_mode?: ClientIPResolutionMode;
  client_ip_trusted_proxies?: string[];
  linuxdo_connect_enabled?: boolean;
  linuxdo_connect_client_id?: string;
  linuxdo_connect_client_secret?: string;
  linuxdo_connect_redirect_url?: string;
  dingtalk_connect_enabled?: boolean;
  dingtalk_connect_client_id?: string;
  dingtalk_connect_client_secret?: string;
  dingtalk_connect_redirect_url?: string;
  dingtalk_connect_corp_restriction_policy?: string;
  dingtalk_connect_internal_corp_id?: string;
  dingtalk_connect_bypass_registration?: boolean;
  dingtalk_connect_sync_corp_email?: boolean;
  dingtalk_connect_sync_display_name?: boolean;
  dingtalk_connect_sync_dept?: boolean;
  dingtalk_connect_sync_corp_email_attr_key?: string;
  dingtalk_connect_sync_display_name_attr_key?: string;
  dingtalk_connect_sync_dept_attr_key?: string;
  dingtalk_connect_sync_corp_email_attr_name?: string;
  dingtalk_connect_sync_display_name_attr_name?: string;
  dingtalk_connect_sync_dept_attr_name?: string;
  wechat_connect_enabled?: boolean;
  wechat_connect_app_id?: string;
  wechat_connect_app_secret?: string;
  wechat_connect_open_app_id?: string;
  wechat_connect_open_app_secret?: string;
  wechat_connect_mp_app_id?: string;
  wechat_connect_mp_app_secret?: string;
  wechat_connect_mobile_app_id?: string;
  wechat_connect_mobile_app_secret?: string;
  wechat_connect_open_enabled?: boolean;
  wechat_connect_mp_enabled?: boolean;
  wechat_connect_mobile_enabled?: boolean;
  wechat_connect_mode?: string;
  wechat_connect_scopes?: string;
  wechat_connect_redirect_url?: string;
  wechat_connect_frontend_redirect_url?: string;
  oidc_connect_enabled?: boolean;
  oidc_connect_provider_name?: string;
  oidc_connect_client_id?: string;
  oidc_connect_client_secret?: string;
  oidc_connect_issuer_url?: string;
  oidc_connect_discovery_url?: string;
  oidc_connect_authorize_url?: string;
  oidc_connect_token_url?: string;
  oidc_connect_userinfo_url?: string;
  oidc_connect_jwks_url?: string;
  oidc_connect_scopes?: string;
  oidc_connect_redirect_url?: string;
  oidc_connect_frontend_redirect_url?: string;
  oidc_connect_token_auth_method?: string;
  oidc_connect_use_pkce?: boolean;
  oidc_connect_validate_id_token?: boolean;
  oidc_connect_allowed_signing_algs?: string;
  oidc_connect_clock_skew_seconds?: number;
  oidc_connect_require_email_verified?: boolean;
  oidc_connect_userinfo_email_path?: string;
  oidc_connect_userinfo_id_path?: string;
  oidc_connect_userinfo_username_path?: string;
  github_oauth_enabled?: boolean;
  github_oauth_client_id?: string;
  github_oauth_client_secret?: string;
  github_oauth_redirect_url?: string;
  github_oauth_frontend_redirect_url?: string;
  google_oauth_enabled?: boolean;
  google_oauth_client_id?: string;
  google_oauth_client_secret?: string;
  google_oauth_redirect_url?: string;
  google_oauth_frontend_redirect_url?: string;
  enable_model_fallback?: boolean;
  fallback_model_anthropic?: string;
  fallback_model_openai?: string;
  fallback_model_gemini?: string;
  fallback_model_antigravity?: string;
  enable_identity_patch?: boolean;
  identity_patch_prompt?: string;
  ops_monitoring_enabled?: boolean;
  ops_realtime_monitoring_enabled?: boolean;
  ops_query_mode_default?: "auto" | "raw" | "preagg" | string;
  ops_metrics_interval_seconds?: number;
  min_claude_code_version?: string;
  max_claude_code_version?: string;
  allow_ungrouped_key_scheduling?: boolean;
  scheduler_v2_enabled?: boolean;
  scheduler_v2_candidate_limit?: number;
  scheduler_v2_scan_limit?: number;
  request_priority_admission_enabled?: boolean;
  request_priority_pending_limit_per_instance?: number;
  request_priority_pending_mib_per_instance?: number;
  enable_fingerprint_unification?: boolean;
  enable_metadata_passthrough?: boolean;
  enable_cch_signing?: boolean;
  enable_claude_oauth_system_prompt_injection?: boolean;
  claude_oauth_system_prompt?: string;
  claude_oauth_system_prompt_blocks?: string;
  enable_anthropic_cache_ttl_1h_injection?: boolean;
  rewrite_message_cache_control?: boolean;
  enable_client_dateline_normalization?: boolean;
  antigravity_user_agent_version?: string;
  openai_codex_user_agent?: string;
  openai_codex_client_version?: string;
  openai_codex_version_auto_sync_enabled?: boolean;
  // codex_cli_only 加固
  min_codex_version?: string;
  max_codex_version?: string;
  codex_cli_only_blacklist?: string;
  codex_cli_only_whitelist?: string;
  codex_cli_only_allow_app_server_clients?: boolean;
  codex_cli_only_engine_fingerprint_signals?: string;
  // Payment configuration
  payment_enabled?: boolean;
  risk_control_enabled?: boolean;

  // Cyber session block
  cyber_session_block_enabled?: boolean;
  cyber_session_block_ttl_seconds?: number;

  payment_min_amount?: number;
  payment_max_amount?: number;
  payment_daily_limit?: number;
  payment_order_timeout_minutes?: number;
  payment_max_pending_orders?: number;
  payment_enabled_types?: string[];
  payment_balance_disabled?: boolean;
  payment_balance_recharge_multiplier?: number;
  payment_subscription_usd_to_cny_rate?: number;
  payment_recharge_fee_rate?: number;
  payment_load_balance_strategy?: string;
  payment_product_name_prefix?: string;
  payment_product_name_suffix?: string;
  payment_help_image_url?: string;
  payment_help_text?: string;
  payment_cancel_rate_limit_enabled?: boolean;
  payment_cancel_rate_limit_max?: number;
  payment_cancel_rate_limit_window?: number;
  payment_cancel_rate_limit_unit?: string;
  payment_cancel_rate_limit_window_mode?: string;
  payment_alipay_force_qrcode?: boolean;
  payment_alipay_mobile_precreate_deep_link?: boolean;
  payment_visible_method_alipay_source?: string;
  payment_visible_method_wxpay_source?: string;
  payment_visible_method_alipay_enabled?: boolean;
  payment_visible_method_wxpay_enabled?: boolean;
  openai_low_upstream_rate_priority_enabled?: boolean;
  openai_oauth_scheduling_rate_multiplier?: number;
  openai_content_session_burst_balance_enabled?: boolean;
  openai_advanced_scheduler_enabled?: boolean;
  openai_advanced_scheduler_sticky_weighted_enabled?: boolean;
  openai_advanced_scheduler_subscription_priority_enabled?: boolean;
  openai_advanced_scheduler_lb_top_k?: string;
  openai_advanced_scheduler_weight_priority?: string;
  openai_advanced_scheduler_weight_load?: string;
  openai_advanced_scheduler_weight_queue?: string;
  openai_advanced_scheduler_weight_error_rate?: string;
  openai_advanced_scheduler_weight_ttft?: string;
  openai_advanced_scheduler_weight_reset?: string;
  openai_advanced_scheduler_weight_quota_headroom?: string;
  openai_advanced_scheduler_weight_upstream_cost?: string;
  openai_advanced_scheduler_weight_previous_response?: string;
  openai_advanced_scheduler_weight_session_sticky?: string;
  // 余额、订阅到期与账号限额通知
  balance_low_notify_enabled?: boolean;
  balance_low_notify_threshold?: number;
  balance_low_notify_recharge_url?: string;
  subscription_expiry_notify_enabled?: boolean;
  account_quota_notify_enabled?: boolean;
  account_quota_notify_emails?: NotifyEmailEntry[];

  // Channel Monitor feature switch
  channel_monitor_enabled?: boolean;
  channel_monitor_default_interval_seconds?: number;
  channel_monitor_latency_unit?: "ms" | "s" | string;
  channel_monitor_public_share_enabled?: boolean;
  channel_monitor_public_share_require_auth?: boolean;

  // Available Channels feature switch
  available_channels_enabled?: boolean;

  // Support Chat feature switch
  support_chat_enabled?: boolean;
  support_chat_retention_days?: number;

  // Model Plaza feature switches + description
  model_plaza_enabled?: boolean;
  model_plaza_require_auth?: boolean;
  model_plaza_auto_public_models?: boolean;
  model_plaza_description?: string;

  // Media Studio feature switch
  media_studio_enabled?: boolean;

  // IPv6 egress management UI feature switch
  ipv6_egress_ui_enabled?: boolean;

  // Affiliate (邀请返利) feature switch
  affiliate_enabled?: boolean;

  // OpenAI fast/flex policy
  openai_fast_policy_settings?: OpenAIFastPolicySettings;

  allow_user_view_error_requests?: boolean;
  allow_user_view_usage_details?: boolean;
}

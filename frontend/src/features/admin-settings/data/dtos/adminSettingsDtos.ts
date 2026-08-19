export interface EmailTemplateOption {
  value: string;
  label?: string;
  description?: string;
  category?: string;
  optional?: boolean;
}

export type EmailTemplateEventOption = string | EmailTemplateOption;

export interface EmailTemplateSummary {
  event: string;
  locale: string;
  subject: string;
  is_custom?: boolean;
  updated_at?: string;
}

export interface EmailTemplateListResponse {
  events: EmailTemplateEventOption[];
  locales: string[];
  templates?: EmailTemplateSummary[];
  placeholders?: string[];
}

export interface EmailTemplateDetail {
  event: string;
  locale: string;
  subject: string;
  html: string;
  is_custom?: boolean;
  updated_at?: string;
  placeholders?: string[];
}

export interface UpdateEmailTemplateRequest {
  subject: string;
  html: string;
}

export interface PreviewEmailTemplateRequest extends UpdateEmailTemplateRequest {
  event: string;
  locale: string;
}

export interface EmailTemplatePreviewResponse {
  subject: string;
  html: string;
}

export interface TestSmtpRequest {
  smtp_host: string;
  smtp_port: number;
  smtp_username: string;
  smtp_password: string;
  smtp_use_tls: boolean;
}

export interface SendTestEmailRequest extends TestSmtpRequest {
  email: string;
  smtp_from_email: string;
  smtp_from_name: string;
}

export interface AdminApiKeyStatus {
  exists: boolean;
  masked_key: string;
}

export type AdminApiKeyScope =
  | "admin.read"
  | "admin.write"
  | "admin.users.read"
  | "admin.users.write"
  | "admin.accounts.read"
  | "admin.accounts.write"
  | "admin.settings.read"
  | "admin.settings.write"
  | "admin.backups.read"
  | "admin.backups.write"
  | "admin.system.read"
  | "admin.system.write"
  | "admin.audit.read"
  | "admin.audit.write"
  | "admin.ops.read"
  | "admin.ops.write";

export interface AdminApiKey {
  id: string;
  name: string;
  key_prefix: string;
  last_four: string;
  scopes: AdminApiKeyScope[];
  status: "active" | "revoked" | string;
  expires_at?: string | null;
  created_by: number;
  last_used_at?: string | null;
  created_at: string;
  updated_at: string;
  revoked_at?: string | null;
}

export interface CreateAdminApiKeyRequest {
  name: string;
  scopes: AdminApiKeyScope[];
  expires_at?: string | null;
}

export interface UpdateAdminApiKeyRequest {
  name?: string;
  scopes?: AdminApiKeyScope[];
  expires_at?: string | null;
}

export interface OverloadCooldownSettings {
  enabled: boolean;
  cooldown_minutes: number;
}

export interface RateLimit429CooldownSettings {
  enabled: boolean;
  cooldown_seconds: number;
}

export interface GlobalTempUnschedulableSettings {
  enabled: boolean;
}

export type CodexContinuationMode = "off" | "shadow" | "enforce";

export interface CodexSimulationSettings {
  full_simulation_enabled: boolean;
  continuation_mode: CodexContinuationMode;
  state_ttl_seconds: number;
  identity_secret_configured: boolean;
}

export type UpdateCodexSimulationSettings = Omit<
  CodexSimulationSettings,
  "identity_secret_configured"
>;

export interface StreamTimeoutSettings {
  response_header_timeout_degradation_enabled: boolean;
  response_header_timeout_seconds: number;
  enabled: boolean;
  action: "temp_unsched" | "error" | "none";
  temp_unsched_minutes: number;
  threshold_count: number;
  threshold_window_minutes: number;
  openai_first_output_timeout_seconds?: number;
  openai_high_effort_first_output_timeout_seconds?: number;
  stream_keepalive_interval_seconds?: number;
}

/**
 * `display_only` only reveals generated summaries; `force` can change cost and
 * cache behavior by enabling thinking when the request did not ask for it.
 */
export type ThinkingDisplayMode = "off" | "display_only" | "force";

export interface RectifierSettings {
  enabled: boolean;
  thinking_signature_enabled: boolean;
  thinking_budget_enabled: boolean;
  thinking_display_mode: ThinkingDisplayMode;
  apikey_signature_enabled: boolean;
  apikey_signature_patterns: string[];
}

export interface OpenAIFastPolicyRule {
  service_tier: "all" | "priority" | "flex";
  action: "pass" | "filter" | "block" | "force_priority";
  scope: "all" | "oauth" | "apikey" | "bedrock";
  user_ids?: number[];
  error_message?: string;
  model_whitelist?: string[];
  fallback_action?: "pass" | "filter" | "block" | "force_priority";
  fallback_error_message?: string;
}

export interface OpenAIFastPolicySettings {
  rules: OpenAIFastPolicyRule[];
}

export interface BetaPolicyRule {
  beta_token: string;
  action: "pass" | "filter" | "block";
  scope: "all" | "oauth" | "apikey" | "bedrock";
  error_message?: string;
  model_whitelist?: string[];
  fallback_action?: "pass" | "filter" | "block";
  fallback_error_message?: string;
}

export interface BetaPolicySettings {
  rules: BetaPolicyRule[];
}

export interface WebSearchProviderConfig {
  type: "brave" | "tavily";
  api_key: string;
  api_key_configured: boolean;
  quota_limit: number | null;
  subscribed_at: number | null;
  quota_used?: number;
  proxy_id: number | null;
  expires_at: number | null;
}

export interface WebSearchEmulationConfig {
  enabled: boolean;
  providers: WebSearchProviderConfig[];
}

export interface WebSearchTestResult {
  provider: string;
  results: Array<{
    url: string;
    title: string;
    snippet: string;
    page_age?: string;
  }>;
  query: string;
}

/**
 * Authenticated panel endpoints are limited per user account; public endpoints
 * are limited per publicly routable client IP.
 */
export interface PanelRateLimitSettings {
  enabled: boolean;
  user_rpm: number;
  heavy_rpm: number;
  exempt_admin: boolean;
  public_ip_rpm: number;
}

export const DEFAULT_PANEL_RATE_LIMIT_SETTINGS: Readonly<PanelRateLimitSettings> =
  Object.freeze({
    enabled: false,
    user_rpm: 240,
    heavy_rpm: 60,
    exempt_admin: true,
    public_ip_rpm: 300,
  });

const PANEL_RATE_LIMIT_RPM_MAX = 100000;

function normalizePanelRate(value: unknown, fallback: number): number {
  return typeof value === "number" &&
    Number.isInteger(value) &&
    value >= 0 &&
    value <= PANEL_RATE_LIMIT_RPM_MAX
    ? value
    : fallback;
}

export function normalizePanelRateLimitSettings(
  input: unknown,
): PanelRateLimitSettings {
  const value =
    input && typeof input === "object"
      ? (input as Record<string, unknown>)
      : {};

  return {
    enabled: value.enabled === true,
    user_rpm: normalizePanelRate(
      value.user_rpm,
      DEFAULT_PANEL_RATE_LIMIT_SETTINGS.user_rpm,
    ),
    heavy_rpm: normalizePanelRate(
      value.heavy_rpm,
      DEFAULT_PANEL_RATE_LIMIT_SETTINGS.heavy_rpm,
    ),
    exempt_admin:
      typeof value.exempt_admin === "boolean"
        ? value.exempt_admin
        : DEFAULT_PANEL_RATE_LIMIT_SETTINGS.exempt_admin,
    public_ip_rpm: normalizePanelRate(
      value.public_ip_rpm,
      DEFAULT_PANEL_RATE_LIMIT_SETTINGS.public_ip_rpm,
    ),
  };
}

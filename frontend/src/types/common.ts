import type { UserSubscription } from './usage'

/**
 * Core Type Definitions for Sub2API Frontend
 */

// ==================== Common Types ====================

export interface SelectOption {
  value: string | number | boolean | null
  label: string
  [key: string]: any // Support extra properties for custom templates
}

export interface BasePaginationResponse<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  pages: number
}

export interface FetchOptions {
  signal?: AbortSignal
}

// ==================== Notification Types ====================

/** Notification email entry with enable/disable and verification state.
 *  email="" is a placeholder for the primary email (user's registration email or admin email). */
export interface NotifyEmailEntry {
  email: string
  disabled: boolean
  verified: boolean
}

// ==================== User & Auth Types ====================

export type UserAuthProvider = 'email' | 'linuxdo' | 'oidc' | 'wechat' | 'github' | 'google' | 'dingtalk'

export interface UserAuthBindingStatus {
  bound?: boolean
  bound_count?: number
  provider?: UserAuthProvider | string
  provider_key?: string | null
  provider_subject?: string | null
  issuer?: string | null
  label?: string | null
  provider_label?: string | null
  display_name?: string | null
  subject_hint?: string | null
  verified_at?: string | null
  bind_start_path?: string | null
  can_bind?: boolean
  can_unbind?: boolean
  note_key?: string | null
  note?: string | null
  metadata?: Record<string, unknown>
}

export interface UserProfileSourceContext {
  provider?: UserAuthProvider | string
  source?: string | null
  label?: string | null
  provider_label?: string | null
}

export interface User {
  id: number
  username: string
  email: string
  avatar_url?: string | null
  avatar_source?: string | UserProfileSourceContext | null
  username_source?: string | UserProfileSourceContext | null
  display_name_source?: string | UserProfileSourceContext | null
  nickname_source?: string | UserProfileSourceContext | null
  profile_sources?: {
    avatar?: string | UserProfileSourceContext | null
    username?: string | UserProfileSourceContext | null
    display_name?: string | UserProfileSourceContext | null
    nickname?: string | UserProfileSourceContext | null
  }
  auth_bindings?: Partial<Record<UserAuthProvider, boolean | UserAuthBindingStatus>>
  identity_bindings?: Partial<Record<UserAuthProvider, boolean | UserAuthBindingStatus>>
  email_bound?: boolean
  linuxdo_bound?: boolean
  oidc_bound?: boolean
  wechat_bound?: boolean
  role: 'admin' | 'user' // User role for authorization
  balance: number // User balance for API usage
  frozen_balance?: number // Balance currently held by async batch jobs
  available_balance?: number // Balance after queued usage awaiting settlement
  pending_settlement?: number // Queued usage not yet persisted to the ledger balance
  balance_sync_status?: 'synced' | 'unavailable'
  concurrency: number // Allowed concurrent requests
  rpm_limit?: number // User-level RPM cap (0 = unlimited); effective as fallback when group has no rpm_limit
  status: 'active' | 'disabled' // Account status
  allowed_groups: number[] | null // Allowed group IDs (null = all non-exclusive groups)
  balance_notify_enabled: boolean
  balance_notify_threshold: number | null
  balance_notify_extra_emails: NotifyEmailEntry[]
  subscriptions?: UserSubscription[] // User's active subscriptions
  last_active_at?: string | null
  created_at: string
  updated_at: string
  deleted_at?: string | null
}

export interface AdminUser extends User {
  // 管理员备注（普通用户接口不返回）
  notes: string
  last_used_at?: string | null
  // 用户专属分组倍率配置 (group_id -> rate_multiplier)
  group_rates?: Record<number, number>
  // 当前并发数（仅管理员列表接口返回）
  current_concurrency?: number
  // 管理员可见的请求调度等级：0 优先，1 普通，2 低调度
  scheduling_tier: RequestSchedulingTier
}

export type RequestSchedulingTier = 0 | 1 | 2

export interface TencentCaptchaRequestProof {
  tencent_captcha_ticket: string
  tencent_captcha_randstr: string
}

export interface ActionCaptchaRequestProof extends Partial<TencentCaptchaRequestProof> {
  captcha_token?: string
  turnstile_token?: string
}

export interface LoginRequest {
  email: string
  password: string
  turnstile_token?: string
  captcha_token?: string
  captcha_id?: string
  captcha_code?: string
  tencent_captcha_ticket?: string
  tencent_captcha_randstr?: string
}

export interface CredentialEnvelope {
  algorithm: 'RSA-OAEP-256+A256GCM'
  key_id: string
  encrypted_key: string
  iv: string
  ciphertext: string
}

export interface RegisterRequest {
  email: string
  password: string
  verify_code?: string
  turnstile_token?: string
  captcha_token?: string
  captcha_id?: string
  captcha_code?: string
  tencent_captcha_ticket?: string
  tencent_captcha_randstr?: string
  promo_code?: string
  invitation_code?: string
  aff_code?: string
}

export interface EncryptedRegisterRequest extends Omit<RegisterRequest, 'email' | 'password'> {
  credential_envelope: CredentialEnvelope
}

export interface AffiliateInvitee {
  user_id: number
  email: string
  username: string
  created_at?: string
  total_rebate: number
}

export interface UserAffiliateDetail {
  user_id: number
  aff_code: string
  inviter_id?: number | null
  aff_count: number
  aff_quota: number
  aff_frozen_quota: number
  aff_history_quota: number
  /** 当前用户作为邀请人时实际生效的返利比例（专属覆盖全局）。0-100。 */
  effective_rebate_rate_percent: number
  invitees: AffiliateInvitee[]
}

export interface AffiliateTransferResponse {
  transferred_quota: number
  balance: number
}

export interface SendVerifyCodeRequest {
  email: string
  turnstile_token?: string
  captcha_token?: string
  captcha_id?: string
  captcha_code?: string
  tencent_captcha_ticket?: string
  tencent_captcha_randstr?: string
  pending_auth_token?: string
  pending_oauth_token?: string
}

export interface SendVerifyCodeResponse {
  message: string
  countdown: number
}

export interface CustomMenuItem {
  id: string
  label: string
  icon_svg: string
  url: string
  page_slug?: string
  visibility: 'user' | 'admin'
  sort_order: number
}

export interface CustomEndpoint {
  name: string
  endpoint: string
  description: string
}

export interface LoginAgreementDocument {
  id: string
  title: string
  content_md: string
}

export interface PublicSettings {
  registration_enabled: boolean
  email_verify_enabled: boolean
  force_email_on_third_party_signup: boolean
  registration_email_suffix_whitelist: string[]
  promo_code_enabled: boolean
  password_reset_enabled: boolean
  invitation_code_enabled: boolean
  passkey_enabled?: boolean
  login_agreement_enabled?: boolean
  login_agreement_mode?: 'modal' | 'checkbox' | string
  login_agreement_updated_at?: string
  login_agreement_revision?: string
  login_agreement_documents?: LoginAgreementDocument[]
  turnstile_enabled: boolean
  turnstile_site_key: string
  recaptcha_enabled: boolean
  recaptcha_site_key: string
  cap_enabled: boolean
  cap_api_endpoint: string
  tencent_captcha_enabled?: boolean
  tencent_captcha_app_id?: string
  tencent_captcha_region?: 'cn' | 'intl' | string
  aliyun_captcha_enabled?: boolean
  aliyun_captcha_scene_id?: string
  aliyun_captcha_prefix?: string
  aliyun_captcha_region?: 'cn' | 'sgp' | string
  local_captcha_enabled?: boolean
  site_name: string
  site_logo: string
  site_subtitle: string
  api_base_url: string
  contact_info: string
  doc_url: string
	home_content: string
	compact_home_enabled: boolean
	hide_ccs_import_button: boolean
  payment_enabled: boolean
  risk_control_enabled: boolean
  table_default_page_size: number
  table_page_size_options: number[]
  custom_menu_items: CustomMenuItem[]
  custom_endpoints: CustomEndpoint[]
  linuxdo_oauth_enabled: boolean
  dingtalk_oauth_enabled?: boolean
  wechat_oauth_enabled: boolean
  wechat_oauth_open_enabled?: boolean
  wechat_oauth_mp_enabled?: boolean
  wechat_oauth_mobile_enabled?: boolean
  oidc_oauth_enabled: boolean
  oidc_oauth_provider_name: string
  github_oauth_enabled: boolean
  google_oauth_enabled: boolean
  backend_mode_enabled: boolean
  version: string
  // 服务器全局时区（IANA 名称与当前 UTC 偏移），高峰时段等服务端本地时间窗口的展示标注用；
  // 可选：注入的 __APP_CONFIG__ 旧缓存可能缺失
  server_timezone?: string
  server_utc_offset?: string
  balance_low_notify_enabled: boolean
  account_quota_notify_enabled: boolean
  balance_low_notify_threshold: number
  channel_monitor_enabled: boolean
  channel_monitor_default_interval_seconds: number
  channel_monitor_latency_unit?: 'ms' | 's' | string
  channel_monitor_public_share_enabled?: boolean
  channel_monitor_public_share_require_auth?: boolean
  available_channels_enabled: boolean
  support_chat_enabled: boolean
  model_plaza_enabled: boolean
  model_plaza_require_auth: boolean
  service_quota_enabled: boolean
  affiliate_enabled: boolean
  allow_user_view_error_requests?: boolean
  allow_user_view_usage_details?: boolean
}

export interface AuthResponse {
  access_token: string
  refresh_token?: string  // New: Refresh Token for token renewal
  expires_in?: number     // New: Access Token expiry time in seconds
  token_type: string
  user: User & { run_mode?: 'standard' | 'simple' }
}

export interface CurrentUserResponse extends User {
  run_mode?: 'standard' | 'simple'
}

// ==================== Subscription Types ====================

export interface Subscription {
  id: number
  user_id: number
  name: string
  url: string
  type: 'clash' | 'v2ray' | 'surge' | 'quantumult' | 'shadowrocket'
  update_interval: number // in hours
  last_updated: string | null
  node_count: number
  is_active: boolean
  created_at: string
  updated_at: string
}

export interface CreateSubscriptionRequest {
  name: string
  url: string
  type: Subscription['type']
  update_interval?: number
}

export interface UpdateSubscriptionRequest {
  name?: string
  url?: string
  type?: Subscription['type']
  update_interval?: number
  is_active?: boolean
}

// ==================== Announcement Types ====================

export type AnnouncementStatus = 'draft' | 'active' | 'archived'
export type AnnouncementNotifyMode = 'silent' | 'popup'

export type AnnouncementConditionType = 'subscription' | 'balance'

export type AnnouncementOperator = 'in' | 'gt' | 'gte' | 'lt' | 'lte' | 'eq'

export interface AnnouncementCondition {
  type: AnnouncementConditionType
  operator: AnnouncementOperator
  group_ids?: number[]
  value?: number
}

export interface AnnouncementConditionGroup {
  all_of?: AnnouncementCondition[]
}

export interface AnnouncementTargeting {
  any_of?: AnnouncementConditionGroup[]
}

export interface Announcement {
  id: number
  title: string
  content: string
  status: AnnouncementStatus
  notify_mode: AnnouncementNotifyMode
  targeting: AnnouncementTargeting
  starts_at?: string
  ends_at?: string
  created_by?: number
  updated_by?: number
  created_at: string
  updated_at: string
}

export interface UserAnnouncement {
  id: number
  title: string
  content: string
  notify_mode: AnnouncementNotifyMode
  starts_at?: string
  ends_at?: string
  read_at?: string
  created_at: string
  updated_at: string
}

export interface CreateAnnouncementRequest {
  title: string
  content: string
  status?: AnnouncementStatus
  notify_mode?: AnnouncementNotifyMode
  targeting: AnnouncementTargeting
  starts_at?: number
  ends_at?: number
}

export interface UpdateAnnouncementRequest {
  title?: string
  content?: string
  status?: AnnouncementStatus
  notify_mode?: AnnouncementNotifyMode
  targeting?: AnnouncementTargeting
  starts_at?: number
  ends_at?: number
}

export interface AnnouncementUserReadStatus {
  user_id: number
  email: string
  username: string
  balance: number
  eligible: boolean
  read_at?: string
}

// ==================== Proxy Node Types ====================

export interface ProxyNode {
  id: number
  subscription_id: number
  name: string
  type: 'ss' | 'ssr' | 'vmess' | 'vless' | 'trojan' | 'hysteria' | 'hysteria2'
  server: string
  port: number
  config: Record<string, unknown> // JSON configuration specific to proxy type
  latency: number | null // in milliseconds
  last_checked: string | null
  is_available: boolean
  created_at: string
  updated_at: string
}

// ==================== Conversion Types ====================

export interface ConversionRequest {
  subscription_ids: number[]
  target_type: 'clash' | 'v2ray' | 'surge' | 'quantumult' | 'shadowrocket'
  filter?: {
    name_pattern?: string
    types?: ProxyNode['type'][]
    min_latency?: number
    max_latency?: number
    available_only?: boolean
  }
  sort?: {
    by: 'name' | 'latency' | 'type'
    order: 'asc' | 'desc'
  }
}

export interface ConversionResult {
  url: string // URL to download the converted subscription
  expires_at: string
  node_count: number
}

// ==================== Statistics Types ====================

export interface SubscriptionStats {
  subscription_id: number
  total_nodes: number
  available_nodes: number
  avg_latency: number | null
  by_type: Record<ProxyNode['type'], number>
  last_update: string
}

export interface UserStats {
  total_subscriptions: number
  total_nodes: number
  active_subscriptions: number
  total_conversions: number
  last_conversion: string | null
}

// ==================== API Response Types ====================

export interface ApiResponse<T = unknown> {
  code: number
  message: string
  data: T
}

export interface ApiError {
  detail: string
  code?: string
  field?: string
}

export interface PaginatedResponse<T> {
  items: T[]
  total: number
  page: number
  page_size: number
  pages: number
}

// ==================== UI State Types ====================

export type ToastType = 'success' | 'error' | 'info' | 'warning'

export interface Toast {
  id: string
  type: ToastType
  message: string
  title?: string
  duration?: number // in milliseconds, undefined means no auto-dismiss
  startTime?: number // timestamp when toast was created, for progress bar
}

export interface AppState {
  sidebarCollapsed: boolean
  loading: boolean
  toasts: Toast[]
}

// ==================== Validation Types ====================

export interface ValidationError {
  field: string
  message: string
}

// ==================== Table/List Types ====================

export interface SortConfig {
  key: string
  order: 'asc' | 'desc'
}

export interface FilterConfig {
  [key: string]: string | number | boolean | null | undefined
}

export interface PaginationConfig {
  page: number
  page_size: number
}

// ==================== API Key & Group Types ====================

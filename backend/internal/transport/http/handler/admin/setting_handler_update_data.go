package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler/dto"
)

// UpdateSettingsRequest 更新设置请求
type UpdateSettingsRequest struct {
	// 注册设置
	RegistrationEnabled              bool                         `json:"registration_enabled"`
	EmailVerifyEnabled               bool                         `json:"email_verify_enabled"`
	RegistrationEmailSuffixWhitelist []string                     `json:"registration_email_suffix_whitelist"`
	PromoCodeEnabled                 bool                         `json:"promo_code_enabled"`
	PasswordResetEnabled             bool                         `json:"password_reset_enabled"`
	FrontendURL                      string                       `json:"frontend_url"`
	InvitationCodeEnabled            bool                         `json:"invitation_code_enabled"`
	TotpEnabled                      bool                         `json:"totp_enabled"`             // TOTP 双因素认证
	PasskeyEnabled                   *bool                        `json:"passkey_enabled"`          // Passkey 登录（省略=保持现值）
	SessionBindingEnabled            *bool                        `json:"session_binding_enabled"`  // 会话 IP/UA 绑定（省略=保持现值）
	StepUpEnabled                    *bool                        `json:"step_up_enabled"`          // 敏感操作 step-up 2FA（省略=保持现值）
	AuditLogRetentionDays            int                          `json:"audit_log_retention_days"` // 审计日志保留天数
	LoginAgreementEnabled            bool                         `json:"login_agreement_enabled"`
	LoginAgreementMode               string                       `json:"login_agreement_mode"`
	LoginAgreementUpdatedAt          string                       `json:"login_agreement_updated_at"`
	LoginAgreementDocuments          []dto.LoginAgreementDocument `json:"login_agreement_documents"`

	// 邮件服务设置
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	SMTPPassword string `json:"smtp_password"`
	SMTPFrom     string `json:"smtp_from_email"`
	SMTPFromName string `json:"smtp_from_name"`
	SMTPUseTLS   bool   `json:"smtp_use_tls"`

	// 人机验证设置。各 enabled 字段在服务端强制互斥。
	TurnstileEnabled             bool   `json:"turnstile_enabled"`
	TurnstileSiteKey             string `json:"turnstile_site_key"`
	TurnstileSecretKey           string `json:"turnstile_secret_key"`
	RecaptchaEnabled             *bool  `json:"recaptcha_enabled"` // 省略=保持现值
	RecaptchaSiteKey             string `json:"recaptcha_site_key"`
	RecaptchaSecretKey           string `json:"recaptcha_secret_key"`
	CapEnabled                   *bool  `json:"cap_enabled"` // 省略=保持现值
	CapAPIEndpoint               string `json:"cap_api_endpoint"`
	CapSecretKey                 string `json:"cap_secret_key"`
	TencentCaptchaEnabled        *bool  `json:"tencent_captcha_enabled"`
	TencentCaptchaAppID          string `json:"tencent_captcha_app_id"`
	TencentCaptchaAppSecretKey   string `json:"tencent_captcha_app_secret_key"`
	TencentCaptchaCloudSecretID  string `json:"tencent_captcha_cloud_secret_id"`
	TencentCaptchaCloudSecretKey string `json:"tencent_captcha_cloud_secret_key"`
	TencentCaptchaRegion         string `json:"tencent_captcha_region"`
	AliyunCaptchaEnabled         *bool  `json:"aliyun_captcha_enabled"`
	AliyunCaptchaAccessKeyID     string `json:"aliyun_captcha_access_key_id"`
	AliyunCaptchaAccessKeySecret string `json:"aliyun_captcha_access_key_secret"`
	AliyunCaptchaSceneID         string `json:"aliyun_captcha_scene_id"`
	AliyunCaptchaPrefix          string `json:"aliyun_captcha_prefix"`
	AliyunCaptchaRegion          string `json:"aliyun_captcha_region"`
	LocalCaptchaEnabled          *bool  `json:"local_captcha_enabled"` // 省略=保持现值

	// API Key IP 访问控制设置
	APIKeyACLTrustForwardedIP *bool     `json:"api_key_acl_trust_forwarded_ip"`
	ClientIPResolutionMode    *string   `json:"client_ip_resolution_mode"`
	ClientIPTrustedProxies    *[]string `json:"client_ip_trusted_proxies"`

	// LinuxDo Connect OAuth 登录
	LinuxDoConnectEnabled      bool   `json:"linuxdo_connect_enabled"`
	LinuxDoConnectClientID     string `json:"linuxdo_connect_client_id"`
	LinuxDoConnectClientSecret string `json:"linuxdo_connect_client_secret"`
	LinuxDoConnectRedirectURL  string `json:"linuxdo_connect_redirect_url"`

	// DingTalk Connect OAuth 登录
	DingTalkConnectEnabled                 bool   `json:"dingtalk_connect_enabled"`
	DingTalkConnectClientID                string `json:"dingtalk_connect_client_id"`
	DingTalkConnectClientSecret            string `json:"dingtalk_connect_client_secret"`
	DingTalkConnectRedirectURL             string `json:"dingtalk_connect_redirect_url"`
	DingTalkConnectCorpRestrictionPolicy   string `json:"dingtalk_connect_corp_restriction_policy"`
	DingTalkConnectInternalCorpID          string `json:"dingtalk_connect_internal_corp_id"`
	DingTalkConnectBypassRegistration      bool   `json:"dingtalk_connect_bypass_registration"`
	DingTalkConnectSyncCorpEmail           bool   `json:"dingtalk_connect_sync_corp_email"`
	DingTalkConnectSyncDisplayName         bool   `json:"dingtalk_connect_sync_display_name"`
	DingTalkConnectSyncDept                bool   `json:"dingtalk_connect_sync_dept"`
	DingTalkConnectSyncCorpEmailAttrKey    string `json:"dingtalk_connect_sync_corp_email_attr_key"`
	DingTalkConnectSyncDisplayNameAttrKey  string `json:"dingtalk_connect_sync_display_name_attr_key"`
	DingTalkConnectSyncDeptAttrKey         string `json:"dingtalk_connect_sync_dept_attr_key"`
	DingTalkConnectSyncCorpEmailAttrName   string `json:"dingtalk_connect_sync_corp_email_attr_name"`
	DingTalkConnectSyncDisplayNameAttrName string `json:"dingtalk_connect_sync_display_name_attr_name"`
	DingTalkConnectSyncDeptAttrName        string `json:"dingtalk_connect_sync_dept_attr_name"`

	// WeChat Connect OAuth 登录
	WeChatConnectEnabled             bool   `json:"wechat_connect_enabled"`
	WeChatConnectAppID               string `json:"wechat_connect_app_id"`
	WeChatConnectAppSecret           string `json:"wechat_connect_app_secret"`
	WeChatConnectOpenAppID           string `json:"wechat_connect_open_app_id"`
	WeChatConnectOpenAppSecret       string `json:"wechat_connect_open_app_secret"`
	WeChatConnectMPAppID             string `json:"wechat_connect_mp_app_id"`
	WeChatConnectMPAppSecret         string `json:"wechat_connect_mp_app_secret"`
	WeChatConnectMobileAppID         string `json:"wechat_connect_mobile_app_id"`
	WeChatConnectMobileAppSecret     string `json:"wechat_connect_mobile_app_secret"`
	WeChatConnectOpenEnabled         bool   `json:"wechat_connect_open_enabled"`
	WeChatConnectMPEnabled           bool   `json:"wechat_connect_mp_enabled"`
	WeChatConnectMobileEnabled       bool   `json:"wechat_connect_mobile_enabled"`
	WeChatConnectMode                string `json:"wechat_connect_mode"`
	WeChatConnectScopes              string `json:"wechat_connect_scopes"`
	WeChatConnectRedirectURL         string `json:"wechat_connect_redirect_url"`
	WeChatConnectFrontendRedirectURL string `json:"wechat_connect_frontend_redirect_url"`

	// Generic OIDC OAuth 登录
	OIDCConnectEnabled              bool   `json:"oidc_connect_enabled"`
	OIDCConnectProviderName         string `json:"oidc_connect_provider_name"`
	OIDCConnectClientID             string `json:"oidc_connect_client_id"`
	OIDCConnectClientSecret         string `json:"oidc_connect_client_secret"`
	OIDCConnectIssuerURL            string `json:"oidc_connect_issuer_url"`
	OIDCConnectDiscoveryURL         string `json:"oidc_connect_discovery_url"`
	OIDCConnectAuthorizeURL         string `json:"oidc_connect_authorize_url"`
	OIDCConnectTokenURL             string `json:"oidc_connect_token_url"`
	OIDCConnectUserInfoURL          string `json:"oidc_connect_userinfo_url"`
	OIDCConnectJWKSURL              string `json:"oidc_connect_jwks_url"`
	OIDCConnectScopes               string `json:"oidc_connect_scopes"`
	OIDCConnectRedirectURL          string `json:"oidc_connect_redirect_url"`
	OIDCConnectFrontendRedirectURL  string `json:"oidc_connect_frontend_redirect_url"`
	OIDCConnectTokenAuthMethod      string `json:"oidc_connect_token_auth_method"`
	OIDCConnectUsePKCE              *bool  `json:"oidc_connect_use_pkce"`
	OIDCConnectValidateIDToken      *bool  `json:"oidc_connect_validate_id_token"`
	OIDCConnectAllowedSigningAlgs   string `json:"oidc_connect_allowed_signing_algs"`
	OIDCConnectClockSkewSeconds     int    `json:"oidc_connect_clock_skew_seconds"`
	OIDCConnectRequireEmailVerified bool   `json:"oidc_connect_require_email_verified"`
	OIDCConnectUserInfoEmailPath    string `json:"oidc_connect_userinfo_email_path"`
	OIDCConnectUserInfoIDPath       string `json:"oidc_connect_userinfo_id_path"`
	OIDCConnectUserInfoUsernamePath string `json:"oidc_connect_userinfo_username_path"`

	GitHubOAuthEnabled             bool   `json:"github_oauth_enabled"`
	GitHubOAuthClientID            string `json:"github_oauth_client_id"`
	GitHubOAuthClientSecret        string `json:"github_oauth_client_secret"`
	GitHubOAuthRedirectURL         string `json:"github_oauth_redirect_url"`
	GitHubOAuthFrontendRedirectURL string `json:"github_oauth_frontend_redirect_url"`
	GoogleOAuthEnabled             bool   `json:"google_oauth_enabled"`
	GoogleOAuthClientID            string `json:"google_oauth_client_id"`
	GoogleOAuthClientSecret        string `json:"google_oauth_client_secret"`
	GoogleOAuthRedirectURL         string `json:"google_oauth_redirect_url"`
	GoogleOAuthFrontendRedirectURL string `json:"google_oauth_frontend_redirect_url"`

	// OEM设置
	SiteName                    string                `json:"site_name"`
	SiteLogo                    string                `json:"site_logo"`
	SiteSubtitle                string                `json:"site_subtitle"`
	APIBaseURL                  string                `json:"api_base_url"`
	ContactInfo                 string                `json:"contact_info"`
	DocURL                      string                `json:"doc_url"`
	HomeContent                 string                `json:"home_content"`
	CompactHomeEnabled          bool                  `json:"compact_home_enabled"`
	HideCcsImportButton         bool                  `json:"hide_ccs_import_button"`
	PurchaseSubscriptionEnabled *bool                 `json:"purchase_subscription_enabled"`
	PurchaseSubscriptionURL     *string               `json:"purchase_subscription_url"`
	TableDefaultPageSize        int                   `json:"table_default_page_size"`
	TablePageSizeOptions        []int                 `json:"table_page_size_options"`
	CustomMenuItems             *[]dto.CustomMenuItem `json:"custom_menu_items"`
	CustomEndpoints             *[]dto.CustomEndpoint `json:"custom_endpoints"`

	// 默认配置
	DefaultConcurrency                        int                               `json:"default_concurrency"`
	DefaultBalance                            float64                           `json:"default_balance"`
	AffiliateRebateRate                       *float64                          `json:"affiliate_rebate_rate"`
	AffiliateRebateFreezeHours                *int                              `json:"affiliate_rebate_freeze_hours"`
	AffiliateRebateDurationDays               *int                              `json:"affiliate_rebate_duration_days"`
	AffiliateRebatePerInviteeCap              *float64                          `json:"affiliate_rebate_per_invitee_cap"`
	AdminRechargeRebateEnabled                *bool                             `json:"affiliate_admin_recharge_enabled"`
	DefaultUserRPMLimit                       int                               `json:"default_user_rpm_limit"`
	DefaultSubscriptions                      []dto.DefaultSubscriptionSetting  `json:"default_subscriptions"`
	AuthSourceDefaultEmailBalance             *float64                          `json:"auth_source_default_email_balance"`
	AuthSourceDefaultEmailConcurrency         *int                              `json:"auth_source_default_email_concurrency"`
	AuthSourceDefaultEmailSubscriptions       *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_email_subscriptions"`
	AuthSourceDefaultEmailGrantOnSignup       *bool                             `json:"auth_source_default_email_grant_on_signup"`
	AuthSourceDefaultEmailGrantOnFirstBind    *bool                             `json:"auth_source_default_email_grant_on_first_bind"`
	AuthSourceDefaultLinuxDoBalance           *float64                          `json:"auth_source_default_linuxdo_balance"`
	AuthSourceDefaultLinuxDoConcurrency       *int                              `json:"auth_source_default_linuxdo_concurrency"`
	AuthSourceDefaultLinuxDoSubscriptions     *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_linuxdo_subscriptions"`
	AuthSourceDefaultLinuxDoGrantOnSignup     *bool                             `json:"auth_source_default_linuxdo_grant_on_signup"`
	AuthSourceDefaultLinuxDoGrantOnFirstBind  *bool                             `json:"auth_source_default_linuxdo_grant_on_first_bind"`
	AuthSourceDefaultOIDCBalance              *float64                          `json:"auth_source_default_oidc_balance"`
	AuthSourceDefaultOIDCConcurrency          *int                              `json:"auth_source_default_oidc_concurrency"`
	AuthSourceDefaultOIDCSubscriptions        *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_oidc_subscriptions"`
	AuthSourceDefaultOIDCGrantOnSignup        *bool                             `json:"auth_source_default_oidc_grant_on_signup"`
	AuthSourceDefaultOIDCGrantOnFirstBind     *bool                             `json:"auth_source_default_oidc_grant_on_first_bind"`
	AuthSourceDefaultWeChatBalance            *float64                          `json:"auth_source_default_wechat_balance"`
	AuthSourceDefaultWeChatConcurrency        *int                              `json:"auth_source_default_wechat_concurrency"`
	AuthSourceDefaultWeChatSubscriptions      *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_wechat_subscriptions"`
	AuthSourceDefaultWeChatGrantOnSignup      *bool                             `json:"auth_source_default_wechat_grant_on_signup"`
	AuthSourceDefaultWeChatGrantOnFirstBind   *bool                             `json:"auth_source_default_wechat_grant_on_first_bind"`
	AuthSourceDefaultGitHubBalance            *float64                          `json:"auth_source_default_github_balance"`
	AuthSourceDefaultGitHubConcurrency        *int                              `json:"auth_source_default_github_concurrency"`
	AuthSourceDefaultGitHubSubscriptions      *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_github_subscriptions"`
	AuthSourceDefaultGitHubGrantOnSignup      *bool                             `json:"auth_source_default_github_grant_on_signup"`
	AuthSourceDefaultGitHubGrantOnFirstBind   *bool                             `json:"auth_source_default_github_grant_on_first_bind"`
	AuthSourceDefaultGoogleBalance            *float64                          `json:"auth_source_default_google_balance"`
	AuthSourceDefaultGoogleConcurrency        *int                              `json:"auth_source_default_google_concurrency"`
	AuthSourceDefaultGoogleSubscriptions      *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_google_subscriptions"`
	AuthSourceDefaultGoogleGrantOnSignup      *bool                             `json:"auth_source_default_google_grant_on_signup"`
	AuthSourceDefaultGoogleGrantOnFirstBind   *bool                             `json:"auth_source_default_google_grant_on_first_bind"`
	AuthSourceDefaultDingTalkBalance          *float64                          `json:"auth_source_default_dingtalk_balance"`
	AuthSourceDefaultDingTalkConcurrency      *int                              `json:"auth_source_default_dingtalk_concurrency"`
	AuthSourceDefaultDingTalkSubscriptions    *[]dto.DefaultSubscriptionSetting `json:"auth_source_default_dingtalk_subscriptions"`
	AuthSourceDefaultDingTalkGrantOnSignup    *bool                             `json:"auth_source_default_dingtalk_grant_on_signup"`
	AuthSourceDefaultDingTalkGrantOnFirstBind *bool                             `json:"auth_source_default_dingtalk_grant_on_first_bind"`
	ForceEmailOnThirdPartySignup              *bool                             `json:"force_email_on_third_party_signup"`

	// Model fallback configuration
	EnableModelFallback      bool   `json:"enable_model_fallback"`
	FallbackModelAnthropic   string `json:"fallback_model_anthropic"`
	FallbackModelOpenAI      string `json:"fallback_model_openai"`
	FallbackModelGemini      string `json:"fallback_model_gemini"`
	FallbackModelAntigravity string `json:"fallback_model_antigravity"`

	// Identity patch configuration (Claude -> Gemini)
	EnableIdentityPatch bool   `json:"enable_identity_patch"`
	IdentityPatchPrompt string `json:"identity_patch_prompt"`

	// Ops monitoring (vNext)
	OpsMonitoringEnabled         *bool   `json:"ops_monitoring_enabled"`
	OpsRealtimeMonitoringEnabled *bool   `json:"ops_realtime_monitoring_enabled"`
	OpsQueryModeDefault          *string `json:"ops_query_mode_default"`
	OpsMetricsIntervalSeconds    *int    `json:"ops_metrics_interval_seconds"`

	MinClaudeCodeVersion string `json:"min_claude_code_version"`
	MaxClaudeCodeVersion string `json:"max_claude_code_version"`

	// 分组隔离
	AllowUngroupedKeyScheduling            bool  `json:"allow_ungrouped_key_scheduling"`
	SchedulerV2Enabled                     *bool `json:"scheduler_v2_enabled"`
	SchedulerV2CandidateLimit              *int  `json:"scheduler_v2_candidate_limit"`
	SchedulerV2ScanLimit                   *int  `json:"scheduler_v2_scan_limit"`
	RequestPriorityAdmissionEnabled        *bool `json:"request_priority_admission_enabled"`
	RequestPriorityPendingLimitPerInstance *int  `json:"request_priority_pending_limit_per_instance"`
	RequestPriorityPendingMiBPerInstance   *int  `json:"request_priority_pending_mib_per_instance"`

	// Backend Mode
	BackendModeEnabled bool `json:"backend_mode_enabled"`

	// Performance settings
	StreamModePerformanceEnabled   *bool `json:"stream_mode_performance_enabled"`
	OpenAIWSModeRouterV2Enabled    *bool `json:"openai_ws_mode_router_v2_enabled"`
	OpenAIVisibleOutputTTFTEnabled *bool `json:"openai_visible_output_ttft_enabled"`

	// Gateway forwarding behavior
	EnableFingerprintUnification           *bool   `json:"enable_fingerprint_unification"`
	EnableMetadataPassthrough              *bool   `json:"enable_metadata_passthrough"`
	EnableCCHSigning                       *bool   `json:"enable_cch_signing"`
	EnableClaudeOAuthSystemPromptInjection *bool   `json:"enable_claude_oauth_system_prompt_injection"`
	ClaudeOAuthSystemPrompt                *string `json:"claude_oauth_system_prompt"`
	ClaudeOAuthSystemPromptBlocks          *string `json:"claude_oauth_system_prompt_blocks"`
	EnableAnthropicCacheTTL1hInjection     *bool   `json:"enable_anthropic_cache_ttl_1h_injection"`
	RewriteMessageCacheControl             *bool   `json:"rewrite_message_cache_control"`
	EnableClientDatelineNormalization      *bool   `json:"enable_client_dateline_normalization"`
	AntigravityUserAgentVersion            *string `json:"antigravity_user_agent_version"`
	OpenAICodexUserAgent                   *string `json:"openai_codex_user_agent"`
	OpenAICodexClientVersion               *string `json:"openai_codex_client_version"`
	OpenAICodexVersionAutoSyncEnabled      *bool   `json:"openai_codex_version_auto_sync_enabled"`

	// codex_cli_only 加固（global-only）
	MinCodexVersion                      string `json:"min_codex_version"`
	MaxCodexVersion                      string `json:"max_codex_version"`
	CodexCLIOnlyBlacklist                string `json:"codex_cli_only_blacklist"`
	CodexCLIOnlyWhitelist                string `json:"codex_cli_only_whitelist"`
	CodexCLIOnlyAllowAppServerClients    *bool  `json:"codex_cli_only_allow_app_server_clients"`
	CodexCLIOnlyEngineFingerprintSignals string `json:"codex_cli_only_engine_fingerprint_signals"`

	// Payment visible method routing
	PaymentVisibleMethodAlipaySource  *string `json:"payment_visible_method_alipay_source"`
	PaymentVisibleMethodWxpaySource   *string `json:"payment_visible_method_wxpay_source"`
	PaymentVisibleMethodAlipayEnabled *bool   `json:"payment_visible_method_alipay_enabled"`
	PaymentVisibleMethodWxpayEnabled  *bool   `json:"payment_visible_method_wxpay_enabled"`

	// OpenAI account scheduling
	OpenAILowUpstreamRatePriorityEnabled               *bool    `json:"openai_low_upstream_rate_priority_enabled"`
	OpenAIOAuthSchedulingRateMultiplier                *float64 `json:"openai_oauth_scheduling_rate_multiplier"`
	OpenAIContentSessionBurstBalanceEnabled            *bool    `json:"openai_content_session_burst_balance_enabled"`
	OpenAIAdvancedSchedulerEnabled                     *bool    `json:"openai_advanced_scheduler_enabled"`
	OpenAIAdvancedSchedulerStickyWeightedEnabled       *bool    `json:"openai_advanced_scheduler_sticky_weighted_enabled"`
	OpenAIAdvancedSchedulerSubscriptionPriorityEnabled *bool    `json:"openai_advanced_scheduler_subscription_priority_enabled"`
	OpenAIAdvancedSchedulerLBTopK                      *string  `json:"openai_advanced_scheduler_lb_top_k"`
	OpenAIAdvancedSchedulerWeightPriority              *string  `json:"openai_advanced_scheduler_weight_priority"`
	OpenAIAdvancedSchedulerWeightLoad                  *string  `json:"openai_advanced_scheduler_weight_load"`
	OpenAIAdvancedSchedulerWeightQueue                 *string  `json:"openai_advanced_scheduler_weight_queue"`
	OpenAIAdvancedSchedulerWeightErrorRate             *string  `json:"openai_advanced_scheduler_weight_error_rate"`
	OpenAIAdvancedSchedulerWeightTTFT                  *string  `json:"openai_advanced_scheduler_weight_ttft"`
	OpenAIAdvancedSchedulerWeightReset                 *string  `json:"openai_advanced_scheduler_weight_reset"`
	OpenAIAdvancedSchedulerWeightQuotaHeadroom         *string  `json:"openai_advanced_scheduler_weight_quota_headroom"`
	OpenAIAdvancedSchedulerWeightUpstreamCost          *string  `json:"openai_advanced_scheduler_weight_upstream_cost"`
	OpenAIAdvancedSchedulerWeightPreviousResponse      *string  `json:"openai_advanced_scheduler_weight_previous_response"`
	OpenAIAdvancedSchedulerWeightSessionSticky         *string  `json:"openai_advanced_scheduler_weight_session_sticky"`

	// 余额不足提醒
	BalanceLowNotifyEnabled         *bool                   `json:"balance_low_notify_enabled"`
	BalanceLowNotifyThreshold       *float64                `json:"balance_low_notify_threshold"`
	BalanceLowNotifyRechargeURL     *string                 `json:"balance_low_notify_recharge_url"`
	SubscriptionExpiryNotifyEnabled *bool                   `json:"subscription_expiry_notify_enabled"`
	AccountQuotaNotifyEnabled       *bool                   `json:"account_quota_notify_enabled"`
	AccountQuotaNotifyEmails        *[]dto.NotifyEmailEntry `json:"account_quota_notify_emails"`

	// Payment configuration (integrated into settings, full replace)
	PaymentEnabled                   *bool    `json:"payment_enabled"`
	PaymentMinAmount                 *float64 `json:"payment_min_amount"`
	PaymentMaxAmount                 *float64 `json:"payment_max_amount"`
	PaymentDailyLimit                *float64 `json:"payment_daily_limit"`
	PaymentOrderTimeoutMin           *int     `json:"payment_order_timeout_minutes"`
	PaymentMaxPendingOrders          *int     `json:"payment_max_pending_orders"`
	PaymentEnabledTypes              []string `json:"payment_enabled_types"`
	PaymentBalanceDisabled           *bool    `json:"payment_balance_disabled"`
	PaymentBalanceRechargeMultiplier *float64 `json:"payment_balance_recharge_multiplier"`
	PaymentSubscriptionUSDToCNYRate  *float64 `json:"payment_subscription_usd_to_cny_rate"`
	PaymentRechargeFeeRate           *float64 `json:"payment_recharge_fee_rate"`
	PaymentLoadBalanceStrat          *string  `json:"payment_load_balance_strategy"`
	PaymentProductNamePrefix         *string  `json:"payment_product_name_prefix"`
	PaymentProductNameSuffix         *string  `json:"payment_product_name_suffix"`
	PaymentHelpImageURL              *string  `json:"payment_help_image_url"`
	PaymentHelpText                  *string  `json:"payment_help_text"`

	// Cancel rate limit
	PaymentCancelRateLimitEnabled *bool   `json:"payment_cancel_rate_limit_enabled"`
	PaymentCancelRateLimitMax     *int    `json:"payment_cancel_rate_limit_max"`
	PaymentCancelRateLimitWindow  *int    `json:"payment_cancel_rate_limit_window"`
	PaymentCancelRateLimitUnit    *string `json:"payment_cancel_rate_limit_unit"`
	PaymentCancelRateLimitMode    *string `json:"payment_cancel_rate_limit_window_mode"`

	// Force Alipay mobile clients to use QR code payment instead of mobile redirect
	PaymentAlipayForceQRCode *bool `json:"payment_alipay_force_qrcode"`
	// Use Alipay face-to-face precreate and an app deep link on mobile clients.
	PaymentAlipayMobilePrecreateDeepLink *bool `json:"payment_alipay_mobile_precreate_deep_link"`

	// Channel Monitor feature switch
	ChannelMonitorEnabled                *bool   `json:"channel_monitor_enabled"`
	ChannelMonitorDefaultIntervalSeconds *int    `json:"channel_monitor_default_interval_seconds"`
	ChannelMonitorLatencyUnit            *string `json:"channel_monitor_latency_unit"`
	ChannelMonitorPublicShareEnabled     *bool   `json:"channel_monitor_public_share_enabled"`
	ChannelMonitorPublicShareRequireAuth *bool   `json:"channel_monitor_public_share_require_auth"`

	// Available Channels feature switch (user-facing)
	AvailableChannelsEnabled *bool `json:"available_channels_enabled"`

	// Support Chat feature switch
	SupportChatEnabled *bool `json:"support_chat_enabled"`

	// Model Plaza feature switch and public-page description
	ModelPlazaEnabled          *bool   `json:"model_plaza_enabled"`
	ModelPlazaRequireAuth      *bool   `json:"model_plaza_require_auth"`
	ModelPlazaAutoPublicModels *bool   `json:"model_plaza_auto_public_models"`
	ModelPlazaDescription      *string `json:"model_plaza_description"`

	// Affiliate (邀请返利) feature switch
	AffiliateEnabled *bool `json:"affiliate_enabled"`

	// 风控中心功能开关
	RiskControlEnabled *bool `json:"risk_control_enabled"`

	// cyber 会话屏蔽开关 + TTL
	CyberSessionBlockEnabled    *bool `json:"cyber_session_block_enabled"`
	CyberSessionBlockTTLSeconds *int  `json:"cyber_session_block_ttl_seconds"`

	// OpenAI fast/flex policy (optional, only updated when provided)
	OpenAIFastPolicySettings *dto.OpenAIFastPolicySettings `json:"openai_fast_policy_settings,omitempty"`

	// 系统全局 platform quota 默认值（整体替换语义：nil = 不修改，non-nil = 整体覆盖）。
	DefaultPlatformQuotas map[string]*service.DefaultPlatformQuotaSetting `json:"default_platform_quotas"`

	// auth-source 层 platform quota 覆盖（override 语义：nil = 不修改，non-nil = 整体覆盖该 source 的 quota 配置）。
	AuthSourceEmailPlatformQuotas    map[string]*service.DefaultPlatformQuotaSetting `json:"auth_source_default_email_platform_quotas"`
	AuthSourceLinuxDoPlatformQuotas  map[string]*service.DefaultPlatformQuotaSetting `json:"auth_source_default_linuxdo_platform_quotas"`
	AuthSourceOIDCPlatformQuotas     map[string]*service.DefaultPlatformQuotaSetting `json:"auth_source_default_oidc_platform_quotas"`
	AuthSourceWeChatPlatformQuotas   map[string]*service.DefaultPlatformQuotaSetting `json:"auth_source_default_wechat_platform_quotas"`
	AuthSourceGitHubPlatformQuotas   map[string]*service.DefaultPlatformQuotaSetting `json:"auth_source_default_github_platform_quotas"`
	AuthSourceGooglePlatformQuotas   map[string]*service.DefaultPlatformQuotaSetting `json:"auth_source_default_google_platform_quotas"`
	AuthSourceDingTalkPlatformQuotas map[string]*service.DefaultPlatformQuotaSetting `json:"auth_source_default_dingtalk_platform_quotas"`

	AllowUserViewErrorRequests *bool `json:"allow_user_view_error_requests"`
	AllowUserViewUsageDetails  *bool `json:"allow_user_view_usage_details"`
}

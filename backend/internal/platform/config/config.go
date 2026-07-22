// Package config provides configuration loading, defaults, and validation.
package config

import (
	"os"
	"strings"

	"github.com/spf13/viper"
)

const (
	RunModeStandard = "standard"
	RunModeSimple   = "simple"

	DeploymentModeStandalone    = "standalone"
	DeploymentModeMultiInstance = "multi_instance"

	WorkerModeAuto     = "auto"
	WorkerModeEnabled  = "true"
	WorkerModeDisabled = "false"
)

// 使用量记录队列溢出策略
const (
	UsageRecordOverflowPolicyDrop   = "drop"
	UsageRecordOverflowPolicySample = "sample"
	UsageRecordOverflowPolicySync   = "sync"
)

// DefaultCSPPolicy is the default Content-Security-Policy with nonce support
// __CSP_NONCE__ will be replaced with actual nonce at request time by the SecurityHeaders middleware
const DefaultCSPPolicy = "default-src 'self'; script-src 'self' 'wasm-unsafe-eval' __CSP_NONCE__ https://challenges.cloudflare.com https://www.google.com https://www.gstatic.com https://cdn.jsdelivr.net https://static.cloudflareinsights.com https://*.stripe.com https://static.airwallex.com https://checkout.airwallex.com https://static-demo.airwallex.com https://checkout-demo.airwallex.com; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com https://static.airwallex.com https://checkout.airwallex.com https://static-demo.airwallex.com https://checkout-demo.airwallex.com; img-src 'self' data: blob: https:; font-src 'self' data: https://fonts.gstatic.com; connect-src 'self' https:; worker-src 'self' blob:; frame-src 'self' https://challenges.cloudflare.com https://www.google.com https://recaptcha.google.com https://*.stripe.com https://checkout.airwallex.com https://checkout-demo.airwallex.com; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

// UMQ（用户消息队列）模式常量
const (
	// UMQModeSerialize: 账号级串行锁 + RPM 自适应延迟
	UMQModeSerialize = "serialize"
	// UMQModeThrottle: 仅 RPM 自适应前置延迟，不阻塞并发
	UMQModeThrottle = "throttle"
)

// 连接池隔离策略常量
// 用于控制上游 HTTP 连接池的隔离粒度，影响连接复用和资源消耗
const (
	// ConnectionPoolIsolationProxy: 按代理隔离
	// 同一代理地址共享连接池，适合代理数量少、账户数量多的场景
	ConnectionPoolIsolationProxy = "proxy"
	// ConnectionPoolIsolationAccount: 按账户隔离
	// 每个账户独立连接池，适合账户数量少、需要严格隔离的场景
	ConnectionPoolIsolationAccount = "account"
	// ConnectionPoolIsolationAccountProxy: 按账户+代理组合隔离（默认）
	// 同一账户+代理组合共享连接池，提供最细粒度的隔离
	ConnectionPoolIsolationAccountProxy = "account_proxy"
)

// DefaultUpstreamResponseReadMaxBytes 上游非流式响应体的默认读取上限。
// 128 MB 可容纳 2-3 张 4K PNG（base64 膨胀 33%，单张 4K PNG 最坏约 67MB base64）。
// 可通过 gateway.upstream_response_read_max_bytes 配置项覆盖。
const DefaultUpstreamResponseReadMaxBytes int64 = 128 * 1024 * 1024

type Config struct {
	Deployment              DeploymentConfig              `mapstructure:"deployment"`
	Server                  ServerConfig                  `mapstructure:"server"`
	Log                     LogConfig                     `mapstructure:"log"`
	CORS                    CORSConfig                    `mapstructure:"cors"`
	Security                SecurityConfig                `mapstructure:"security"`
	Billing                 BillingConfig                 `mapstructure:"billing"`
	Turnstile               TurnstileConfig               `mapstructure:"turnstile"`
	Database                DatabaseConfig                `mapstructure:"database"`
	Redis                   RedisConfig                   `mapstructure:"redis"`
	Ops                     OpsConfig                     `mapstructure:"ops"`
	JWT                     JWTConfig                     `mapstructure:"jwt"`
	Totp                    TotpConfig                    `mapstructure:"totp"`
	LinuxDo                 LinuxDoConnectConfig          `mapstructure:"linuxdo_connect"`
	WeChat                  WeChatConnectConfig           `mapstructure:"wechat_connect"`
	OIDC                    OIDCConnectConfig             `mapstructure:"oidc_connect"`
	DingTalk                DingTalkConnectConfig         `mapstructure:"dingtalk_connect"`
	GitHubOAuth             EmailOAuthProviderConfig      `mapstructure:"github_oauth"`
	GoogleOAuth             EmailOAuthProviderConfig      `mapstructure:"google_oauth"`
	Default                 DefaultConfig                 `mapstructure:"default"`
	RateLimit               RateLimitConfig               `mapstructure:"rate_limit"`
	Pricing                 PricingConfig                 `mapstructure:"pricing"`
	Gateway                 GatewayConfig                 `mapstructure:"gateway"`
	APIKeyAuth              APIKeyAuthCacheConfig         `mapstructure:"api_key_auth_cache"`
	SubscriptionCache       SubscriptionCacheConfig       `mapstructure:"subscription_cache"`
	SubscriptionMaintenance SubscriptionMaintenanceConfig `mapstructure:"subscription_maintenance"`
	Dashboard               DashboardCacheConfig          `mapstructure:"dashboard_cache"`
	DashboardAgg            DashboardAggregationConfig    `mapstructure:"dashboard_aggregation"`
	UsageCleanup            UsageCleanupConfig            `mapstructure:"usage_cleanup"`
	Concurrency             ConcurrencyConfig             `mapstructure:"concurrency"`
	TokenRefresh            TokenRefreshConfig            `mapstructure:"token_refresh"`
	RunMode                 string                        `mapstructure:"run_mode" yaml:"run_mode"`
	Timezone                string                        `mapstructure:"timezone"` // e.g. "Asia/Shanghai", "UTC"
	Gemini                  GeminiConfig                  `mapstructure:"gemini"`
	Update                  UpdateConfig                  `mapstructure:"update"`
	Idempotency             IdempotencyConfig             `mapstructure:"idempotency"`
	BatchImage              BatchImageConfig              `mapstructure:"batch_image"`
	ImageStorage            ImageStorageConfig            `mapstructure:"image_storage"`
}

// DeploymentConfig controls cluster identity and cluster-wide scheduled work.
// Every node always serves the complete API and embedded frontend. WorkerEnabled
// is tri-state: auto/true are worker candidates, while false disables only
// cluster-wide scheduled workers on this node.
type DeploymentConfig struct {
	Mode                     string `mapstructure:"mode"`
	NodeName                 string `mapstructure:"node_name"`
	WorkerEnabled            string `mapstructure:"worker_enabled"`
	HeartbeatIntervalSeconds int    `mapstructure:"heartbeat_interval_seconds"`
	StaleAfterSeconds        int    `mapstructure:"stale_after_seconds"`
	TaskLeaseSeconds         int    `mapstructure:"task_lease_seconds"`
}

func (c DeploymentConfig) IsMultiInstance() bool {
	return c.Mode == DeploymentModeMultiInstance
}

func (c DeploymentConfig) WorkerMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.WorkerEnabled))
	switch mode {
	case WorkerModeEnabled, "1", "yes", "on", "enabled":
		return WorkerModeEnabled
	case WorkerModeDisabled, "0", "no", "off", "disabled":
		return WorkerModeDisabled
	default:
		return WorkerModeAuto
	}
}

// WorkerEnabledResolved reports whether this node may contend for distributed
// work. Auto deliberately enables candidacy on every node; the task lease picks
// the actual executor and preserves failover without a manually selected master.
func (c DeploymentConfig) WorkerEnabledResolved() bool {
	return c.WorkerMode() != WorkerModeDisabled
}

type LogConfig struct {
	Level           string            `mapstructure:"level"`
	Format          string            `mapstructure:"format"`
	ServiceName     string            `mapstructure:"service_name"`
	Environment     string            `mapstructure:"env"`
	Caller          bool              `mapstructure:"caller"`
	StacktraceLevel string            `mapstructure:"stacktrace_level"`
	Output          LogOutputConfig   `mapstructure:"output"`
	Rotation        LogRotationConfig `mapstructure:"rotation"`
	Sampling        LogSamplingConfig `mapstructure:"sampling"`
}

type LogOutputConfig struct {
	ToStdout bool   `mapstructure:"to_stdout"`
	ToFile   bool   `mapstructure:"to_file"`
	FilePath string `mapstructure:"file_path"`
}

type LogRotationConfig struct {
	MaxSizeMB  int  `mapstructure:"max_size_mb"`
	MaxBackups int  `mapstructure:"max_backups"`
	MaxAgeDays int  `mapstructure:"max_age_days"`
	Compress   bool `mapstructure:"compress"`
	LocalTime  bool `mapstructure:"local_time"`
}

type LogSamplingConfig struct {
	Enabled    bool `mapstructure:"enabled"`
	Initial    int  `mapstructure:"initial"`
	Thereafter int  `mapstructure:"thereafter"`
}

type GeminiConfig struct {
	OAuth GeminiOAuthConfig `mapstructure:"oauth"`
	Quota GeminiQuotaConfig `mapstructure:"quota"`
}

type GeminiOAuthConfig struct {
	ClientID     string `mapstructure:"client_id"`
	ClientSecret string `mapstructure:"client_secret"`
	Scopes       string `mapstructure:"scopes"`
}

type GeminiQuotaConfig struct {
	Tiers  map[string]GeminiTierQuotaConfig `mapstructure:"tiers"`
	Policy string                           `mapstructure:"policy"`
}

type GeminiTierQuotaConfig struct {
	ProRPD          *int64 `mapstructure:"pro_rpd" json:"pro_rpd"`
	FlashRPD        *int64 `mapstructure:"flash_rpd" json:"flash_rpd"`
	CooldownMinutes *int   `mapstructure:"cooldown_minutes" json:"cooldown_minutes"`
}

type UpdateConfig struct {
	// ProxyURL 用于访问 GitHub 的代理地址
	// 支持 http/https/socks5/socks5h 协议
	// 例如: "http://127.0.0.1:7890", "socks5://127.0.0.1:1080"
	ProxyURL string `mapstructure:"proxy_url"`
}

type IdempotencyConfig struct {
	// ObserveOnly 为 true 时处于观察期：未携带 Idempotency-Key 的请求继续放行。
	ObserveOnly bool `mapstructure:"observe_only"`
	// DefaultTTLSeconds 关键写接口的幂等记录默认 TTL（秒）。
	DefaultTTLSeconds int `mapstructure:"default_ttl_seconds"`
	// SystemOperationTTLSeconds 系统操作接口的幂等记录 TTL（秒）。
	SystemOperationTTLSeconds int `mapstructure:"system_operation_ttl_seconds"`
	// ProcessingTimeoutSeconds processing 状态锁超时（秒）。
	ProcessingTimeoutSeconds int `mapstructure:"processing_timeout_seconds"`
	// FailedRetryBackoffSeconds 失败退避窗口（秒）。
	FailedRetryBackoffSeconds int `mapstructure:"failed_retry_backoff_seconds"`
	// MaxStoredResponseLen 持久化响应体最大长度（字节）。
	MaxStoredResponseLen int `mapstructure:"max_stored_response_len"`
	// CleanupIntervalSeconds 过期记录清理周期（秒）。
	CleanupIntervalSeconds int `mapstructure:"cleanup_interval_seconds"`
	// CleanupBatchSize 每次清理的最大记录数。
	CleanupBatchSize int `mapstructure:"cleanup_batch_size"`
}

type BatchImageConfig struct {
	Enabled                           bool   `mapstructure:"enabled"`
	MaxItemsPerJobDefault             int    `mapstructure:"max_items_per_job_default"`
	MaxItemsPerJobTrial               int    `mapstructure:"max_items_per_job_trial"`
	MaxOutputImagesPerJob             int    `mapstructure:"max_output_images_per_job"`
	MaxOutputImagesPerItem            int    `mapstructure:"max_output_images_per_item"`
	MaxPromptCharsPerItem             int    `mapstructure:"max_prompt_chars_per_item"`
	MaxReferenceImagesPerJob          int    `mapstructure:"max_reference_images_per_job"`
	MaxReferenceInlineBytesPerJob     int    `mapstructure:"max_reference_inline_bytes_per_job"`
	DefaultResponseMimeType           string `mapstructure:"default_response_mime_type"`
	DefaultImageSize                  string `mapstructure:"default_image_size"`
	MaxDownloadItemsZip               int    `mapstructure:"max_download_items_zip"`
	MaxDownloadBytesPerRequest        int64  `mapstructure:"max_download_bytes_per_request"`
	MaxDownloadDurationSeconds        int    `mapstructure:"max_download_duration_seconds"`
	MaxDownloadConcurrencyPerUser     int    `mapstructure:"max_download_concurrency_per_user"`
	InputRetentionAfterTerminalHours  int    `mapstructure:"input_retention_after_terminal_hours"`
	OutputRetentionAfterTerminalHours int    `mapstructure:"output_retention_after_terminal_hours"`
	OutputRetentionMaxDays            int    `mapstructure:"output_retention_max_days"`
	CleanupIntervalMinutes            int    `mapstructure:"cleanup_interval_minutes"`
	CleanupBatchSize                  int    `mapstructure:"cleanup_batch_size"`
	QueueEnabled                      bool   `mapstructure:"queue_enabled"`
	QueueReadyKey                     string `mapstructure:"queue_ready_key"`
	QueueDelayedKey                   string `mapstructure:"queue_delayed_key"`
	QueueActiveKey                    string `mapstructure:"queue_active_key"`
	InflightKeyPrefix                 string `mapstructure:"inflight_key_prefix"`
	LockKeyPrefix                     string `mapstructure:"lock_key_prefix"`
	IdempotencyKeyPrefix              string `mapstructure:"idempotency_key_prefix"`
	InflightTTLSeconds                int    `mapstructure:"inflight_ttl_seconds"`
	JobLockTTLSeconds                 int    `mapstructure:"job_lock_ttl_seconds"`
	DefaultRequeueDelaySeconds        int    `mapstructure:"default_requeue_delay_seconds"`
	ErrorRetryDelaySeconds            int    `mapstructure:"error_retry_delay_seconds"`
	LockConflictDelaySeconds          int    `mapstructure:"lock_conflict_delay_seconds"`
	StaleActiveAfterSeconds           int    `mapstructure:"stale_active_after_seconds"`
	DelayedMoverIntervalSeconds       int    `mapstructure:"delayed_mover_interval_seconds"`
	RecoveryIntervalSeconds           int    `mapstructure:"recovery_interval_seconds"`
	DelayedMoveLimit                  int    `mapstructure:"delayed_move_limit"`
	RecoverLimit                      int    `mapstructure:"recover_limit"`
	VertexEnabled                     bool   `mapstructure:"vertex_enabled"`
	VertexProjectID                   string `mapstructure:"vertex_project_id"`
	VertexLocation                    string `mapstructure:"vertex_location"`
	// VertexManagedGCSBucket is a server-owned bucket for batch JSONL input/output.
	// Disable Cloud Storage soft delete on this bucket to avoid retaining deleted batch objects.
	VertexManagedGCSBucket       string `mapstructure:"vertex_managed_gcs_bucket"`
	VertexManagedGCSPrefix       string `mapstructure:"vertex_managed_gcs_prefix"`
	VertexInputRetentionHours    int    `mapstructure:"vertex_input_retention_hours"`
	VertexOutputRetentionHours   int    `mapstructure:"vertex_output_retention_hours"`
	VertexBatchPredictionBaseURL string `mapstructure:"vertex_batch_prediction_base_url"`
	VertexGCSBaseURL             string `mapstructure:"vertex_gcs_base_url"`
}

// ImageStorageConfig 配置异步图片任务结果上传的 S3 兼容对象存储。
// Enabled 同时作为异步图片任务功能的总开关：未启用或未配置完整凭证时，
// 异步生图接口整体禁用，避免把上游返回的大 base64 结果塞进 Redis。
type ImageStorageConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	Endpoint        string `mapstructure:"endpoint"` // e.g. https://<account_id>.r2.cloudflarestorage.com
	Region          string `mapstructure:"region"`   // R2 用 "auto"
	Bucket          string `mapstructure:"bucket"`
	AccessKeyID     string `mapstructure:"access_key_id"`
	SecretAccessKey string `mapstructure:"secret_access_key"`
	Prefix          string `mapstructure:"prefix"`               // S3 key 前缀，如 "images/"
	ForcePathStyle  bool   `mapstructure:"force_path_style"`     // MinIO/路径风格桶
	PublicBaseURL   string `mapstructure:"public_base_url"`      // 配了则返回 public_base_url/key 直链；否则 presigned
	PresignExpiry   int    `mapstructure:"presign_expiry_hours"` // public_base_url 为空时的 presigned 过期时长(小时)
	MaxDownloadByte int64  `mapstructure:"max_download_bytes"`   // 下载上游 url 图片的字节上限
	MaxInFlight     int    `mapstructure:"max_in_flight"`        // 单实例同时执行的异步生图任务上限
}

// IsConfigured 检查对象存储必要字段是否已配置
func (c *ImageStorageConfig) IsConfigured() bool {
	return c.Bucket != "" && c.AccessKeyID != "" && c.SecretAccessKey != ""
}

// Active 返回异步图片任务是否可用：开关打开且凭证齐全
func (c *ImageStorageConfig) Active() bool {
	return c.Enabled && c.IsConfigured()
}

// MissingCredentialKeys 返回 IsConfigured 所缺的配置键名。
// 用于启动日志：只说"凭证不完整"会让运维以为自己漏填了，而实际可能是值填了却没被读到。
func (c *ImageStorageConfig) MissingCredentialKeys() []string {
	var missing []string
	if c.Bucket == "" {
		missing = append(missing, "image_storage.bucket")
	}
	if c.AccessKeyID == "" {
		missing = append(missing, "image_storage.access_key_id")
	}
	if c.SecretAccessKey == "" {
		missing = append(missing, "image_storage.secret_access_key")
	}
	return missing
}

type LinuxDoConnectConfig struct {
	Enabled             bool   `mapstructure:"enabled"`
	ClientID            string `mapstructure:"client_id"`
	ClientSecret        string `mapstructure:"client_secret"`
	AuthorizeURL        string `mapstructure:"authorize_url"`
	TokenURL            string `mapstructure:"token_url"`
	UserInfoURL         string `mapstructure:"userinfo_url"`
	Scopes              string `mapstructure:"scopes"`
	RedirectURL         string `mapstructure:"redirect_url"`          // 后端回调地址（需在提供方后台登记）
	FrontendRedirectURL string `mapstructure:"frontend_redirect_url"` // 前端接收 token 的路由（默认：/auth/linuxdo/callback）
	TokenAuthMethod     string `mapstructure:"token_auth_method"`     // client_secret_post / client_secret_basic / none
	UsePKCE             bool   `mapstructure:"use_pkce"`

	// 可选：用于从 userinfo JSON 中提取字段的 gjson 路径。
	// 为空时，服务端会尝试一组常见字段名。
	UserInfoEmailPath    string `mapstructure:"userinfo_email_path"`
	UserInfoIDPath       string `mapstructure:"userinfo_id_path"`
	UserInfoUsernamePath string `mapstructure:"userinfo_username_path"`
}

type WeChatConnectConfig struct {
	Enabled             bool   `mapstructure:"enabled"`
	AppID               string `mapstructure:"app_id"`
	AppSecret           string `mapstructure:"app_secret"`
	OpenAppID           string `mapstructure:"open_app_id"`
	OpenAppSecret       string `mapstructure:"open_app_secret"`
	MPAppID             string `mapstructure:"mp_app_id"`
	MPAppSecret         string `mapstructure:"mp_app_secret"`
	MobileAppID         string `mapstructure:"mobile_app_id"`
	MobileAppSecret     string `mapstructure:"mobile_app_secret"`
	OpenEnabled         bool   `mapstructure:"open_enabled"`
	MPEnabled           bool   `mapstructure:"mp_enabled"`
	MobileEnabled       bool   `mapstructure:"mobile_enabled"`
	Mode                string `mapstructure:"mode"`
	Scopes              string `mapstructure:"scopes"`
	RedirectURL         string `mapstructure:"redirect_url"`
	FrontendRedirectURL string `mapstructure:"frontend_redirect_url"`
}

type OIDCConnectConfig struct {
	Enabled                 bool   `mapstructure:"enabled"`
	ProviderName            string `mapstructure:"provider_name"` // 显示名: "Keycloak" 等
	ClientID                string `mapstructure:"client_id"`
	ClientSecret            string `mapstructure:"client_secret"`
	IssuerURL               string `mapstructure:"issuer_url"`
	DiscoveryURL            string `mapstructure:"discovery_url"`
	AuthorizeURL            string `mapstructure:"authorize_url"`
	TokenURL                string `mapstructure:"token_url"`
	UserInfoURL             string `mapstructure:"userinfo_url"`
	JWKSURL                 string `mapstructure:"jwks_url"`
	Scopes                  string `mapstructure:"scopes"`                // 默认 "openid email profile"
	RedirectURL             string `mapstructure:"redirect_url"`          // 后端回调地址（需在提供方后台登记）
	FrontendRedirectURL     string `mapstructure:"frontend_redirect_url"` // 前端接收 token 的路由（默认：/auth/oidc/callback）
	TokenAuthMethod         string `mapstructure:"token_auth_method"`     // client_secret_post / client_secret_basic / none
	UsePKCE                 bool   `mapstructure:"use_pkce"`
	ValidateIDToken         bool   `mapstructure:"validate_id_token"`
	UsePKCEExplicit         bool   `mapstructure:"-" yaml:"-"`
	ValidateIDTokenExplicit bool   `mapstructure:"-" yaml:"-"`
	AllowedSigningAlgs      string `mapstructure:"allowed_signing_algs"`   // 默认 "RS256,ES256,PS256"
	ClockSkewSeconds        int    `mapstructure:"clock_skew_seconds"`     // 默认 120
	RequireEmailVerified    bool   `mapstructure:"require_email_verified"` // 默认 false

	// 可选：用于从 userinfo JSON 中提取字段的 gjson 路径。
	// 为空时，服务端会尝试一组常见字段名。
	UserInfoEmailPath    string `mapstructure:"userinfo_email_path"`
	UserInfoIDPath       string `mapstructure:"userinfo_id_path"`
	UserInfoUsernamePath string `mapstructure:"userinfo_username_path"`
}

type DingTalkConnectConfig struct {
	Enabled             bool   `mapstructure:"enabled"`
	ClientID            string `mapstructure:"client_id"`
	ClientSecret        string `mapstructure:"client_secret"`
	AuthorizeURL        string `mapstructure:"authorize_url"`
	TokenURL            string `mapstructure:"token_url"`
	UserInfoURL         string `mapstructure:"userinfo_url"`
	Scopes              string `mapstructure:"scopes"`
	RedirectURL         string `mapstructure:"redirect_url"`
	FrontendRedirectURL string `mapstructure:"frontend_redirect_url"`

	// 平台底座 + 业务行为
	DingTalkAppKind string `mapstructure:"dingtalk_app_kind"` // 仅 "internal_app"（V4 fail-closed）
	AppType         string `mapstructure:"app_type"`          // "public" (default) | "internal"

	// Corp 限定（none | internal_only）
	CorpRestrictionPolicy   string `mapstructure:"corp_restriction_policy"`
	InternalCorpID          string `mapstructure:"internal_corp_id"`
	BypassRegistration      bool   `mapstructure:"bypass_registration"`
	SyncCorpEmail           bool   `mapstructure:"sync_corp_email"`
	SyncDisplayName         bool   `mapstructure:"sync_display_name"`
	SyncDept                bool   `mapstructure:"sync_dept"`
	SyncCorpEmailAttrKey    string `mapstructure:"sync_corp_email_attr_key"`
	SyncDisplayNameAttrKey  string `mapstructure:"sync_display_name_attr_key"`
	SyncDeptAttrKey         string `mapstructure:"sync_dept_attr_key"`
	SyncCorpEmailAttrName   string `mapstructure:"sync_corp_email_attr_name"`
	SyncDisplayNameAttrName string `mapstructure:"sync_display_name_attr_name"`
	SyncDeptAttrName        string `mapstructure:"sync_dept_attr_name"`

	// 邮箱 + Username
	RequireEmail            bool   `mapstructure:"require_email"`
	UsernameOverwritePolicy string `mapstructure:"username_overwrite_policy"`

	// Attribute（私有版扩展点；开源版仅声明）
	UsernameAttributeKey         string   `mapstructure:"username_attribute_key"`
	EnableAttributeMatching      bool     `mapstructure:"enable_attribute_matching"`
	EnableAttributeSync          bool     `mapstructure:"enable_attribute_sync"`
	AttributeSyncFields          []string `mapstructure:"attribute_sync_fields"`
	AttributeSyncOverwritePolicy string   `mapstructure:"attribute_sync_overwrite_policy"`
}

type EmailOAuthProviderConfig struct {
	Enabled             bool   `mapstructure:"enabled"`
	ClientID            string `mapstructure:"client_id"`
	ClientSecret        string `mapstructure:"client_secret"`
	AuthorizeURL        string `mapstructure:"authorize_url"`
	TokenURL            string `mapstructure:"token_url"`
	UserInfoURL         string `mapstructure:"userinfo_url"`
	EmailsURL           string `mapstructure:"emails_url"`
	Scopes              string `mapstructure:"scopes"`
	RedirectURL         string `mapstructure:"redirect_url"`
	FrontendRedirectURL string `mapstructure:"frontend_redirect_url"`
}

const (
	defaultWeChatConnectMode             = "open"
	defaultWeChatConnectScopes           = "snsapi_login"
	defaultWeChatConnectFrontendRedirect = "/auth/wechat/callback"
)

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeWeChatConnectMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "mp":
		return "mp"
	case "mobile":
		return "mobile"
	default:
		return defaultWeChatConnectMode
	}
}

func normalizeWeChatConnectStoredMode(openEnabled, mpEnabled, mobileEnabled bool, mode string) string {
	mode = normalizeWeChatConnectMode(mode)
	switch mode {
	case "open":
		if openEnabled {
			return "open"
		}
	case "mp":
		if mpEnabled {
			return "mp"
		}
	case "mobile":
		if mobileEnabled {
			return "mobile"
		}
	}
	switch {
	case openEnabled:
		return "open"
	case mpEnabled:
		return "mp"
	case mobileEnabled:
		return "mobile"
	default:
		return mode
	}
}

func defaultWeChatConnectScopesForMode(mode string) string {
	switch normalizeWeChatConnectMode(mode) {
	case "mp":
		return "snsapi_userinfo"
	case "mobile":
		return ""
	default:
		return defaultWeChatConnectScopes
	}
}

func normalizeWeChatConnectScopes(raw, mode string) string {
	switch normalizeWeChatConnectMode(mode) {
	case "mp":
		switch strings.TrimSpace(raw) {
		case "snsapi_base":
			return "snsapi_base"
		case "snsapi_userinfo":
			return "snsapi_userinfo"
		default:
			return defaultWeChatConnectScopesForMode(mode)
		}
	case "mobile":
		return ""
	default:
		return defaultWeChatConnectScopes
	}
}

func shouldApplyLegacyWeChatEnv(configKey, envKey string) bool {
	if viper.InConfig(configKey) {
		return false
	}
	_, hasNewEnv := os.LookupEnv(envKey)
	return !hasNewEnv
}

func hasExplicitConfigOrEnv(configKey, envKey string) bool {
	if viper.InConfig(configKey) {
		return true
	}
	_, ok := os.LookupEnv(envKey)
	return ok
}

func applyLegacyWeChatConnectEnvCompatibility(cfg *WeChatConnectConfig) {
	if cfg == nil {
		return
	}

	legacyOpenAppID := ""
	if shouldApplyLegacyWeChatEnv("wechat_connect.open_app_id", "WECHAT_CONNECT_OPEN_APP_ID") &&
		shouldApplyLegacyWeChatEnv("wechat_connect.app_id", "WECHAT_CONNECT_APP_ID") {
		legacyOpenAppID = strings.TrimSpace(os.Getenv("WECHAT_OAUTH_OPEN_APP_ID"))
		if legacyOpenAppID != "" {
			cfg.OpenAppID = legacyOpenAppID
		}
	}

	legacyOpenAppSecret := ""
	if shouldApplyLegacyWeChatEnv("wechat_connect.open_app_secret", "WECHAT_CONNECT_OPEN_APP_SECRET") &&
		shouldApplyLegacyWeChatEnv("wechat_connect.app_secret", "WECHAT_CONNECT_APP_SECRET") {
		legacyOpenAppSecret = strings.TrimSpace(os.Getenv("WECHAT_OAUTH_OPEN_APP_SECRET"))
		if legacyOpenAppSecret != "" {
			cfg.OpenAppSecret = legacyOpenAppSecret
		}
	}

	legacyMPAppID := ""
	if shouldApplyLegacyWeChatEnv("wechat_connect.mp_app_id", "WECHAT_CONNECT_MP_APP_ID") &&
		shouldApplyLegacyWeChatEnv("wechat_connect.app_id", "WECHAT_CONNECT_APP_ID") {
		legacyMPAppID = strings.TrimSpace(os.Getenv("WECHAT_OAUTH_MP_APP_ID"))
		if legacyMPAppID != "" {
			cfg.MPAppID = legacyMPAppID
		}
	}

	legacyMPAppSecret := ""
	if shouldApplyLegacyWeChatEnv("wechat_connect.mp_app_secret", "WECHAT_CONNECT_MP_APP_SECRET") &&
		shouldApplyLegacyWeChatEnv("wechat_connect.app_secret", "WECHAT_CONNECT_APP_SECRET") {
		legacyMPAppSecret = strings.TrimSpace(os.Getenv("WECHAT_OAUTH_MP_APP_SECRET"))
		if legacyMPAppSecret != "" {
			cfg.MPAppSecret = legacyMPAppSecret
		}
	}

	if shouldApplyLegacyWeChatEnv("wechat_connect.frontend_redirect_url", "WECHAT_CONNECT_FRONTEND_REDIRECT_URL") {
		if legacyFrontend := strings.TrimSpace(os.Getenv("WECHAT_OAUTH_FRONTEND_REDIRECT_URL")); legacyFrontend != "" {
			cfg.FrontendRedirectURL = legacyFrontend
		}
	}

	hasLegacyOpen := legacyOpenAppID != "" && legacyOpenAppSecret != ""
	hasLegacyMP := legacyMPAppID != "" && legacyMPAppSecret != ""

	if shouldApplyLegacyWeChatEnv("wechat_connect.enabled", "WECHAT_CONNECT_ENABLED") && (hasLegacyOpen || hasLegacyMP) {
		cfg.Enabled = true
	}
	if shouldApplyLegacyWeChatEnv("wechat_connect.open_enabled", "WECHAT_CONNECT_OPEN_ENABLED") && hasLegacyOpen {
		cfg.OpenEnabled = true
	}
	if shouldApplyLegacyWeChatEnv("wechat_connect.mp_enabled", "WECHAT_CONNECT_MP_ENABLED") && hasLegacyMP {
		cfg.MPEnabled = true
	}
	if shouldApplyLegacyWeChatEnv("wechat_connect.mode", "WECHAT_CONNECT_MODE") {
		switch {
		case hasLegacyMP && !hasLegacyOpen:
			cfg.Mode = "mp"
		case hasLegacyOpen:
			cfg.Mode = "open"
		}
	}
	if shouldApplyLegacyWeChatEnv("wechat_connect.scopes", "WECHAT_CONNECT_SCOPES") {
		switch {
		case hasLegacyMP && !hasLegacyOpen:
			cfg.Scopes = defaultWeChatConnectScopesForMode("mp")
		case hasLegacyOpen:
			cfg.Scopes = defaultWeChatConnectScopesForMode("open")
		}
	}
}

func normalizeWeChatConnectConfig(cfg *WeChatConnectConfig) {
	if cfg == nil {
		return
	}

	cfg.AppID = strings.TrimSpace(cfg.AppID)
	cfg.AppSecret = strings.TrimSpace(cfg.AppSecret)
	cfg.OpenAppID = strings.TrimSpace(cfg.OpenAppID)
	cfg.OpenAppSecret = strings.TrimSpace(cfg.OpenAppSecret)
	cfg.MPAppID = strings.TrimSpace(cfg.MPAppID)
	cfg.MPAppSecret = strings.TrimSpace(cfg.MPAppSecret)
	cfg.MobileAppID = strings.TrimSpace(cfg.MobileAppID)
	cfg.MobileAppSecret = strings.TrimSpace(cfg.MobileAppSecret)
	cfg.Mode = normalizeWeChatConnectMode(cfg.Mode)
	cfg.RedirectURL = strings.TrimSpace(cfg.RedirectURL)
	cfg.FrontendRedirectURL = strings.TrimSpace(cfg.FrontendRedirectURL)

	cfg.AppID = firstNonEmptyString(cfg.AppID, cfg.OpenAppID, cfg.MPAppID, cfg.MobileAppID)
	cfg.AppSecret = firstNonEmptyString(cfg.AppSecret, cfg.OpenAppSecret, cfg.MPAppSecret, cfg.MobileAppSecret)
	cfg.OpenAppID = firstNonEmptyString(cfg.OpenAppID, cfg.AppID)
	cfg.OpenAppSecret = firstNonEmptyString(cfg.OpenAppSecret, cfg.AppSecret)
	cfg.MPAppID = firstNonEmptyString(cfg.MPAppID, cfg.AppID)
	cfg.MPAppSecret = firstNonEmptyString(cfg.MPAppSecret, cfg.AppSecret)
	cfg.MobileAppID = firstNonEmptyString(cfg.MobileAppID, cfg.AppID)
	cfg.MobileAppSecret = firstNonEmptyString(cfg.MobileAppSecret, cfg.AppSecret)

	if !cfg.OpenEnabled && !cfg.MPEnabled && !cfg.MobileEnabled && cfg.Enabled {
		switch cfg.Mode {
		case "mp":
			cfg.MPEnabled = true
		case "mobile":
			cfg.MobileEnabled = true
		default:
			cfg.OpenEnabled = true
		}
	}
	cfg.Mode = normalizeWeChatConnectStoredMode(cfg.OpenEnabled, cfg.MPEnabled, cfg.MobileEnabled, cfg.Mode)
	cfg.Scopes = normalizeWeChatConnectScopes(cfg.Scopes, cfg.Mode)
	if cfg.FrontendRedirectURL == "" {
		cfg.FrontendRedirectURL = defaultWeChatConnectFrontendRedirect
	}
}

// TokenRefreshConfig OAuth token自动刷新配置

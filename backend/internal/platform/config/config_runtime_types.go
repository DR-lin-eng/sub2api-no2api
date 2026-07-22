package config

import (
	"sync/atomic"
)

type TokenRefreshConfig struct {
	// 是否启用自动刷新
	Enabled bool `mapstructure:"enabled"`
	// 检查间隔（分钟）
	CheckIntervalMinutes int `mapstructure:"check_interval_minutes"`
	// 提前刷新时间（小时），在token过期前多久开始刷新
	RefreshBeforeExpiryHours float64 `mapstructure:"refresh_before_expiry_hours"`
	// 最大重试次数
	MaxRetries int `mapstructure:"max_retries"`
	// 重试退避基础时间（秒）
	RetryBackoffSeconds int `mapstructure:"retry_backoff_seconds"`
	// 每次从数据库读取的候选账号上限
	CandidatePageSize int `mapstructure:"candidate_page_size"`
	// 每个平台允许的并发刷新数
	ProviderConcurrency int `mapstructure:"provider_concurrency"`
	// 每个平台、每个进程允许的刷新请求速率
	ProviderQPS int `mapstructure:"provider_qps"`
	// 一个周期内连续临时失败达到此值后停止该平台
	ProviderFailureThreshold int `mapstructure:"provider_failure_threshold"`
	// 单次上游刷新尝试的超时（秒）
	AttemptTimeoutSeconds int `mapstructure:"attempt_timeout_seconds"`
	// 单个后台刷新周期的总超时（秒）
	CycleTimeoutSeconds int `mapstructure:"cycle_timeout_seconds"`
}

type PricingConfig struct {
	// 价格数据远程URL（默认使用LiteLLM镜像）
	RemoteURL string `mapstructure:"remote_url"`
	// 哈希校验文件URL
	HashURL string `mapstructure:"hash_url"`
	// 本地数据目录
	DataDir string `mapstructure:"data_dir"`
	// 回退文件路径
	FallbackFile string `mapstructure:"fallback_file"`
	// 更新间隔（小时）
	UpdateIntervalHours int `mapstructure:"update_interval_hours"`
	// 哈希校验间隔（分钟）
	HashCheckIntervalMinutes int `mapstructure:"hash_check_interval_minutes"`
}

type ServerConfig struct {
	Host               string    `mapstructure:"host"`
	Port               int       `mapstructure:"port"`
	Mode               string    `mapstructure:"mode"`                  // debug/release
	EnableServerTiming bool      `mapstructure:"enable_server_timing"`  // Admin UI Server-Timing response header
	FrontendURL        string    `mapstructure:"frontend_url"`          // 前端基础 URL，用于生成邮件中的外部链接
	ReadHeaderTimeout  int       `mapstructure:"read_header_timeout"`   // 读取请求头超时（秒）
	MaxHeaderBytes     int       `mapstructure:"max_header_bytes"`      // 请求头最大字节数（HTTP/2 映射为 header-list 上限）
	IdleTimeout        int       `mapstructure:"idle_timeout"`          // 空闲连接超时（秒）
	TrustedProxies     []string  `mapstructure:"trusted_proxies"`       // 可信代理列表（CIDR/IP）
	MaxRequestBodySize int64     `mapstructure:"max_request_body_size"` // 全局最大请求体限制
	H2C                H2CConfig `mapstructure:"h2c"`                   // HTTP/2 Cleartext 配置
}

// H2CConfig HTTP/2 Cleartext 配置
type H2CConfig struct {
	Enabled                      bool   `mapstructure:"enabled"`                          // 是否启用 H2C
	MaxConcurrentStreams         uint32 `mapstructure:"max_concurrent_streams"`           // 最大并发流数量
	IdleTimeout                  int    `mapstructure:"idle_timeout"`                     // 空闲超时（秒）
	MaxReadFrameSize             int    `mapstructure:"max_read_frame_size"`              // 最大帧大小（字节）
	MaxUploadBufferPerConnection int    `mapstructure:"max_upload_buffer_per_connection"` // 每个连接的上传缓冲区（字节）
	MaxUploadBufferPerStream     int    `mapstructure:"max_upload_buffer_per_stream"`     // 每个流的上传缓冲区（字节）
}

type CORSConfig struct {
	AllowedOrigins   []string `mapstructure:"allowed_origins"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
}

type SecurityConfig struct {
	URLAllowlist                     URLAllowlistConfig   `mapstructure:"url_allowlist"`
	ResponseHeaders                  ResponseHeaderConfig `mapstructure:"response_headers"`
	CSP                              CSPConfig            `mapstructure:"csp"`
	ProxyFallback                    ProxyFallbackConfig  `mapstructure:"proxy_fallback"`
	ProxyProbe                       ProxyProbeConfig     `mapstructure:"proxy_probe"`
	TrustForwardedIPForAPIKeyACL     bool                 `mapstructure:"trust_forwarded_ip_for_api_key_acl"`
	trustForwardedIPForAPIKeyACLLive *atomic.Bool         `mapstructure:"-"`
}

func (c *Config) TrustForwardedIPForAPIKeyACL() bool {
	if c == nil {
		return false
	}
	live := c.Security.trustForwardedIPForAPIKeyACLLive
	if live == nil {
		return c.Security.TrustForwardedIPForAPIKeyACL
	}
	return live.Load()
}

func (c *Config) SetTrustForwardedIPForAPIKeyACL(enabled bool) {
	if c == nil {
		return
	}
	c.Security.TrustForwardedIPForAPIKeyACL = enabled
	if c.Security.trustForwardedIPForAPIKeyACLLive == nil {
		c.Security.trustForwardedIPForAPIKeyACLLive = &atomic.Bool{}
	}
	c.Security.trustForwardedIPForAPIKeyACLLive.Store(enabled)
}

type URLAllowlistConfig struct {
	Enabled           bool     `mapstructure:"enabled"`
	UpstreamHosts     []string `mapstructure:"upstream_hosts"`
	PricingHosts      []string `mapstructure:"pricing_hosts"`
	CRSHosts          []string `mapstructure:"crs_hosts"`
	AllowPrivateHosts bool     `mapstructure:"allow_private_hosts"`
	// 关闭 URL 白名单校验时，是否允许 http URL（默认只允许 https）
	AllowInsecureHTTP bool `mapstructure:"allow_insecure_http"`
}

type ResponseHeaderConfig struct {
	Enabled           bool     `mapstructure:"enabled"`
	AdditionalAllowed []string `mapstructure:"additional_allowed"`
	ForceRemove       []string `mapstructure:"force_remove"`
}

type CSPConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Policy  string `mapstructure:"policy"`
}

type ProxyFallbackConfig struct {
	// AllowDirectOnError 当辅助服务的代理初始化失败时是否允许回退直连。
	// 仅影响以下非 AI 账号连接的辅助服务：
	//   - GitHub Release 更新检查
	//   - 定价数据拉取
	// 不影响 AI 账号网关连接（Claude/OpenAI/Gemini/Antigravity），
	// 这些关键路径的代理失败始终返回错误，不会回退直连。
	// 默认 false：避免因代理配置错误导致服务器真实 IP 泄露。
	AllowDirectOnError bool `mapstructure:"allow_direct_on_error"`
}

type ProxyProbeConfig struct {
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify"` // 已禁用：禁止跳过 TLS 证书验证
}

type BillingConfig struct {
	CircuitBreaker CircuitBreakerConfig    `mapstructure:"circuit_breaker"`
	Queue          UsageBillingQueueConfig `mapstructure:"queue"`
	// MinimumBalanceReserve is the conservative preflight floor for balance billing.
	// Requests in balance mode are rejected when the cached balance is below this
	// amount, even if it is still positive. Set to 0 to keep the legacy balance > 0 gate.
	MinimumBalanceReserve float64 `mapstructure:"minimum_balance_reserve"`
	// UserPlatformQuotaCacheTTLSeconds 用户 × 平台 quota 缓存 TTL（秒），默认 86400=1天，覆盖典型 daily 窗口。
	// 消费点：
	//   - billing_cache_service.cacheWriteWorker 异步累加
	//   - billing_cache_service.checkUserPlatformQuotaEligibility 首次缓存装载
	// 读写两端必须共用同一 TTL，避免缓存生命周期不一致导致 quota 计数漂移。
	UserPlatformQuotaCacheTTLSeconds int `mapstructure:"user_platform_quota_cache_ttl_seconds"`
	// UserPlatformQuotaSentinelTTLSeconds sentinel(无 limit 占位)entry 的 TTL,
	// 显著短于 quota cache 默认 86400s 以控 Redis 内存;默认 3600=1h。
	UserPlatformQuotaSentinelTTLSeconds int `mapstructure:"user_platform_quota_sentinel_ttl_seconds"`
}

// UsageBillingQueueConfig controls the PostgreSQL WAL-backed billing queue.
type UsageBillingQueueConfig struct {
	Enabled               bool `mapstructure:"enabled"`
	ConsumerCount         int  `mapstructure:"consumer_count"`
	MaxConsumerCount      int  `mapstructure:"max_consumer_count"`
	ReadBatchSize         int  `mapstructure:"read_batch_size"`
	ReadBlockMilliseconds int  `mapstructure:"read_block_milliseconds"`
	CommandTimeoutSeconds int  `mapstructure:"command_timeout_seconds"`
	MaxRetryDelaySeconds  int  `mapstructure:"max_retry_delay_seconds"`
}

type CircuitBreakerConfig struct {
	Enabled             bool `mapstructure:"enabled"`
	FailureThreshold    int  `mapstructure:"failure_threshold"`
	ResetTimeoutSeconds int  `mapstructure:"reset_timeout_seconds"`
	HalfOpenRequests    int  `mapstructure:"half_open_requests"`
}

type ConcurrencyConfig struct {
	// PingInterval: 并发等待期间的 SSE ping 间隔（秒）
	PingInterval int `mapstructure:"ping_interval"`
}

type ImageConcurrencyConfig struct {
	// Enabled: 是否启用图片生成独立并发限制，默认关闭以保持现有行为
	Enabled bool `mapstructure:"enabled"`
	// MaxConcurrentRequests: 当前进程允许同时处理的图片生成请求数，0表示不限制
	MaxConcurrentRequests int `mapstructure:"max_concurrent_requests"`
	// OverflowMode: 图片并发达到上限后的处理方式：reject/wait
	OverflowMode string `mapstructure:"overflow_mode"`
	// WaitTimeoutSeconds: overflow_mode=wait 时等待图片并发槽位的超时时间（秒）
	WaitTimeoutSeconds int `mapstructure:"wait_timeout_seconds"`
	// MaxWaitingRequests: overflow_mode=wait 时当前进程允许排队等待的图片请求数
	MaxWaitingRequests int `mapstructure:"max_waiting_requests"`
}

const (
	ImageConcurrencyOverflowModeReject = "reject"
	ImageConcurrencyOverflowModeWait   = "wait"
)

// GatewayConfig API网关相关配置

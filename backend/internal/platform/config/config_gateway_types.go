package config

import (
	"math"
	"time"
)

type GatewayConfig struct {
	// 等待上游响应头的超时时间（秒），0表示无超时
	// 注意：这不影响流式数据传输，只控制等待响应头的时间
	ResponseHeaderTimeout int `mapstructure:"response_header_timeout"`
	// OpenAIResponseHeaderTimeout: OpenAI/Codex 上游等待响应头的超时时间（秒），0表示无超时
	// OpenAI/Codex 请求可能在上游排队较久；默认不使用通用响应头超时截断。
	OpenAIResponseHeaderTimeout int `mapstructure:"openai_response_header_timeout"`
	// OpenAIFirstOutputTimeoutSeconds: native HTTP Responses 首个语义输出超时（秒），0表示禁用。
	OpenAIFirstOutputTimeoutSeconds int `mapstructure:"openai_first_output_timeout_seconds"`
	// OpenAIHighEffortFirstOutputTimeoutSeconds: high/xhigh/max 推理的首个语义输出超时（秒）。
	// 0 表示回退到 OpenAIFirstOutputTimeoutSeconds。
	OpenAIHighEffortFirstOutputTimeoutSeconds int `mapstructure:"openai_high_effort_first_output_timeout_seconds"`
	// OpenAIAPIKeyStreamResponseHeaderTimeout: API-key 流式请求等待上游响应头的时间（秒），0表示无超时。
	// 与非流式生成分离，避免失效渠道无限保留完整请求体，同时不截断耗时较长的非流式生成。
	OpenAIAPIKeyStreamResponseHeaderTimeout int `mapstructure:"openai_apikey_stream_response_header_timeout"`
	// 请求体最大字节数，用于网关请求体大小限制
	MaxBodySize int64 `mapstructure:"max_body_size"`
	// TextMaxBodySize limits endpoints that cannot carry inline image/video payloads.
	TextMaxBodySize int64 `mapstructure:"text_max_body_size"`
	// 非流式上游响应体读取上限（字节），用于防止无界读取导致内存放大
	UpstreamResponseReadMaxBytes int64 `mapstructure:"upstream_response_read_max_bytes"`
	// 代理探测响应体读取上限（字节）
	ProxyProbeResponseReadMaxBytes int64 `mapstructure:"proxy_probe_response_read_max_bytes"`
	// Gemini 上游响应头调试日志开关（默认关闭，避免高频日志开销）
	GeminiDebugResponseHeaders bool `mapstructure:"gemini_debug_response_headers"`
	// ConnectionPoolIsolation: 上游连接池隔离策略（proxy/account/account_proxy）
	ConnectionPoolIsolation string `mapstructure:"connection_pool_isolation"`
	// ForceCodexCLI: 强制将 OpenAI `/v1/responses` 请求按 Codex CLI 处理。
	// 用于网关未透传/改写 User-Agent 时的兼容兜底（默认关闭，避免影响其他客户端）。
	ForceCodexCLI bool `mapstructure:"force_codex_cli"`
	// CodexImageGenerationBridgeEnabled: 是否为 Codex `/v1/responses` 自动注入 image_generation 工具和桥接指令。
	// 默认关闭，避免纯文本 Codex 请求被意外改写；显式携带 image_generation 工具的请求仍按分组能力转发。
	CodexImageGenerationBridgeEnabled bool `mapstructure:"codex_image_generation_bridge_enabled"`
	// ForcedCodexInstructionsTemplateFile: 服务端强制附加到 Codex 顶层 instructions 的模板文件路径。
	// 模板渲染后会直接覆盖最终 instructions；若需要保留客户端 system 转换结果，请在模板中显式引用 {{ .ExistingInstructions }}。
	ForcedCodexInstructionsTemplateFile string `mapstructure:"forced_codex_instructions_template_file"`
	// ForcedCodexInstructionsTemplate: 启动时从模板文件读取并缓存的模板内容。
	// 该字段不直接参与配置反序列化，仅用于请求热路径避免重复读盘。
	ForcedCodexInstructionsTemplate string `mapstructure:"-"`
	// OpenAIPassthroughAllowTimeoutHeaders: OpenAI 透传模式是否放行客户端超时头
	// 关闭（默认）可避免 x-stainless-timeout 等头导致上游提前断流。
	OpenAIPassthroughAllowTimeoutHeaders bool `mapstructure:"openai_passthrough_allow_timeout_headers"`
	// OpenAICompactModel: /responses/compact 上游使用的模型。
	// compact 端点支持模型滞后于普通 /responses 时，可用该配置降级规避上游错误。
	OpenAICompactModel string `mapstructure:"openai_compact_model"`
	// OpenAIWS: OpenAI Responses WebSocket 配置（默认开启，可按需回滚到 HTTP）
	OpenAIWS GatewayOpenAIWSConfig `mapstructure:"openai_ws"`
	// OpenAIScheduler: OpenAI 高级调度器粘性逃逸配置
	OpenAIScheduler GatewayOpenAISchedulerConfig `mapstructure:"openai_scheduler"`
	// OpenAIHTTP2: OpenAI HTTP 上游协议策略（默认启用 HTTP/2，可按代理能力回退 HTTP/1.1）
	OpenAIHTTP2 GatewayOpenAIHTTP2Config `mapstructure:"openai_http2"`
	// ImageConcurrency: 图片生成独立并发限制配置（默认关闭）
	ImageConcurrency ImageConcurrencyConfig `mapstructure:"image_concurrency"`

	// HTTP 上游连接池配置（性能优化：支持高并发场景调优）
	// MaxIdleConns: 所有主机的最大空闲连接总数
	MaxIdleConns int `mapstructure:"max_idle_conns"`
	// MaxIdleConnsPerHost: 每个主机的最大空闲连接数（关键参数，影响连接复用率）
	MaxIdleConnsPerHost int `mapstructure:"max_idle_conns_per_host"`
	// MaxConnsPerHost: 每个主机的最大连接数（包括活跃+空闲），0表示无限制
	MaxConnsPerHost int `mapstructure:"max_conns_per_host"`
	// IdleConnTimeoutSeconds: 空闲连接超时时间（秒）
	IdleConnTimeoutSeconds int `mapstructure:"idle_conn_timeout_seconds"`
	// MaxUpstreamClients: 上游连接池客户端最大缓存数量
	// 当使用连接池隔离策略时，系统会为不同的账户/代理组合创建独立的 HTTP 客户端
	// 此参数限制缓存的客户端数量，超出后会淘汰最久未使用的客户端
	// 建议值：预估的活跃账户数 * 1.2（留有余量）
	MaxUpstreamClients int `mapstructure:"max_upstream_clients"`
	// ClientIdleTTLSeconds: 上游连接池客户端空闲回收阈值（秒）
	// 超过此时间未使用的客户端会被标记为可回收
	// 建议值：根据用户访问频率设置，一般 10-30 分钟
	ClientIdleTTLSeconds int `mapstructure:"client_idle_ttl_seconds"`
	// ConcurrencySlotTTLMinutes: 并发槽位过期时间（分钟）
	// 应大于最长 LLM 请求时间，防止请求完成前槽位过期
	ConcurrencySlotTTLMinutes int `mapstructure:"concurrency_slot_ttl_minutes"`
	// SessionIdleTimeoutMinutes: 会话空闲超时时间（分钟），默认 5 分钟
	// 用于 Anthropic OAuth/SetupToken 账号的会话数量限制功能
	// 空闲超过此时间的会话将被自动释放
	SessionIdleTimeoutMinutes int `mapstructure:"session_idle_timeout_minutes"`

	// StreamDataIntervalTimeout: 流数据间隔超时（秒），0表示禁用
	StreamDataIntervalTimeout int `mapstructure:"stream_data_interval_timeout"`
	// StreamKeepaliveInterval: 流式 keepalive 间隔（秒），0表示禁用
	StreamKeepaliveInterval int `mapstructure:"stream_keepalive_interval"`
	// ImageStreamDataIntervalTimeout: 图片流数据间隔超时（秒），0表示禁用
	ImageStreamDataIntervalTimeout int `mapstructure:"image_stream_data_interval_timeout"`
	// ImageStreamKeepaliveInterval: 图片流式 keepalive 间隔（秒），0表示禁用
	ImageStreamKeepaliveInterval int `mapstructure:"image_stream_keepalive_interval"`
	// ImageNonstreamKeepaliveInterval: 图片非流式 JSON keepalive 间隔（秒），0表示禁用
	ImageNonstreamKeepaliveInterval int `mapstructure:"image_nonstream_keepalive_interval"`
	// MaxLineSize: 上游 SSE 单行最大字节数（0使用默认值）
	MaxLineSize int `mapstructure:"max_line_size"`

	// 是否记录上游错误响应体摘要（避免输出请求内容）
	LogUpstreamErrorBody bool `mapstructure:"log_upstream_error_body"`
	// 上游错误响应体记录最大字节数（超过会截断）
	LogUpstreamErrorBodyMaxBytes int `mapstructure:"log_upstream_error_body_max_bytes"`

	// API-key 账号在客户端未提供 anthropic-beta 时，是否按需自动补齐（默认关闭以保持兼容）
	InjectBetaForAPIKey bool `mapstructure:"inject_beta_for_apikey"`

	// 是否允许对部分 400 错误触发 failover（默认关闭以避免改变语义）
	FailoverOn400 bool `mapstructure:"failover_on_400"`

	// 账户切换最大次数（遇到上游错误时切换到其他账户的次数上限）
	MaxAccountSwitches int `mapstructure:"max_account_switches"`
	// Gemini 账户切换最大次数（Gemini 平台单独配置，因 API 限制更严格）
	MaxAccountSwitchesGemini int `mapstructure:"max_account_switches_gemini"`

	// Antigravity 429 fallback 限流时间（分钟），解析重置时间失败时使用
	AntigravityFallbackCooldownMinutes int `mapstructure:"antigravity_fallback_cooldown_minutes"`

	// Scheduling: 账号调度相关配置
	Scheduling GatewaySchedulingConfig `mapstructure:"scheduling"`

	// TLSFingerprint: TLS指纹伪装配置
	TLSFingerprint TLSFingerprintConfig `mapstructure:"tls_fingerprint"`

	// UsageRecord: 使用量记录异步队列配置（有界队列 + 固定 worker）
	UsageRecord GatewayUsageRecordConfig `mapstructure:"usage_record"`

	// UserGroupRateCacheTTLSeconds: 用户分组倍率热路径缓存 TTL（秒）
	UserGroupRateCacheTTLSeconds int `mapstructure:"user_group_rate_cache_ttl_seconds"`
	// ModelsListCacheTTLSeconds: /v1/models 模型列表短缓存 TTL（秒）
	ModelsListCacheTTLSeconds int `mapstructure:"models_list_cache_ttl_seconds"`

	// UserMessageQueue: 用户消息串行队列配置
	// 对 role:"user" 的真实用户消息实施账号级串行化 + RPM 自适应延迟
	UserMessageQueue UserMessageQueueConfig `mapstructure:"user_message_queue"`
}

// GatewayOpenAIHTTP2Config OpenAI HTTP 上游协议配置。
// 默认启用 HTTP/2；在部分代理不兼容时按策略回退 HTTP/1.1。
type GatewayOpenAIHTTP2Config struct {
	// Enabled: 是否启用 OpenAI HTTP/2 优先策略
	Enabled bool `mapstructure:"enabled"`
	// AllowProxyFallbackToHTTP1: HTTP/HTTPS 代理出现明确 H2 兼容错误时，临时回退 HTTP/1.1
	AllowProxyFallbackToHTTP1 bool `mapstructure:"allow_proxy_fallback_to_http1"`
	// FallbackErrorThreshold: 回退窗口内累计多少次兼容错误后触发回退
	FallbackErrorThreshold int `mapstructure:"fallback_error_threshold"`
	// FallbackWindowSeconds: 统计兼容错误的时间窗口（秒）
	FallbackWindowSeconds int `mapstructure:"fallback_window_seconds"`
	// FallbackTTLSeconds: 触发后回退 HTTP/1.1 的持续时间（秒）
	FallbackTTLSeconds int `mapstructure:"fallback_ttl_seconds"`
}

// UserMessageQueueConfig 用户消息串行队列配置
// 用于 Anthropic OAuth/SetupToken 账号的用户消息串行化发送
type UserMessageQueueConfig struct {
	// Mode: 模式选择
	// "serialize" = 账号级串行锁 + RPM 自适应延迟
	// "throttle" = 仅 RPM 自适应前置延迟，不阻塞并发
	// "" = 禁用（默认）
	Mode string `mapstructure:"mode"`
	// Enabled: 已废弃，仅向后兼容（等同于 mode: "serialize"）
	Enabled bool `mapstructure:"enabled"`
	// LockTTLMs: 串行锁 TTL（毫秒），应大于最长请求时间
	LockTTLMs int `mapstructure:"lock_ttl_ms"`
	// WaitTimeoutMs: 等待获取锁的超时时间（毫秒）
	WaitTimeoutMs int `mapstructure:"wait_timeout_ms"`
	// MinDelayMs: RPM 自适应延迟下限（毫秒）
	MinDelayMs int `mapstructure:"min_delay_ms"`
	// MaxDelayMs: RPM 自适应延迟上限（毫秒）
	MaxDelayMs int `mapstructure:"max_delay_ms"`
	// CleanupIntervalSeconds: 孤儿锁清理间隔（秒），0 表示禁用
	CleanupIntervalSeconds int `mapstructure:"cleanup_interval_seconds"`
}

// WaitTimeout 返回等待超时的 time.Duration
func (c *UserMessageQueueConfig) WaitTimeout() time.Duration {
	if c.WaitTimeoutMs <= 0 {
		return 30 * time.Second
	}
	return time.Duration(c.WaitTimeoutMs) * time.Millisecond
}

// GetEffectiveMode 返回生效的模式
// 注意：Mode 字段已在 load() 中做过白名单校验和规范化，此处无需重复验证
func (c *UserMessageQueueConfig) GetEffectiveMode() string {
	if c.Mode == UMQModeSerialize || c.Mode == UMQModeThrottle {
		return c.Mode
	}
	if c.Enabled {
		return UMQModeSerialize // 向后兼容
	}
	return ""
}

// DefaultOpenAIWSClientFirstMessageTimeoutSeconds preserves the legacy ingress deadline.
const DefaultOpenAIWSClientFirstMessageTimeoutSeconds = 30

// GatewayOpenAIWSConfig OpenAI Responses WebSocket 配置。
// 注意：默认全局开启；如需回滚可使用 force_http 或关闭 enabled。
type GatewayOpenAIWSConfig struct {
	// ModeRouterV2Enabled: 新版 WS mode 路由开关（默认 false；关闭时保持 legacy 行为）
	ModeRouterV2Enabled bool `mapstructure:"mode_router_v2_enabled"`
	// IngressModeDefault: ingress 默认模式（off/ctx_pool/passthrough/http_bridge）
	IngressModeDefault string `mapstructure:"ingress_mode_default"`
	// ClientFirstMessageTimeoutSeconds bounds the total time to read and decompress
	// the first client response.create message after the WebSocket upgrade.
	ClientFirstMessageTimeoutSeconds int `mapstructure:"client_first_message_timeout_seconds"`
	// IngressInterTurnIdleTimeoutSeconds bounds the time a client may remain idle
	// between completed ingress turns. Zero disables this protection.
	IngressInterTurnIdleTimeoutSeconds int `mapstructure:"ingress_inter_turn_idle_timeout_seconds"`
	// MaxIngressConnectionsPerAPIKey bounds live client WebSocket ingress sessions
	// per API key across all instances. Zero disables this protection.
	MaxIngressConnectionsPerAPIKey int `mapstructure:"max_ingress_connections_per_api_key"`
	// Enabled: 全局总开关（默认 true）
	Enabled bool `mapstructure:"enabled"`
	// OAuthEnabled: 是否允许 OpenAI OAuth 账号使用 WS
	OAuthEnabled bool `mapstructure:"oauth_enabled"`
	// APIKeyEnabled: 是否允许 OpenAI API Key 账号使用 WS
	APIKeyEnabled bool `mapstructure:"apikey_enabled"`
	// ForceHTTP: 全局强制 HTTP（用于紧急回滚）
	ForceHTTP bool `mapstructure:"force_http"`
	// AllowStoreRecovery: 允许在 WSv2 下按策略恢复 store=true（默认 false）
	AllowStoreRecovery bool `mapstructure:"allow_store_recovery"`
	// IngressPreviousResponseRecoveryEnabled: ingress 模式收到 previous_response_not_found 时，是否允许自动去掉 previous_response_id 重试一次（默认 true）
	IngressPreviousResponseRecoveryEnabled bool `mapstructure:"ingress_previous_response_recovery_enabled"`
	// StoreDisabledConnMode: store=false 且无可复用会话连接时的建连策略（strict/adaptive/off）
	// - strict: 强制新建连接（隔离优先）
	// - adaptive: 仅在高风险失败后强制新建连接（性能与隔离折中）
	// - off: 不强制新建连接（复用优先）
	StoreDisabledConnMode string `mapstructure:"store_disabled_conn_mode"`
	// StoreDisabledForceNewConn: store=false 且无可复用粘连连接时是否强制新建连接（默认 true，保障会话隔离）
	// 兼容旧配置；当 StoreDisabledConnMode 为空时才生效。
	StoreDisabledForceNewConn bool `mapstructure:"store_disabled_force_new_conn"`
	// PrewarmGenerateEnabled: 是否启用 WSv2 generate=false 预热（默认 false）
	PrewarmGenerateEnabled bool `mapstructure:"prewarm_generate_enabled"`
	// ClientReadLimitBytes: 入站客户端 WS 单帧读取上限。
	ClientReadLimitBytes int64 `mapstructure:"client_read_limit_bytes"`
	// HTTPBridgeEnabled: 首包过大时，保持客户端 WS，改用 HTTP Responses 上游。
	HTTPBridgeEnabled bool `mapstructure:"http_bridge_enabled"`
	// HTTPBridgeThresholdBytes: 触发 HTTP bridge 的入站 WS payload 阈值。
	HTTPBridgeThresholdBytes int64 `mapstructure:"http_bridge_threshold_bytes"`

	// Feature 开关：v2 优先于 v1
	ResponsesWebsockets   bool `mapstructure:"responses_websockets"`
	ResponsesWebsocketsV2 bool `mapstructure:"responses_websockets_v2"`

	// 连接池参数
	MaxConnsPerAccount int `mapstructure:"max_conns_per_account"`
	MinIdlePerAccount  int `mapstructure:"min_idle_per_account"`
	MaxIdlePerAccount  int `mapstructure:"max_idle_per_account"`
	// DynamicMaxConnsByAccountConcurrencyEnabled: 是否按账号并发动态计算连接池上限
	DynamicMaxConnsByAccountConcurrencyEnabled bool `mapstructure:"dynamic_max_conns_by_account_concurrency_enabled"`
	// OAuthMaxConnsFactor: OAuth 账号连接池系数（effective=ceil(concurrency*factor)）
	OAuthMaxConnsFactor float64 `mapstructure:"oauth_max_conns_factor"`
	// APIKeyMaxConnsFactor: API Key 账号连接池系数（effective=ceil(concurrency*factor)）
	APIKeyMaxConnsFactor  float64 `mapstructure:"apikey_max_conns_factor"`
	DialTimeoutSeconds    int     `mapstructure:"dial_timeout_seconds"`
	ReadTimeoutSeconds    int     `mapstructure:"read_timeout_seconds"`
	WriteTimeoutSeconds   int     `mapstructure:"write_timeout_seconds"`
	PoolTargetUtilization float64 `mapstructure:"pool_target_utilization"`
	QueueLimitPerConn     int     `mapstructure:"queue_limit_per_conn"`
	// EventFlushBatchSize: WS 流式写出批量 flush 阈值（事件条数）
	EventFlushBatchSize int `mapstructure:"event_flush_batch_size"`
	// EventFlushIntervalMS: WS 流式写出最大等待时间（毫秒）；0 表示仅按 batch 触发
	EventFlushIntervalMS int `mapstructure:"event_flush_interval_ms"`
	// PrewarmCooldownMS: 连接池预热触发冷却时间（毫秒）
	PrewarmCooldownMS int `mapstructure:"prewarm_cooldown_ms"`
	// FallbackCooldownSeconds: WS 回退冷却窗口，避免 WS/HTTP 抖动；0 表示关闭冷却
	FallbackCooldownSeconds int `mapstructure:"fallback_cooldown_seconds"`
	// RetryBackoffInitialMS: WS 重试初始退避（毫秒）；<=0 表示关闭退避
	RetryBackoffInitialMS int `mapstructure:"retry_backoff_initial_ms"`
	// RetryBackoffMaxMS: WS 重试最大退避（毫秒）
	RetryBackoffMaxMS int `mapstructure:"retry_backoff_max_ms"`
	// RetryJitterRatio: WS 重试退避抖动比例（0-1）
	RetryJitterRatio float64 `mapstructure:"retry_jitter_ratio"`
	// RetryTotalBudgetMS: WS 单次请求重试总预算（毫秒）；0 表示关闭预算限制
	RetryTotalBudgetMS int `mapstructure:"retry_total_budget_ms"`
	// PayloadLogSampleRate: payload_schema 日志采样率（0-1）
	PayloadLogSampleRate float64 `mapstructure:"payload_log_sample_rate"`

	// 账号调度与粘连参数
	LBTopK int `mapstructure:"lb_top_k"`
	// StickySessionTTLSeconds: session_hash -> account_id 粘连 TTL
	StickySessionTTLSeconds int `mapstructure:"sticky_session_ttl_seconds"`
	// SessionHashReadOldFallback: 会话哈希迁移期是否允许“新 key 未命中时回退读旧 SHA-256 key”
	SessionHashReadOldFallback bool `mapstructure:"session_hash_read_old_fallback"`
	// SessionHashDualWriteOld: 会话哈希迁移期是否双写旧 SHA-256 key（短 TTL）
	SessionHashDualWriteOld bool `mapstructure:"session_hash_dual_write_old"`
	// MetadataBridgeEnabled: RequestMetadata 迁移期是否保留旧 ctxkey.* 兼容桥接
	MetadataBridgeEnabled bool `mapstructure:"metadata_bridge_enabled"`
	// StickyResponseIDTTLSeconds: response_id -> account_id 粘连 TTL
	StickyResponseIDTTLSeconds int `mapstructure:"sticky_response_id_ttl_seconds"`
	// StickyPreviousResponseTTLSeconds: 兼容旧键（当新键未设置时回退）
	StickyPreviousResponseTTLSeconds int `mapstructure:"sticky_previous_response_ttl_seconds"`

	SchedulerScoreWeights GatewayOpenAIWSSchedulerScoreWeights `mapstructure:"scheduler_score_weights"`
}

// GatewayOpenAIWSSchedulerScoreWeights 账号调度打分权重。
type GatewayOpenAIWSSchedulerScoreWeights struct {
	Priority  float64 `mapstructure:"priority"`
	Load      float64 `mapstructure:"load"`
	Queue     float64 `mapstructure:"queue"`
	ErrorRate float64 `mapstructure:"error_rate"`
	TTFT      float64 `mapstructure:"ttft"`
	// Reset 倾向「会话窗口最早重置」的账号（use-it-or-lose-it）。
	// >0 时，剩余重置时间越短的账号得分越高，从而被优先用尽。默认 0（关闭，不改变原有行为）。
	Reset float64 `mapstructure:"reset"`
	// QuotaHeadroom 倾向 7d 剩余额度更健康的账号；默认 0（关闭，不改变原有行为）。
	QuotaHeadroom float64 `mapstructure:"quota_headroom"`
	// UpstreamCost 倾向上游声明倍率更低的账号；默认 0（关闭，不改变原有行为）。
	UpstreamCost float64 `mapstructure:"upstream_cost"`
	// PreviousResponse/SessionSticky 仅在开启 OpenAI 高级调度的粘性加权时生效。
	PreviousResponse float64 `mapstructure:"previous_response"`
	SessionSticky    float64 `mapstructure:"session_sticky"`
}

func (w GatewayOpenAIWSSchedulerScoreWeights) BaseWeightSum() float64 {
	return w.Priority + w.Load + w.Queue + w.ErrorRate + w.TTFT + w.Reset + w.QuotaHeadroom + w.UpstreamCost
}

func (w GatewayOpenAIWSSchedulerScoreWeights) TotalWeightSum() float64 {
	return w.BaseWeightSum() + w.PreviousResponse + w.SessionSticky
}

func (w GatewayOpenAIWSSchedulerScoreWeights) IsValid() bool {
	for _, weight := range []float64{
		w.Priority, w.Load, w.Queue, w.ErrorRate, w.TTFT, w.Reset,
		w.QuotaHeadroom, w.UpstreamCost, w.PreviousResponse, w.SessionSticky,
	} {
		if weight < 0 || math.IsNaN(weight) || math.IsInf(weight, 0) {
			return false
		}
	}
	baseSum := w.BaseWeightSum()
	return baseSum > 0 && !math.IsNaN(baseSum) && !math.IsInf(baseSum, 0) &&
		!math.IsNaN(w.TotalWeightSum()) && !math.IsInf(w.TotalWeightSum(), 0)
}

// GatewayOpenAISchedulerConfig OpenAI 高级调度器配置。
type GatewayOpenAISchedulerConfig struct {
	// StickyEscapeEnabled: 是否允许 session_hash sticky 在账号健康度劣化时临时逃逸
	StickyEscapeEnabled bool `mapstructure:"sticky_escape_enabled"`
	// StickyEscapeTTFTMs: TTFT EWMA 超过该阈值时跳过 sticky
	StickyEscapeTTFTMs int `mapstructure:"sticky_escape_ttft_ms"`
	// StickyEscapeErrorRate: 错误率 EWMA 超过该阈值时跳过 sticky
	StickyEscapeErrorRate float64 `mapstructure:"sticky_escape_error_rate"`
}

// GatewayUsageRecordConfig 使用量记录异步队列配置
type GatewayUsageRecordConfig struct {
	// WorkerCount: worker 初始数量（自动扩缩容开启时作为初始并发上限）
	WorkerCount int `mapstructure:"worker_count"`
	// QueueSize: 队列容量（有界）
	QueueSize int `mapstructure:"queue_size"`
	// TaskTimeoutSeconds: 单个使用量记录任务超时（秒）
	TaskTimeoutSeconds int `mapstructure:"task_timeout_seconds"`
	// OverflowPolicy: 队列满时策略（drop/sample/sync）
	OverflowPolicy string `mapstructure:"overflow_policy"`
	// OverflowSamplePercent: sample 策略下，同步回写采样百分比（1-100）
	OverflowSamplePercent int `mapstructure:"overflow_sample_percent"`

	// AutoScaleEnabled: 是否启用 worker 自动扩缩容
	AutoScaleEnabled bool `mapstructure:"auto_scale_enabled"`
	// AutoScaleMinWorkers: 自动扩缩容最小 worker 数
	AutoScaleMinWorkers int `mapstructure:"auto_scale_min_workers"`
	// AutoScaleMaxWorkers: 自动扩缩容最大 worker 数
	AutoScaleMaxWorkers int `mapstructure:"auto_scale_max_workers"`
	// AutoScaleUpQueuePercent: 队列占用率达到该阈值时触发扩容
	AutoScaleUpQueuePercent int `mapstructure:"auto_scale_up_queue_percent"`
	// AutoScaleDownQueuePercent: 队列占用率低于该阈值时触发缩容
	AutoScaleDownQueuePercent int `mapstructure:"auto_scale_down_queue_percent"`
	// AutoScaleUpStep: 每次扩容步长
	AutoScaleUpStep int `mapstructure:"auto_scale_up_step"`
	// AutoScaleDownStep: 每次缩容步长
	AutoScaleDownStep int `mapstructure:"auto_scale_down_step"`
	// AutoScaleCheckIntervalSeconds: 自动扩缩容检测间隔（秒）
	AutoScaleCheckIntervalSeconds int `mapstructure:"auto_scale_check_interval_seconds"`
	// AutoScaleCooldownSeconds: 自动扩缩容冷却时间（秒）
	AutoScaleCooldownSeconds int `mapstructure:"auto_scale_cooldown_seconds"`
}

// TLSFingerprintConfig TLS指纹伪装配置
// 用于模拟 Claude CLI (Node.js) 的 TLS 握手特征，避免被识别为非官方客户端
type TLSFingerprintConfig struct {
	// Enabled: 是否全局启用TLS指纹功能
	Enabled bool `mapstructure:"enabled"`
	// Profiles: 预定义的TLS指纹配置模板
	// key 为模板名称，如 "claude_cli_v2", "chrome_120" 等
	Profiles map[string]TLSProfileConfig `mapstructure:"profiles"`
}

// TLSProfileConfig 单个TLS指纹模板的配置
// 所有列表字段为空时使用内置默认值（Claude CLI 2.x / Node.js 20.x）
// 建议通过 TLS 指纹采集工具 (tests/tls-fingerprint-web) 获取完整配置
type TLSProfileConfig struct {
	// Name: 模板显示名称
	Name string `mapstructure:"name"`
	// EnableGREASE: 是否启用GREASE扩展（Chrome使用，Node.js不使用）
	EnableGREASE bool `mapstructure:"enable_grease"`
	// CipherSuites: TLS加密套件列表
	CipherSuites []uint16 `mapstructure:"cipher_suites"`
	// Curves: 椭圆曲线列表
	Curves []uint16 `mapstructure:"curves"`
	// PointFormats: 点格式列表
	PointFormats []uint16 `mapstructure:"point_formats"`
	// SignatureAlgorithms: 签名算法列表
	SignatureAlgorithms []uint16 `mapstructure:"signature_algorithms"`
	// ALPNProtocols: ALPN协议列表（如 ["h2", "http/1.1"]）
	ALPNProtocols []string `mapstructure:"alpn_protocols"`
	// SupportedVersions: 支持的TLS版本列表（如 [0x0304, 0x0303] 即 TLS1.3, TLS1.2）
	SupportedVersions []uint16 `mapstructure:"supported_versions"`
	// KeyShareGroups: Key Share中发送的曲线组（如 [29] 即 X25519）
	KeyShareGroups []uint16 `mapstructure:"key_share_groups"`
	// PSKModes: PSK密钥交换模式（如 [1] 即 psk_dhe_ke）
	PSKModes []uint16 `mapstructure:"psk_modes"`
	// Extensions: TLS扩展类型ID列表，按发送顺序排列
	// 空则使用内置默认顺序 [0,11,10,35,16,22,23,13,43,45,51]
	// GREASE值(如0x0a0a)会自动插入GREASE扩展
	Extensions []uint16 `mapstructure:"extensions"`
}

// GatewaySchedulingConfig accounts scheduling configuration.
type GatewaySchedulingConfig struct {
	// 粘性会话排队配置
	StickySessionMaxWaiting  int           `mapstructure:"sticky_session_max_waiting"`
	StickySessionWaitTimeout time.Duration `mapstructure:"sticky_session_wait_timeout"`

	// 兜底排队配置
	FallbackWaitTimeout time.Duration `mapstructure:"fallback_wait_timeout"`
	FallbackMaxWaiting  int           `mapstructure:"fallback_max_waiting"`

	// 兜底层账户选择策略: "last_used"(按最后使用时间排序，默认) 或 "random"(随机)
	FallbackSelectionMode string `mapstructure:"fallback_selection_mode"`

	// PreferSoonestReset 开启后，负载感知选择会优先选用「会话窗口最早重置」的账号
	// （use-it-or-lose-it：先用尽即将重置的账号，保留重置时间还很久的账号）。
	// 默认 false，保持原有「优先级 → 负载率 → LRU」行为不变。
	PreferSoonestReset bool `mapstructure:"prefer_soonest_reset"`

	// 负载计算
	LoadBatchEnabled    bool `mapstructure:"load_batch_enabled"`
	LoadBatchCacheTTLMS int  `mapstructure:"load_batch_cache_ttl_ms"`
	// 快照桶读取时的 MGET 分块大小
	SnapshotMGetChunkSize int `mapstructure:"snapshot_mget_chunk_size"`
	// 快照重建时的缓存写入分块大小
	SnapshotWriteChunkSize int `mapstructure:"snapshot_write_chunk_size"`

	// 过期槽位清理周期（0 表示禁用）
	SlotCleanupInterval time.Duration `mapstructure:"slot_cleanup_interval"`

	// 受控回源配置
	DbFallbackEnabled bool `mapstructure:"db_fallback_enabled"`
	// 受控回源超时（秒），0 表示不额外收紧超时
	DbFallbackTimeoutSeconds int `mapstructure:"db_fallback_timeout_seconds"`
	// 受控回源限流（实例级 QPS），0 表示不限制
	DbFallbackMaxQPS int `mapstructure:"db_fallback_max_qps"`

	// Outbox 轮询与滞后阈值配置
	// Outbox 轮询周期（秒）
	OutboxPollIntervalSeconds int `mapstructure:"outbox_poll_interval_seconds"`
	// Outbox 滞后告警阈值（秒）
	OutboxLagWarnSeconds int `mapstructure:"outbox_lag_warn_seconds"`
	// Outbox 触发强制重建阈值（秒）
	OutboxLagRebuildSeconds int `mapstructure:"outbox_lag_rebuild_seconds"`
	// Outbox 连续滞后触发次数
	OutboxLagRebuildFailures int `mapstructure:"outbox_lag_rebuild_failures"`
	// Outbox 积压触发重建阈值（行数）
	OutboxBacklogRebuildRows int `mapstructure:"outbox_backlog_rebuild_rows"`

	// 全量重建周期配置
	// 全量重建周期（秒），0 表示禁用
	FullRebuildIntervalSeconds int `mapstructure:"full_rebuild_interval_seconds"`
}

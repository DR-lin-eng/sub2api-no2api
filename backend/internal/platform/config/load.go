package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/viper"
)

func NormalizeRunMode(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case RunModeStandard, RunModeSimple:
		return normalized
	default:
		return RunModeStandard
	}
}

// Load 读取并校验完整配置（要求 jwt.secret 已显式提供）。
func Load() (*Config, error) {
	return load(false)
}

// LoadForBootstrap 读取启动阶段配置。
//
// 启动阶段允许 jwt.secret 先留空，后续由数据库初始化流程补齐并再次完整校验。
func LoadForBootstrap() (*Config, error) {
	return load(true)
}

func load(allowMissingJWTSecret bool) (*Config, error) {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	// Add config paths in priority order
	// 1. DATA_DIR environment variable (highest priority)
	if dataDir := os.Getenv("DATA_DIR"); dataDir != "" {
		viper.AddConfigPath(dataDir)
	}
	// 2. Docker data directory
	viper.AddConfigPath("/app/data")
	// 3. Current directory
	viper.AddConfigPath(".")
	// 4. Config subdirectory
	viper.AddConfigPath("./config")
	// 5. System config directory
	viper.AddConfigPath("/etc/sub2api")

	// 环境变量支持
	viper.AutomaticEnv()
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	if err := viper.BindEnv("deployment.mode", "DEPLOYMENT_MODE"); err != nil {
		return nil, fmt.Errorf("bind DEPLOYMENT_MODE: %w", err)
	}
	if err := viper.BindEnv("deployment.node_name", "NODE_NAME"); err != nil {
		return nil, fmt.Errorf("bind NODE_NAME: %w", err)
	}
	if err := viper.BindEnv("deployment.worker_enabled", "WORKER_ENABLED"); err != nil {
		return nil, fmt.Errorf("bind WORKER_ENABLED: %w", err)
	}
	if err := viper.BindEnv("server.enable_server_timing", "ENABLE_SERVER_TIMING"); err != nil {
		return nil, fmt.Errorf("bind ENABLE_SERVER_TIMING: %w", err)
	}

	// 默认值
	setDefaults()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("read config error: %w", err)
		}
		// 配置文件不存在时使用默认值
	}
	trustedProxiesEnv, trustedProxiesEnvConfigured := os.LookupEnv("SERVER_TRUSTED_PROXIES")

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config error: %w", err)
	}
	if trustedProxiesEnvConfigured {
		cfg.Server.TrustedProxies = normalizeStringSlice(strings.Split(trustedProxiesEnv, ","))
	}
	if cfg.Gateway.OpenAIScheduler.StickyEscapeTTFTMs == 0 {
		cfg.Gateway.OpenAIScheduler.StickyEscapeTTFTMs = 15000
	}
	if cfg.Gateway.OpenAIScheduler.StickyEscapeErrorRate == 0 {
		cfg.Gateway.OpenAIScheduler.StickyEscapeErrorRate = 0.5
	}
	// Kept as a backstop: setEnvReachableDefaults now registers this key with its
	// effective default (true), so IsSet always reports true and this branch no
	// longer fires. It still guards the default if that registration is dropped.
	if !cfg.Gateway.OpenAIScheduler.StickyEscapeEnabled && !viper.IsSet("gateway.openai_scheduler.sticky_escape_enabled") {
		cfg.Gateway.OpenAIScheduler.StickyEscapeEnabled = true
	}
	normalizeDeploymentConfig(&cfg.Deployment)

	cfg.RunMode = NormalizeRunMode(cfg.RunMode)
	cfg.Server.Mode = strings.ToLower(strings.TrimSpace(cfg.Server.Mode))
	if cfg.Server.Mode == "" {
		cfg.Server.Mode = "debug"
	}
	cfg.Server.FrontendURL = strings.TrimSpace(cfg.Server.FrontendURL)
	cfg.JWT.Secret = strings.TrimSpace(cfg.JWT.Secret)
	cfg.LinuxDo.ClientID = strings.TrimSpace(cfg.LinuxDo.ClientID)
	cfg.LinuxDo.ClientSecret = strings.TrimSpace(cfg.LinuxDo.ClientSecret)
	cfg.LinuxDo.AuthorizeURL = strings.TrimSpace(cfg.LinuxDo.AuthorizeURL)
	cfg.LinuxDo.TokenURL = strings.TrimSpace(cfg.LinuxDo.TokenURL)
	cfg.LinuxDo.UserInfoURL = strings.TrimSpace(cfg.LinuxDo.UserInfoURL)
	cfg.LinuxDo.Scopes = strings.TrimSpace(cfg.LinuxDo.Scopes)
	cfg.LinuxDo.RedirectURL = strings.TrimSpace(cfg.LinuxDo.RedirectURL)
	cfg.LinuxDo.FrontendRedirectURL = strings.TrimSpace(cfg.LinuxDo.FrontendRedirectURL)
	cfg.LinuxDo.TokenAuthMethod = strings.ToLower(strings.TrimSpace(cfg.LinuxDo.TokenAuthMethod))
	cfg.LinuxDo.UserInfoEmailPath = strings.TrimSpace(cfg.LinuxDo.UserInfoEmailPath)
	cfg.LinuxDo.UserInfoIDPath = strings.TrimSpace(cfg.LinuxDo.UserInfoIDPath)
	cfg.LinuxDo.UserInfoUsernamePath = strings.TrimSpace(cfg.LinuxDo.UserInfoUsernamePath)
	applyLegacyWeChatConnectEnvCompatibility(&cfg.WeChat)
	normalizeWeChatConnectConfig(&cfg.WeChat)
	cfg.OIDC.ProviderName = strings.TrimSpace(cfg.OIDC.ProviderName)
	cfg.OIDC.ClientID = strings.TrimSpace(cfg.OIDC.ClientID)
	cfg.OIDC.ClientSecret = strings.TrimSpace(cfg.OIDC.ClientSecret)
	cfg.OIDC.IssuerURL = strings.TrimSpace(cfg.OIDC.IssuerURL)
	cfg.OIDC.DiscoveryURL = strings.TrimSpace(cfg.OIDC.DiscoveryURL)
	cfg.OIDC.AuthorizeURL = strings.TrimSpace(cfg.OIDC.AuthorizeURL)
	cfg.OIDC.TokenURL = strings.TrimSpace(cfg.OIDC.TokenURL)
	cfg.OIDC.UserInfoURL = strings.TrimSpace(cfg.OIDC.UserInfoURL)
	cfg.OIDC.JWKSURL = strings.TrimSpace(cfg.OIDC.JWKSURL)
	cfg.OIDC.Scopes = strings.TrimSpace(cfg.OIDC.Scopes)
	cfg.OIDC.RedirectURL = strings.TrimSpace(cfg.OIDC.RedirectURL)
	cfg.OIDC.FrontendRedirectURL = strings.TrimSpace(cfg.OIDC.FrontendRedirectURL)
	cfg.OIDC.TokenAuthMethod = strings.ToLower(strings.TrimSpace(cfg.OIDC.TokenAuthMethod))
	cfg.OIDC.AllowedSigningAlgs = strings.TrimSpace(cfg.OIDC.AllowedSigningAlgs)
	cfg.OIDC.UserInfoEmailPath = strings.TrimSpace(cfg.OIDC.UserInfoEmailPath)
	cfg.OIDC.UserInfoIDPath = strings.TrimSpace(cfg.OIDC.UserInfoIDPath)
	cfg.OIDC.UserInfoUsernamePath = strings.TrimSpace(cfg.OIDC.UserInfoUsernamePath)
	cfg.OIDC.UsePKCEExplicit = hasExplicitConfigOrEnv("oidc_connect.use_pkce", "OIDC_CONNECT_USE_PKCE")
	cfg.OIDC.ValidateIDTokenExplicit = hasExplicitConfigOrEnv("oidc_connect.validate_id_token", "OIDC_CONNECT_VALIDATE_ID_TOKEN")
	cfg.Dashboard.KeyPrefix = strings.TrimSpace(cfg.Dashboard.KeyPrefix)
	cfg.CORS.AllowedOrigins = normalizeStringSlice(cfg.CORS.AllowedOrigins)
	cfg.Security.ResponseHeaders.AdditionalAllowed = normalizeStringSlice(cfg.Security.ResponseHeaders.AdditionalAllowed)
	cfg.Security.ResponseHeaders.ForceRemove = normalizeStringSlice(cfg.Security.ResponseHeaders.ForceRemove)
	cfg.Security.CSP.Policy = strings.TrimSpace(cfg.Security.CSP.Policy)
	cfg.SetTrustForwardedIPForAPIKeyACL(cfg.Security.TrustForwardedIPForAPIKeyACL)
	cfg.Log.Level = strings.ToLower(strings.TrimSpace(cfg.Log.Level))
	cfg.Log.Format = strings.ToLower(strings.TrimSpace(cfg.Log.Format))
	cfg.Log.ServiceName = strings.TrimSpace(cfg.Log.ServiceName)
	cfg.Log.Environment = strings.TrimSpace(cfg.Log.Environment)
	cfg.Log.StacktraceLevel = strings.ToLower(strings.TrimSpace(cfg.Log.StacktraceLevel))
	cfg.Log.Output.FilePath = strings.TrimSpace(cfg.Log.Output.FilePath)
	cfg.Gateway.ForcedCodexInstructionsTemplateFile = strings.TrimSpace(cfg.Gateway.ForcedCodexInstructionsTemplateFile)
	if cfg.Gateway.ForcedCodexInstructionsTemplateFile != "" {
		content, err := os.ReadFile(cfg.Gateway.ForcedCodexInstructionsTemplateFile)
		if err != nil {
			return nil, fmt.Errorf("read forced codex instructions template %q: %w", cfg.Gateway.ForcedCodexInstructionsTemplateFile, err)
		}
		cfg.Gateway.ForcedCodexInstructionsTemplate = string(content)
	}

	// 兼容旧键 gateway.openai_ws.sticky_previous_response_ttl_seconds。
	// 新键未配置（<=0）时回退旧键；新键优先。
	if cfg.Gateway.OpenAIWS.StickyResponseIDTTLSeconds <= 0 && cfg.Gateway.OpenAIWS.StickyPreviousResponseTTLSeconds > 0 {
		cfg.Gateway.OpenAIWS.StickyResponseIDTTLSeconds = cfg.Gateway.OpenAIWS.StickyPreviousResponseTTLSeconds
	}

	// Normalize UMQ mode: 白名单校验，非法值在加载时一次性 warn 并清空
	if m := cfg.Gateway.UserMessageQueue.Mode; m != "" && m != UMQModeSerialize && m != UMQModeThrottle {
		slog.Warn("invalid user_message_queue mode, disabling",
			"mode", m,
			"valid_modes", []string{UMQModeSerialize, UMQModeThrottle})
		cfg.Gateway.UserMessageQueue.Mode = ""
	}

	// Generate a bootstrap candidate when unset. Repository initialization persists
	// the first value in PostgreSQL and replaces later replicas' candidates with it.
	cfg.Totp.EncryptionKey = strings.TrimSpace(cfg.Totp.EncryptionKey)
	if cfg.Totp.EncryptionKey == "" {
		key, err := generateJWTSecret(32) // Reuse the same random generation function
		if err != nil {
			return nil, fmt.Errorf("generate totp encryption key error: %w", err)
		}
		cfg.Totp.EncryptionKey = key
		cfg.Totp.EncryptionKeyConfigured = false
		slog.Info("TOTP encryption key bootstrap candidate generated; PostgreSQL will reconcile it across instances")
	} else {
		cfg.Totp.EncryptionKeyConfigured = true
	}

	originalJWTSecret := cfg.JWT.Secret
	if allowMissingJWTSecret && originalJWTSecret == "" {
		// 启动阶段允许先无 JWT 密钥，后续在数据库初始化后补齐。
		cfg.JWT.Secret = strings.Repeat("0", 32)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config error: %w", err)
	}

	if allowMissingJWTSecret && originalJWTSecret == "" {
		cfg.JWT.Secret = ""
	}

	if !cfg.Security.URLAllowlist.Enabled {
		slog.Warn("security.url_allowlist.enabled=false; allowlist/SSRF checks disabled (minimal format validation only).")
	}
	if !cfg.Security.ResponseHeaders.Enabled {
		slog.Warn("security.response_headers.enabled=false; configurable header filtering disabled (default allowlist only).")
	}

	if cfg.JWT.Secret != "" && isWeakJWTSecret(cfg.JWT.Secret) {
		slog.Warn("JWT secret appears weak; use a 32+ character random secret in production.")
	}
	if len(cfg.Security.ResponseHeaders.AdditionalAllowed) > 0 || len(cfg.Security.ResponseHeaders.ForceRemove) > 0 {
		slog.Info("response header policy configured",
			"additional_allowed", cfg.Security.ResponseHeaders.AdditionalAllowed,
			"force_remove", cfg.Security.ResponseHeaders.ForceRemove,
		)
	}

	return &cfg, nil
}

func normalizeDeploymentConfig(cfg *DeploymentConfig) {
	if cfg == nil {
		return
	}
	cfg.Mode = strings.ToLower(strings.TrimSpace(cfg.Mode))
	if cfg.Mode == "" {
		cfg.Mode = DeploymentModeStandalone
	}
	cfg.WorkerEnabled = cfg.WorkerMode()
	cfg.NodeName = strings.TrimSpace(cfg.NodeName)
	if cfg.NodeName == "" {
		if hostname, err := os.Hostname(); err == nil {
			cfg.NodeName = strings.TrimSpace(hostname)
		}
	}
	if cfg.NodeName == "" {
		cfg.NodeName = "sub2api-node"
	}
	if cfg.HeartbeatIntervalSeconds <= 0 {
		cfg.HeartbeatIntervalSeconds = 30
	}
	if cfg.StaleAfterSeconds <= 0 {
		cfg.StaleAfterSeconds = 90
	}
	if cfg.TaskLeaseSeconds <= 0 {
		cfg.TaskLeaseSeconds = 60
	}
}

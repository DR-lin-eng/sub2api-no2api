package config

import (
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"strings"
)

func (c *Config) Validate() error {
	normalizeDeploymentConfig(&c.Deployment)
	if c.Deployment.Mode != DeploymentModeStandalone && c.Deployment.Mode != DeploymentModeMultiInstance {
		return fmt.Errorf("deployment.mode must be one of: standalone/multi_instance")
	}
	if c.Deployment.HeartbeatIntervalSeconds <= 0 {
		return fmt.Errorf("deployment.heartbeat_interval_seconds must be positive")
	}
	if c.Deployment.StaleAfterSeconds < c.Deployment.HeartbeatIntervalSeconds*2 {
		return fmt.Errorf("deployment.stale_after_seconds must be at least twice heartbeat_interval_seconds")
	}
	if c.Deployment.TaskLeaseSeconds < 15 {
		return fmt.Errorf("deployment.task_lease_seconds must be at least 15")
	}
	if len(c.Deployment.NodeName) > 128 {
		return fmt.Errorf("deployment.node_name must not exceed 128 characters")
	}
	if c.Server.ReadHeaderTimeout < 1 || c.Server.ReadHeaderTimeout > 60 {
		return fmt.Errorf("server.read_header_timeout must be between 1 and 60 seconds")
	}
	if c.Server.MaxHeaderBytes < 8*1024 || c.Server.MaxHeaderBytes > 1024*1024 {
		return fmt.Errorf("server.max_header_bytes must be between 8192 and 1048576 bytes")
	}
	if c.Server.IdleTimeout <= 0 {
		return fmt.Errorf("server.idle_timeout must be positive")
	}
	if c.Server.MaxRequestBodySize < 0 {
		return fmt.Errorf("server.max_request_body_size must be non-negative")
	}
	if c.Server.H2C.Enabled {
		if c.Server.H2C.MaxConcurrentStreams == 0 {
			return fmt.Errorf("server.h2c.max_concurrent_streams must be positive")
		}
		if c.Server.H2C.IdleTimeout <= 0 {
			return fmt.Errorf("server.h2c.idle_timeout must be positive")
		}
		if c.Server.H2C.MaxReadFrameSize < 16*1024 || c.Server.H2C.MaxReadFrameSize > 16*1024*1024-1 {
			return fmt.Errorf("server.h2c.max_read_frame_size must be between 16384 and 16777215 bytes")
		}
		if c.Server.H2C.MaxUploadBufferPerConnection < 65535 {
			return fmt.Errorf("server.h2c.max_upload_buffer_per_connection must be at least 65535 bytes")
		}
		if c.Server.H2C.MaxUploadBufferPerStream <= 0 {
			return fmt.Errorf("server.h2c.max_upload_buffer_per_stream must be positive")
		}
	}
	if c.APIKeyAuth.InvalidAbuse.Enabled {
		if c.APIKeyAuth.InvalidAbuse.Threshold < 10 {
			return fmt.Errorf("api_key_auth_cache.invalid_abuse.threshold must be at least 10")
		}
		if c.APIKeyAuth.InvalidAbuse.WindowSeconds < 1 || c.APIKeyAuth.InvalidAbuse.WindowSeconds > 3600 {
			return fmt.Errorf("api_key_auth_cache.invalid_abuse.window_seconds must be between 1 and 3600")
		}
		if c.APIKeyAuth.InvalidAbuse.BlockSeconds < 1 || c.APIKeyAuth.InvalidAbuse.BlockSeconds > 3600 {
			return fmt.Errorf("api_key_auth_cache.invalid_abuse.block_seconds must be between 1 and 3600")
		}
		if c.APIKeyAuth.InvalidAbuse.Capacity < 256 || c.APIKeyAuth.InvalidAbuse.Capacity > 1_000_000 {
			return fmt.Errorf("api_key_auth_cache.invalid_abuse.capacity must be between 256 and 1000000")
		}
	}
	jwtSecret := strings.TrimSpace(c.JWT.Secret)
	if jwtSecret == "" {
		return fmt.Errorf("jwt.secret is required")
	}
	// NOTE: 按 UTF-8 编码后的字节长度计算。
	// 选择 bytes 而不是 rune 计数，确保二进制/随机串的长度语义更接近“熵”而非“字符数”。
	if len([]byte(jwtSecret)) < 32 {
		return fmt.Errorf("jwt.secret must be at least 32 bytes")
	}
	switch c.Log.Level {
	case "debug", "info", "warn", "error":
	case "":
		return fmt.Errorf("log.level is required")
	default:
		return fmt.Errorf("log.level must be one of: debug/info/warn/error")
	}
	switch c.Log.Format {
	case "json", "console":
	case "":
		return fmt.Errorf("log.format is required")
	default:
		return fmt.Errorf("log.format must be one of: json/console")
	}
	switch c.Log.StacktraceLevel {
	case "none", "error", "fatal":
	case "":
		return fmt.Errorf("log.stacktrace_level is required")
	default:
		return fmt.Errorf("log.stacktrace_level must be one of: none/error/fatal")
	}
	if !c.Log.Output.ToStdout && !c.Log.Output.ToFile {
		return fmt.Errorf("log.output.to_stdout and log.output.to_file cannot both be false")
	}
	if c.Log.Rotation.MaxSizeMB <= 0 {
		return fmt.Errorf("log.rotation.max_size_mb must be positive")
	}
	if c.Log.Rotation.MaxBackups < 0 {
		return fmt.Errorf("log.rotation.max_backups must be non-negative")
	}
	if c.Log.Rotation.MaxAgeDays < 0 {
		return fmt.Errorf("log.rotation.max_age_days must be non-negative")
	}
	if c.Log.Sampling.Enabled {
		if c.Log.Sampling.Initial <= 0 {
			return fmt.Errorf("log.sampling.initial must be positive when sampling is enabled")
		}
		if c.Log.Sampling.Thereafter <= 0 {
			return fmt.Errorf("log.sampling.thereafter must be positive when sampling is enabled")
		}
	} else {
		if c.Log.Sampling.Initial < 0 {
			return fmt.Errorf("log.sampling.initial must be non-negative")
		}
		if c.Log.Sampling.Thereafter < 0 {
			return fmt.Errorf("log.sampling.thereafter must be non-negative")
		}
	}

	if c.SubscriptionMaintenance.WorkerCount < 0 {
		return fmt.Errorf("subscription_maintenance.worker_count must be non-negative")
	}
	if c.SubscriptionMaintenance.QueueSize < 0 {
		return fmt.Errorf("subscription_maintenance.queue_size must be non-negative")
	}

	// Gemini OAuth 配置校验：client_id 与 client_secret 必须同时设置或同时留空。
	// 留空时表示使用内置的 Gemini CLI OAuth 客户端（其 client_secret 通过环境变量注入）。
	geminiClientID := strings.TrimSpace(c.Gemini.OAuth.ClientID)
	geminiClientSecret := strings.TrimSpace(c.Gemini.OAuth.ClientSecret)
	if (geminiClientID == "") != (geminiClientSecret == "") {
		return fmt.Errorf("gemini.oauth.client_id and gemini.oauth.client_secret must be both set or both empty")
	}

	if strings.TrimSpace(c.Server.FrontendURL) != "" {
		if err := ValidateAbsoluteHTTPURL(c.Server.FrontendURL); err != nil {
			return fmt.Errorf("server.frontend_url invalid: %w", err)
		}
		u, err := url.Parse(strings.TrimSpace(c.Server.FrontendURL))
		if err != nil {
			return fmt.Errorf("server.frontend_url invalid: %w", err)
		}
		if u.RawQuery != "" || u.ForceQuery {
			return fmt.Errorf("server.frontend_url invalid: must not include query")
		}
		if u.User != nil {
			return fmt.Errorf("server.frontend_url invalid: must not include userinfo")
		}
		warnIfInsecureURL("server.frontend_url", c.Server.FrontendURL)
	}
	if c.JWT.ExpireHour <= 0 {
		return fmt.Errorf("jwt.expire_hour must be positive")
	}
	if c.JWT.ExpireHour > 168 {
		return fmt.Errorf("jwt.expire_hour must be <= 168 (7 days)")
	}
	if c.JWT.ExpireHour > 24 {
		slog.Warn("jwt.expire_hour is high; consider shorter expiration for security", "expire_hour", c.JWT.ExpireHour)
	}
	// JWT Refresh Token配置验证
	if c.JWT.AccessTokenExpireMinutes < 0 {
		return fmt.Errorf("jwt.access_token_expire_minutes must be non-negative")
	}
	if c.JWT.AccessTokenExpireMinutes > 720 {
		slog.Warn("jwt.access_token_expire_minutes is high; consider shorter expiration for security", "access_token_expire_minutes", c.JWT.AccessTokenExpireMinutes)
	}
	if c.JWT.RefreshTokenExpireDays <= 0 {
		return fmt.Errorf("jwt.refresh_token_expire_days must be positive")
	}
	if c.JWT.RefreshTokenExpireDays < 7 {
		slog.Warn("jwt.refresh_token_expire_days below browser session minimum; effective lifetime is 7 days", "refresh_token_expire_days", c.JWT.RefreshTokenExpireDays)
	}
	if c.JWT.RefreshTokenExpireDays > 90 {
		slog.Warn("jwt.refresh_token_expire_days is high; consider shorter expiration for security", "refresh_token_expire_days", c.JWT.RefreshTokenExpireDays)
	}
	if c.JWT.RefreshWindowMinutes < 0 {
		return fmt.Errorf("jwt.refresh_window_minutes must be non-negative")
	}
	if c.Security.CSP.Enabled && strings.TrimSpace(c.Security.CSP.Policy) == "" {
		return fmt.Errorf("security.csp.policy is required when CSP is enabled")
	}
	if c.LinuxDo.Enabled {
		if strings.TrimSpace(c.LinuxDo.ClientID) == "" {
			return fmt.Errorf("linuxdo_connect.client_id is required when linuxdo_connect.enabled=true")
		}
		if strings.TrimSpace(c.LinuxDo.AuthorizeURL) == "" {
			return fmt.Errorf("linuxdo_connect.authorize_url is required when linuxdo_connect.enabled=true")
		}
		if strings.TrimSpace(c.LinuxDo.TokenURL) == "" {
			return fmt.Errorf("linuxdo_connect.token_url is required when linuxdo_connect.enabled=true")
		}
		if strings.TrimSpace(c.LinuxDo.UserInfoURL) == "" {
			return fmt.Errorf("linuxdo_connect.userinfo_url is required when linuxdo_connect.enabled=true")
		}
		if strings.TrimSpace(c.LinuxDo.RedirectURL) == "" {
			return fmt.Errorf("linuxdo_connect.redirect_url is required when linuxdo_connect.enabled=true")
		}
		method := strings.ToLower(strings.TrimSpace(c.LinuxDo.TokenAuthMethod))
		switch method {
		case "", "client_secret_post", "client_secret_basic", "none":
		default:
			return fmt.Errorf("linuxdo_connect.token_auth_method must be one of: client_secret_post/client_secret_basic/none")
		}
		if (method == "" || method == "client_secret_post" || method == "client_secret_basic") &&
			strings.TrimSpace(c.LinuxDo.ClientSecret) == "" {
			return fmt.Errorf("linuxdo_connect.client_secret is required when linuxdo_connect.enabled=true and token_auth_method is client_secret_post/client_secret_basic")
		}
		if strings.TrimSpace(c.LinuxDo.FrontendRedirectURL) == "" {
			return fmt.Errorf("linuxdo_connect.frontend_redirect_url is required when linuxdo_connect.enabled=true")
		}

		if err := ValidateAbsoluteHTTPURL(c.LinuxDo.AuthorizeURL); err != nil {
			return fmt.Errorf("linuxdo_connect.authorize_url invalid: %w", err)
		}
		if err := ValidateAbsoluteHTTPURL(c.LinuxDo.TokenURL); err != nil {
			return fmt.Errorf("linuxdo_connect.token_url invalid: %w", err)
		}
		if err := ValidateAbsoluteHTTPURL(c.LinuxDo.UserInfoURL); err != nil {
			return fmt.Errorf("linuxdo_connect.userinfo_url invalid: %w", err)
		}
		if err := ValidateAbsoluteHTTPURL(c.LinuxDo.RedirectURL); err != nil {
			return fmt.Errorf("linuxdo_connect.redirect_url invalid: %w", err)
		}
		if err := ValidateFrontendRedirectURL(c.LinuxDo.FrontendRedirectURL); err != nil {
			return fmt.Errorf("linuxdo_connect.frontend_redirect_url invalid: %w", err)
		}

		warnIfInsecureURL("linuxdo_connect.authorize_url", c.LinuxDo.AuthorizeURL)
		warnIfInsecureURL("linuxdo_connect.token_url", c.LinuxDo.TokenURL)
		warnIfInsecureURL("linuxdo_connect.userinfo_url", c.LinuxDo.UserInfoURL)
		warnIfInsecureURL("linuxdo_connect.redirect_url", c.LinuxDo.RedirectURL)
		warnIfInsecureURL("linuxdo_connect.frontend_redirect_url", c.LinuxDo.FrontendRedirectURL)
	}
	if c.WeChat.Enabled {
		weChat := c.WeChat
		normalizeWeChatConnectConfig(&weChat)

		if weChat.OpenEnabled {
			if strings.TrimSpace(weChat.OpenAppID) == "" {
				return fmt.Errorf("wechat_connect.open_app_id is required when wechat_connect.open_enabled=true")
			}
			if strings.TrimSpace(weChat.OpenAppSecret) == "" {
				return fmt.Errorf("wechat_connect.open_app_secret is required when wechat_connect.open_enabled=true")
			}
		}
		if weChat.MPEnabled {
			if strings.TrimSpace(weChat.MPAppID) == "" {
				return fmt.Errorf("wechat_connect.mp_app_id is required when wechat_connect.mp_enabled=true")
			}
			if strings.TrimSpace(weChat.MPAppSecret) == "" {
				return fmt.Errorf("wechat_connect.mp_app_secret is required when wechat_connect.mp_enabled=true")
			}
		}
		if weChat.MobileEnabled {
			if strings.TrimSpace(weChat.MobileAppID) == "" {
				return fmt.Errorf("wechat_connect.mobile_app_id is required when wechat_connect.mobile_enabled=true")
			}
			if strings.TrimSpace(weChat.MobileAppSecret) == "" {
				return fmt.Errorf("wechat_connect.mobile_app_secret is required when wechat_connect.mobile_enabled=true")
			}
		}
		if v := strings.TrimSpace(weChat.RedirectURL); v != "" {
			if err := ValidateAbsoluteHTTPURL(v); err != nil {
				return fmt.Errorf("wechat_connect.redirect_url invalid: %w", err)
			}
			warnIfInsecureURL("wechat_connect.redirect_url", v)
		}
		if err := ValidateFrontendRedirectURL(weChat.FrontendRedirectURL); err != nil {
			return fmt.Errorf("wechat_connect.frontend_redirect_url invalid: %w", err)
		}
		warnIfInsecureURL("wechat_connect.frontend_redirect_url", weChat.FrontendRedirectURL)
	}
	if c.OIDC.Enabled {
		if strings.TrimSpace(c.OIDC.ClientID) == "" {
			return fmt.Errorf("oidc_connect.client_id is required when oidc_connect.enabled=true")
		}
		if strings.TrimSpace(c.OIDC.IssuerURL) == "" {
			return fmt.Errorf("oidc_connect.issuer_url is required when oidc_connect.enabled=true")
		}
		if strings.TrimSpace(c.OIDC.RedirectURL) == "" {
			return fmt.Errorf("oidc_connect.redirect_url is required when oidc_connect.enabled=true")
		}
		if strings.TrimSpace(c.OIDC.FrontendRedirectURL) == "" {
			return fmt.Errorf("oidc_connect.frontend_redirect_url is required when oidc_connect.enabled=true")
		}
		if !scopeContainsOpenID(c.OIDC.Scopes) {
			return fmt.Errorf("oidc_connect.scopes must contain openid")
		}

		method := strings.ToLower(strings.TrimSpace(c.OIDC.TokenAuthMethod))
		switch method {
		case "", "client_secret_post", "client_secret_basic", "none":
		default:
			return fmt.Errorf("oidc_connect.token_auth_method must be one of: client_secret_post/client_secret_basic/none")
		}
		if (method == "" || method == "client_secret_post" || method == "client_secret_basic") &&
			strings.TrimSpace(c.OIDC.ClientSecret) == "" {
			return fmt.Errorf("oidc_connect.client_secret is required when oidc_connect.enabled=true and token_auth_method is client_secret_post/client_secret_basic")
		}
		if c.OIDC.ClockSkewSeconds < 0 || c.OIDC.ClockSkewSeconds > 600 {
			return fmt.Errorf("oidc_connect.clock_skew_seconds must be between 0 and 600")
		}
		if c.OIDC.ValidateIDToken && strings.TrimSpace(c.OIDC.AllowedSigningAlgs) == "" {
			return fmt.Errorf("oidc_connect.allowed_signing_algs is required when oidc_connect.validate_id_token=true")
		}

		if err := ValidateAbsoluteHTTPURL(c.OIDC.IssuerURL); err != nil {
			return fmt.Errorf("oidc_connect.issuer_url invalid: %w", err)
		}
		if v := strings.TrimSpace(c.OIDC.DiscoveryURL); v != "" {
			if err := ValidateAbsoluteHTTPURL(v); err != nil {
				return fmt.Errorf("oidc_connect.discovery_url invalid: %w", err)
			}
		}
		if v := strings.TrimSpace(c.OIDC.AuthorizeURL); v != "" {
			if err := ValidateAbsoluteHTTPURL(v); err != nil {
				return fmt.Errorf("oidc_connect.authorize_url invalid: %w", err)
			}
		}
		if v := strings.TrimSpace(c.OIDC.TokenURL); v != "" {
			if err := ValidateAbsoluteHTTPURL(v); err != nil {
				return fmt.Errorf("oidc_connect.token_url invalid: %w", err)
			}
		}
		if v := strings.TrimSpace(c.OIDC.UserInfoURL); v != "" {
			if err := ValidateAbsoluteHTTPURL(v); err != nil {
				return fmt.Errorf("oidc_connect.userinfo_url invalid: %w", err)
			}
		}
		if v := strings.TrimSpace(c.OIDC.JWKSURL); v != "" {
			if err := ValidateAbsoluteHTTPURL(v); err != nil {
				return fmt.Errorf("oidc_connect.jwks_url invalid: %w", err)
			}
		}
		if err := ValidateAbsoluteHTTPURL(c.OIDC.RedirectURL); err != nil {
			return fmt.Errorf("oidc_connect.redirect_url invalid: %w", err)
		}
		if err := ValidateFrontendRedirectURL(c.OIDC.FrontendRedirectURL); err != nil {
			return fmt.Errorf("oidc_connect.frontend_redirect_url invalid: %w", err)
		}

		warnIfInsecureURL("oidc_connect.issuer_url", c.OIDC.IssuerURL)
		warnIfInsecureURL("oidc_connect.discovery_url", c.OIDC.DiscoveryURL)
		warnIfInsecureURL("oidc_connect.authorize_url", c.OIDC.AuthorizeURL)
		warnIfInsecureURL("oidc_connect.token_url", c.OIDC.TokenURL)
		warnIfInsecureURL("oidc_connect.userinfo_url", c.OIDC.UserInfoURL)
		warnIfInsecureURL("oidc_connect.jwks_url", c.OIDC.JWKSURL)
		warnIfInsecureURL("oidc_connect.redirect_url", c.OIDC.RedirectURL)
		warnIfInsecureURL("oidc_connect.frontend_redirect_url", c.OIDC.FrontendRedirectURL)
	}
	if c.Billing.CircuitBreaker.Enabled {
		if c.Billing.CircuitBreaker.FailureThreshold <= 0 {
			return fmt.Errorf("billing.circuit_breaker.failure_threshold must be positive")
		}
		if c.Billing.CircuitBreaker.ResetTimeoutSeconds <= 0 {
			return fmt.Errorf("billing.circuit_breaker.reset_timeout_seconds must be positive")
		}
		if c.Billing.CircuitBreaker.HalfOpenRequests <= 0 {
			return fmt.Errorf("billing.circuit_breaker.half_open_requests must be positive")
		}
	}
	if c.Billing.MinimumBalanceReserve < 0 {
		return fmt.Errorf("billing.minimum_balance_reserve must be non-negative")
	}
	if c.Billing.Queue.Enabled {
		if c.Billing.Queue.ConsumerCount <= 0 {
			return fmt.Errorf("billing.queue.consumer_count must be positive")
		}
		if c.Billing.Queue.MaxConsumerCount <= 0 {
			return fmt.Errorf("billing.queue.max_consumer_count must be positive")
		}
		if c.Billing.Queue.ConsumerCount > c.Billing.Queue.MaxConsumerCount {
			return fmt.Errorf("billing.queue.consumer_count cannot exceed max_consumer_count")
		}
		if c.Billing.Queue.ReadBatchSize <= 0 {
			return fmt.Errorf("billing.queue.read_batch_size must be positive")
		}
		if c.Billing.Queue.ReadBlockMilliseconds <= 0 {
			return fmt.Errorf("billing.queue.read_block_milliseconds must be positive")
		}
		if c.Billing.Queue.CommandTimeoutSeconds <= 0 {
			return fmt.Errorf("billing.queue.command_timeout_seconds must be positive")
		}
		if c.Billing.Queue.MaxRetryDelaySeconds <= 0 {
			return fmt.Errorf("billing.queue.max_retry_delay_seconds must be positive")
		}
	}
	if c.Database.MaxOpenConns <= 0 {
		return fmt.Errorf("database.max_open_conns must be positive")
	}
	if c.Database.MaxIdleConns < 0 {
		return fmt.Errorf("database.max_idle_conns must be non-negative")
	}
	if c.Database.MaxIdleConns > c.Database.MaxOpenConns {
		return fmt.Errorf("database.max_idle_conns cannot exceed database.max_open_conns")
	}
	if c.Database.ConnMaxLifetimeMinutes < 0 {
		return fmt.Errorf("database.conn_max_lifetime_minutes must be non-negative")
	}
	if c.Database.ConnMaxIdleTimeMinutes < 0 {
		return fmt.Errorf("database.conn_max_idle_time_minutes must be non-negative")
	}
	if c.Redis.DialTimeoutSeconds <= 0 {
		return fmt.Errorf("redis.dial_timeout_seconds must be positive")
	}
	if c.Redis.ReadTimeoutSeconds <= 0 {
		return fmt.Errorf("redis.read_timeout_seconds must be positive")
	}
	if c.Redis.WriteTimeoutSeconds <= 0 {
		return fmt.Errorf("redis.write_timeout_seconds must be positive")
	}
	if c.Redis.PoolSize <= 0 {
		return fmt.Errorf("redis.pool_size must be positive")
	}
	if c.Redis.MinIdleConns < 0 {
		return fmt.Errorf("redis.min_idle_conns must be non-negative")
	}
	if c.Redis.MinIdleConns > c.Redis.PoolSize {
		return fmt.Errorf("redis.min_idle_conns cannot exceed redis.pool_size")
	}
	if c.BatchImage.QueueEnabled {
		if strings.TrimSpace(c.BatchImage.QueueReadyKey) == "" {
			return fmt.Errorf("batch_image.queue_ready_key must not be empty")
		}
		if strings.TrimSpace(c.BatchImage.QueueDelayedKey) == "" {
			return fmt.Errorf("batch_image.queue_delayed_key must not be empty")
		}
		if strings.TrimSpace(c.BatchImage.QueueActiveKey) == "" {
			return fmt.Errorf("batch_image.queue_active_key must not be empty")
		}
		if strings.TrimSpace(c.BatchImage.InflightKeyPrefix) == "" {
			return fmt.Errorf("batch_image.inflight_key_prefix must not be empty")
		}
		if strings.TrimSpace(c.BatchImage.LockKeyPrefix) == "" {
			return fmt.Errorf("batch_image.lock_key_prefix must not be empty")
		}
		if c.BatchImage.InflightTTLSeconds <= 0 {
			return fmt.Errorf("batch_image.inflight_ttl_seconds must be positive")
		}
		if c.BatchImage.JobLockTTLSeconds <= 0 {
			return fmt.Errorf("batch_image.job_lock_ttl_seconds must be positive")
		}
		if c.BatchImage.StaleActiveAfterSeconds <= 0 {
			return fmt.Errorf("batch_image.stale_active_after_seconds must be positive")
		}
		if c.BatchImage.DelayedMoveLimit <= 0 {
			return fmt.Errorf("batch_image.delayed_move_limit must be positive")
		}
		if c.BatchImage.RecoverLimit <= 0 {
			return fmt.Errorf("batch_image.recover_limit must be positive")
		}
	}
	if c.BatchImage.VertexEnabled {
		if strings.TrimSpace(c.BatchImage.VertexManagedGCSBucket) == "" {
			return fmt.Errorf("batch_image.vertex_managed_gcs_bucket must not be empty when vertex is enabled")
		}
		if strings.Contains(c.BatchImage.VertexManagedGCSBucket, "://") {
			return fmt.Errorf("batch_image.vertex_managed_gcs_bucket must be a bucket name, not a URI")
		}
		if strings.TrimSpace(c.BatchImage.VertexLocation) == "" {
			return fmt.Errorf("batch_image.vertex_location must not be empty when vertex is enabled")
		}
		if strings.TrimSpace(c.BatchImage.VertexManagedGCSPrefix) == "" {
			return fmt.Errorf("batch_image.vertex_managed_gcs_prefix must not be empty when vertex is enabled")
		}
		if !strings.Contains(c.BatchImage.VertexManagedGCSPrefix, "{batch_id}") {
			return fmt.Errorf("batch_image.vertex_managed_gcs_prefix must contain {batch_id}")
		}
		if c.BatchImage.VertexInputRetentionHours <= 0 {
			return fmt.Errorf("batch_image.vertex_input_retention_hours must be positive")
		}
		if c.BatchImage.VertexOutputRetentionHours <= 0 {
			return fmt.Errorf("batch_image.vertex_output_retention_hours must be positive")
		}
	}
	if c.Dashboard.Enabled {
		if c.Dashboard.StatsFreshTTLSeconds <= 0 {
			return fmt.Errorf("dashboard_cache.stats_fresh_ttl_seconds must be positive")
		}
		if c.Dashboard.StatsTTLSeconds <= 0 {
			return fmt.Errorf("dashboard_cache.stats_ttl_seconds must be positive")
		}
		if c.Dashboard.StatsRefreshTimeoutSeconds <= 0 {
			return fmt.Errorf("dashboard_cache.stats_refresh_timeout_seconds must be positive")
		}
		if c.Dashboard.StatsFreshTTLSeconds > c.Dashboard.StatsTTLSeconds {
			return fmt.Errorf("dashboard_cache.stats_fresh_ttl_seconds must be <= dashboard_cache.stats_ttl_seconds")
		}
	} else {
		if c.Dashboard.StatsFreshTTLSeconds < 0 {
			return fmt.Errorf("dashboard_cache.stats_fresh_ttl_seconds must be non-negative")
		}
		if c.Dashboard.StatsTTLSeconds < 0 {
			return fmt.Errorf("dashboard_cache.stats_ttl_seconds must be non-negative")
		}
		if c.Dashboard.StatsRefreshTimeoutSeconds < 0 {
			return fmt.Errorf("dashboard_cache.stats_refresh_timeout_seconds must be non-negative")
		}
	}
	if c.DashboardAgg.Enabled {
		if c.DashboardAgg.IntervalSeconds <= 0 {
			return fmt.Errorf("dashboard_aggregation.interval_seconds must be positive")
		}
		if c.DashboardAgg.LookbackSeconds < 0 {
			return fmt.Errorf("dashboard_aggregation.lookback_seconds must be non-negative")
		}
		if c.DashboardAgg.BackfillMaxDays < 0 {
			return fmt.Errorf("dashboard_aggregation.backfill_max_days must be non-negative")
		}
		if c.DashboardAgg.BackfillEnabled && c.DashboardAgg.BackfillMaxDays == 0 {
			return fmt.Errorf("dashboard_aggregation.backfill_max_days must be positive")
		}
		if c.DashboardAgg.Retention.UsageLogsDays <= 0 {
			return fmt.Errorf("dashboard_aggregation.retention.usage_logs_days must be positive")
		}
		if c.DashboardAgg.Retention.UsageBillingDedupDays <= 0 {
			return fmt.Errorf("dashboard_aggregation.retention.usage_billing_dedup_days must be positive")
		}
		if c.DashboardAgg.Retention.UsageBillingDedupDays < c.DashboardAgg.Retention.UsageLogsDays {
			return fmt.Errorf("dashboard_aggregation.retention.usage_billing_dedup_days must be greater than or equal to usage_logs_days")
		}
		if c.DashboardAgg.Retention.HourlyDays <= 0 {
			return fmt.Errorf("dashboard_aggregation.retention.hourly_days must be positive")
		}
		if c.DashboardAgg.Retention.DailyDays <= 0 {
			return fmt.Errorf("dashboard_aggregation.retention.daily_days must be positive")
		}
		if c.DashboardAgg.RecomputeDays < 0 {
			return fmt.Errorf("dashboard_aggregation.recompute_days must be non-negative")
		}
	} else {
		if c.DashboardAgg.IntervalSeconds < 0 {
			return fmt.Errorf("dashboard_aggregation.interval_seconds must be non-negative")
		}
		if c.DashboardAgg.LookbackSeconds < 0 {
			return fmt.Errorf("dashboard_aggregation.lookback_seconds must be non-negative")
		}
		if c.DashboardAgg.BackfillMaxDays < 0 {
			return fmt.Errorf("dashboard_aggregation.backfill_max_days must be non-negative")
		}
		if c.DashboardAgg.Retention.UsageLogsDays < 0 {
			return fmt.Errorf("dashboard_aggregation.retention.usage_logs_days must be non-negative")
		}
		if c.DashboardAgg.Retention.UsageBillingDedupDays < 0 {
			return fmt.Errorf("dashboard_aggregation.retention.usage_billing_dedup_days must be non-negative")
		}
		if c.DashboardAgg.Retention.UsageBillingDedupDays > 0 &&
			c.DashboardAgg.Retention.UsageLogsDays > 0 &&
			c.DashboardAgg.Retention.UsageBillingDedupDays < c.DashboardAgg.Retention.UsageLogsDays {
			return fmt.Errorf("dashboard_aggregation.retention.usage_billing_dedup_days must be greater than or equal to usage_logs_days")
		}
		if c.DashboardAgg.Retention.HourlyDays < 0 {
			return fmt.Errorf("dashboard_aggregation.retention.hourly_days must be non-negative")
		}
		if c.DashboardAgg.Retention.DailyDays < 0 {
			return fmt.Errorf("dashboard_aggregation.retention.daily_days must be non-negative")
		}
		if c.DashboardAgg.RecomputeDays < 0 {
			return fmt.Errorf("dashboard_aggregation.recompute_days must be non-negative")
		}
	}
	if c.UsageCleanup.Enabled {
		if c.UsageCleanup.MaxRangeDays <= 0 {
			return fmt.Errorf("usage_cleanup.max_range_days must be positive")
		}
		if c.UsageCleanup.BatchSize <= 0 {
			return fmt.Errorf("usage_cleanup.batch_size must be positive")
		}
		if c.UsageCleanup.WorkerIntervalSeconds <= 0 {
			return fmt.Errorf("usage_cleanup.worker_interval_seconds must be positive")
		}
		if c.UsageCleanup.TaskTimeoutSeconds <= 0 {
			return fmt.Errorf("usage_cleanup.task_timeout_seconds must be positive")
		}
	} else {
		if c.UsageCleanup.MaxRangeDays < 0 {
			return fmt.Errorf("usage_cleanup.max_range_days must be non-negative")
		}
		if c.UsageCleanup.BatchSize < 0 {
			return fmt.Errorf("usage_cleanup.batch_size must be non-negative")
		}
		if c.UsageCleanup.WorkerIntervalSeconds < 0 {
			return fmt.Errorf("usage_cleanup.worker_interval_seconds must be non-negative")
		}
		if c.UsageCleanup.TaskTimeoutSeconds < 0 {
			return fmt.Errorf("usage_cleanup.task_timeout_seconds must be non-negative")
		}
	}
	if c.Idempotency.DefaultTTLSeconds <= 0 {
		return fmt.Errorf("idempotency.default_ttl_seconds must be positive")
	}
	if c.Idempotency.SystemOperationTTLSeconds <= 0 {
		return fmt.Errorf("idempotency.system_operation_ttl_seconds must be positive")
	}
	if c.Idempotency.ProcessingTimeoutSeconds <= 0 {
		return fmt.Errorf("idempotency.processing_timeout_seconds must be positive")
	}
	if c.Idempotency.FailedRetryBackoffSeconds <= 0 {
		return fmt.Errorf("idempotency.failed_retry_backoff_seconds must be positive")
	}
	if c.Idempotency.MaxStoredResponseLen <= 0 {
		return fmt.Errorf("idempotency.max_stored_response_len must be positive")
	}
	if c.Idempotency.CleanupIntervalSeconds <= 0 {
		return fmt.Errorf("idempotency.cleanup_interval_seconds must be positive")
	}
	if c.Idempotency.CleanupBatchSize <= 0 {
		return fmt.Errorf("idempotency.cleanup_batch_size must be positive")
	}
	if c.Gateway.MaxBodySize <= 0 {
		return fmt.Errorf("gateway.max_body_size must be positive")
	}
	if c.Gateway.TextMaxBodySize <= 0 || c.Gateway.TextMaxBodySize > c.Gateway.MaxBodySize {
		return fmt.Errorf("gateway.text_max_body_size must be positive and no greater than gateway.max_body_size")
	}
	if c.Gateway.UpstreamResponseReadMaxBytes <= 0 {
		return fmt.Errorf("gateway.upstream_response_read_max_bytes must be positive")
	}
	if c.Gateway.ProxyProbeResponseReadMaxBytes <= 0 {
		return fmt.Errorf("gateway.proxy_probe_response_read_max_bytes must be positive")
	}
	if c.Gateway.ResponseHeaderTimeout < 0 {
		return fmt.Errorf("gateway.response_header_timeout must be non-negative")
	}
	if c.Gateway.OpenAIFirstOutputTimeoutSeconds < 0 || c.Gateway.OpenAIFirstOutputTimeoutSeconds > 600 ||
		(c.Gateway.OpenAIFirstOutputTimeoutSeconds > 0 && c.Gateway.OpenAIFirstOutputTimeoutSeconds < 30) {
		return fmt.Errorf("gateway.openai_first_output_timeout_seconds must be 0 or between 30-600 seconds")
	}
	if c.Gateway.OpenAIHighEffortFirstOutputTimeoutSeconds < 0 || c.Gateway.OpenAIHighEffortFirstOutputTimeoutSeconds > 1800 ||
		(c.Gateway.OpenAIHighEffortFirstOutputTimeoutSeconds > 0 && c.Gateway.OpenAIHighEffortFirstOutputTimeoutSeconds < 30) {
		return fmt.Errorf("gateway.openai_high_effort_first_output_timeout_seconds must be 0 or between 30-1800 seconds")
	}
	if strings.TrimSpace(c.Gateway.ConnectionPoolIsolation) != "" {
		switch c.Gateway.ConnectionPoolIsolation {
		case ConnectionPoolIsolationProxy, ConnectionPoolIsolationAccount, ConnectionPoolIsolationAccountProxy:
		default:
			return fmt.Errorf("gateway.connection_pool_isolation must be one of: %s/%s/%s",
				ConnectionPoolIsolationProxy, ConnectionPoolIsolationAccount, ConnectionPoolIsolationAccountProxy)
		}
	}
	if c.Gateway.ImageConcurrency.MaxConcurrentRequests < 0 {
		return fmt.Errorf("gateway.image_concurrency.max_concurrent_requests must be non-negative")
	}
	switch strings.TrimSpace(c.Gateway.ImageConcurrency.OverflowMode) {
	case "", ImageConcurrencyOverflowModeReject, ImageConcurrencyOverflowModeWait:
	default:
		return fmt.Errorf("gateway.image_concurrency.overflow_mode must be one of: %s/%s",
			ImageConcurrencyOverflowModeReject, ImageConcurrencyOverflowModeWait)
	}
	if c.Gateway.ImageConcurrency.WaitTimeoutSeconds < 0 {
		return fmt.Errorf("gateway.image_concurrency.wait_timeout_seconds must be non-negative")
	}
	if c.Gateway.ImageConcurrency.MaxWaitingRequests < 0 {
		return fmt.Errorf("gateway.image_concurrency.max_waiting_requests must be non-negative")
	}
	if c.Gateway.MaxIdleConns <= 0 {
		return fmt.Errorf("gateway.max_idle_conns must be positive")
	}
	if c.Gateway.MaxIdleConnsPerHost <= 0 {
		return fmt.Errorf("gateway.max_idle_conns_per_host must be positive")
	}
	if c.Gateway.MaxConnsPerHost < 0 {
		return fmt.Errorf("gateway.max_conns_per_host must be non-negative")
	}
	if c.Gateway.IdleConnTimeoutSeconds <= 0 {
		return fmt.Errorf("gateway.idle_conn_timeout_seconds must be positive")
	}
	if c.Gateway.IdleConnTimeoutSeconds > 180 {
		slog.Warn("gateway.idle_conn_timeout_seconds is high; consider 60-120 seconds for better connection reuse", "idle_conn_timeout_seconds", c.Gateway.IdleConnTimeoutSeconds)
	}
	if c.Gateway.MaxUpstreamClients <= 0 {
		return fmt.Errorf("gateway.max_upstream_clients must be positive")
	}
	if c.Gateway.ClientIdleTTLSeconds <= 0 {
		return fmt.Errorf("gateway.client_idle_ttl_seconds must be positive")
	}
	if c.Gateway.ConcurrencySlotTTLMinutes <= 0 {
		return fmt.Errorf("gateway.concurrency_slot_ttl_minutes must be positive")
	}
	if c.Gateway.StreamDataIntervalTimeout < 0 {
		return fmt.Errorf("gateway.stream_data_interval_timeout must be non-negative")
	}
	if c.Gateway.StreamDataIntervalTimeout != 0 &&
		(c.Gateway.StreamDataIntervalTimeout < 30 || c.Gateway.StreamDataIntervalTimeout > 300) {
		return fmt.Errorf("gateway.stream_data_interval_timeout must be 0 or between 30-300 seconds")
	}
	if c.Gateway.StreamKeepaliveInterval < 0 {
		return fmt.Errorf("gateway.stream_keepalive_interval must be non-negative")
	}
	if c.Gateway.StreamKeepaliveInterval != 0 &&
		(c.Gateway.StreamKeepaliveInterval < 5 || c.Gateway.StreamKeepaliveInterval > 30) {
		return fmt.Errorf("gateway.stream_keepalive_interval must be 0 or between 5-30 seconds")
	}
	if c.Gateway.ImageStreamDataIntervalTimeout < 0 {
		return fmt.Errorf("gateway.image_stream_data_interval_timeout must be non-negative")
	}
	if c.Gateway.ImageStreamDataIntervalTimeout != 0 &&
		(c.Gateway.ImageStreamDataIntervalTimeout < 60 || c.Gateway.ImageStreamDataIntervalTimeout > 1800) {
		return fmt.Errorf("gateway.image_stream_data_interval_timeout must be 0 or between 60-1800 seconds")
	}
	if c.Gateway.ImageStreamKeepaliveInterval < 0 {
		return fmt.Errorf("gateway.image_stream_keepalive_interval must be non-negative")
	}
	if c.Gateway.ImageStreamKeepaliveInterval != 0 &&
		(c.Gateway.ImageStreamKeepaliveInterval < 5 || c.Gateway.ImageStreamKeepaliveInterval > 60) {
		return fmt.Errorf("gateway.image_stream_keepalive_interval must be 0 or between 5-60 seconds")
	}
	if c.Gateway.ImageNonstreamKeepaliveInterval < 0 {
		return fmt.Errorf("gateway.image_nonstream_keepalive_interval must be non-negative")
	}
	if c.Gateway.ImageNonstreamKeepaliveInterval != 0 &&
		(c.Gateway.ImageNonstreamKeepaliveInterval < 5 || c.Gateway.ImageNonstreamKeepaliveInterval > 60) {
		return fmt.Errorf("gateway.image_nonstream_keepalive_interval must be 0 or between 5-60 seconds")
	}
	// 兼容旧键 sticky_previous_response_ttl_seconds
	if c.Gateway.OpenAIWS.StickyResponseIDTTLSeconds <= 0 && c.Gateway.OpenAIWS.StickyPreviousResponseTTLSeconds > 0 {
		c.Gateway.OpenAIWS.StickyResponseIDTTLSeconds = c.Gateway.OpenAIWS.StickyPreviousResponseTTLSeconds
	}
	if c.Gateway.OpenAIWS.MaxConnsPerAccount <= 0 {
		return fmt.Errorf("gateway.openai_ws.max_conns_per_account must be positive")
	}
	if c.Gateway.OpenAIWS.ClientFirstMessageTimeoutSeconds <= 0 {
		return fmt.Errorf("gateway.openai_ws.client_first_message_timeout_seconds must be positive")
	}
	if c.Gateway.OpenAIWS.IngressInterTurnIdleTimeoutSeconds < 0 {
		return fmt.Errorf("gateway.openai_ws.ingress_inter_turn_idle_timeout_seconds must be non-negative")
	}
	if c.Gateway.OpenAIWS.MaxIngressConnectionsPerAPIKey < 0 {
		return fmt.Errorf("gateway.openai_ws.max_ingress_connections_per_api_key must be non-negative")
	}
	if c.Gateway.OpenAIWS.MinIdlePerAccount < 0 {
		return fmt.Errorf("gateway.openai_ws.min_idle_per_account must be non-negative")
	}
	if c.Gateway.OpenAIWS.MaxIdlePerAccount < 0 {
		return fmt.Errorf("gateway.openai_ws.max_idle_per_account must be non-negative")
	}
	if c.Gateway.OpenAIWS.MinIdlePerAccount > c.Gateway.OpenAIWS.MaxIdlePerAccount {
		return fmt.Errorf("gateway.openai_ws.min_idle_per_account must be <= max_idle_per_account")
	}
	if c.Gateway.OpenAIWS.MaxIdlePerAccount > c.Gateway.OpenAIWS.MaxConnsPerAccount {
		return fmt.Errorf("gateway.openai_ws.max_idle_per_account must be <= max_conns_per_account")
	}
	if c.Gateway.OpenAIWS.OAuthMaxConnsFactor <= 0 {
		return fmt.Errorf("gateway.openai_ws.oauth_max_conns_factor must be positive")
	}
	if c.Gateway.OpenAIWS.APIKeyMaxConnsFactor <= 0 {
		return fmt.Errorf("gateway.openai_ws.apikey_max_conns_factor must be positive")
	}
	if c.Gateway.OpenAIWS.DialTimeoutSeconds <= 0 {
		return fmt.Errorf("gateway.openai_ws.dial_timeout_seconds must be positive")
	}
	if c.Gateway.OpenAIWS.ReadTimeoutSeconds <= 0 {
		return fmt.Errorf("gateway.openai_ws.read_timeout_seconds must be positive")
	}
	if c.Gateway.OpenAIWS.WriteTimeoutSeconds <= 0 {
		return fmt.Errorf("gateway.openai_ws.write_timeout_seconds must be positive")
	}
	if c.Gateway.OpenAIWS.PoolTargetUtilization <= 0 || c.Gateway.OpenAIWS.PoolTargetUtilization > 1 {
		return fmt.Errorf("gateway.openai_ws.pool_target_utilization must be within (0,1]")
	}
	if c.Gateway.OpenAIWS.QueueLimitPerConn <= 0 {
		return fmt.Errorf("gateway.openai_ws.queue_limit_per_conn must be positive")
	}
	if c.Gateway.OpenAIWS.EventFlushBatchSize <= 0 {
		return fmt.Errorf("gateway.openai_ws.event_flush_batch_size must be positive")
	}
	if c.Gateway.OpenAIWS.EventFlushIntervalMS < 0 {
		return fmt.Errorf("gateway.openai_ws.event_flush_interval_ms must be non-negative")
	}
	if c.Gateway.OpenAIWS.PrewarmCooldownMS < 0 {
		return fmt.Errorf("gateway.openai_ws.prewarm_cooldown_ms must be non-negative")
	}
	if c.Gateway.OpenAIWS.ClientReadLimitBytes <= 0 {
		return fmt.Errorf("gateway.openai_ws.client_read_limit_bytes must be positive")
	}
	if c.Gateway.OpenAIWS.HTTPBridgeThresholdBytes < 0 {
		return fmt.Errorf("gateway.openai_ws.http_bridge_threshold_bytes must be non-negative")
	}
	if c.Gateway.OpenAIWS.HTTPBridgeEnabled && c.Gateway.OpenAIWS.HTTPBridgeThresholdBytes == 0 {
		return fmt.Errorf("gateway.openai_ws.http_bridge_threshold_bytes must be positive when http_bridge_enabled is true")
	}
	if c.Gateway.OpenAIWS.FallbackCooldownSeconds < 0 {
		return fmt.Errorf("gateway.openai_ws.fallback_cooldown_seconds must be non-negative")
	}
	if c.Gateway.OpenAIWS.RetryBackoffInitialMS < 0 {
		return fmt.Errorf("gateway.openai_ws.retry_backoff_initial_ms must be non-negative")
	}
	if c.Gateway.OpenAIWS.RetryBackoffMaxMS < 0 {
		return fmt.Errorf("gateway.openai_ws.retry_backoff_max_ms must be non-negative")
	}
	if c.Gateway.OpenAIWS.RetryBackoffInitialMS > 0 && c.Gateway.OpenAIWS.RetryBackoffMaxMS > 0 &&
		c.Gateway.OpenAIWS.RetryBackoffMaxMS < c.Gateway.OpenAIWS.RetryBackoffInitialMS {
		return fmt.Errorf("gateway.openai_ws.retry_backoff_max_ms must be >= retry_backoff_initial_ms")
	}
	if c.Gateway.OpenAIWS.RetryJitterRatio < 0 || c.Gateway.OpenAIWS.RetryJitterRatio > 1 {
		return fmt.Errorf("gateway.openai_ws.retry_jitter_ratio must be within [0,1]")
	}
	if c.Gateway.OpenAIWS.RetryTotalBudgetMS < 0 {
		return fmt.Errorf("gateway.openai_ws.retry_total_budget_ms must be non-negative")
	}
	if mode := strings.ToLower(strings.TrimSpace(c.Gateway.OpenAIWS.IngressModeDefault)); mode != "" {
		switch mode {
		case "off", "ctx_pool", "passthrough", "http_bridge":
		case "shared", "dedicated":
			slog.Warn("gateway.openai_ws.ingress_mode_default is deprecated, treating as ctx_pool; please update to off|ctx_pool|passthrough|http_bridge", "value", mode)
		default:
			return fmt.Errorf("gateway.openai_ws.ingress_mode_default must be one of off|ctx_pool|passthrough|http_bridge")
		}
	}
	if mode := strings.ToLower(strings.TrimSpace(c.Gateway.OpenAIWS.StoreDisabledConnMode)); mode != "" {
		switch mode {
		case "strict", "adaptive", "off":
		default:
			return fmt.Errorf("gateway.openai_ws.store_disabled_conn_mode must be one of strict|adaptive|off")
		}
	}
	if c.Gateway.OpenAIWS.PayloadLogSampleRate < 0 || c.Gateway.OpenAIWS.PayloadLogSampleRate > 1 {
		return fmt.Errorf("gateway.openai_ws.payload_log_sample_rate must be within [0,1]")
	}
	if c.Gateway.OpenAIWS.LBTopK <= 0 {
		return fmt.Errorf("gateway.openai_ws.lb_top_k must be positive")
	}
	if c.Gateway.OpenAIWS.StickySessionTTLSeconds <= 0 {
		return fmt.Errorf("gateway.openai_ws.sticky_session_ttl_seconds must be positive")
	}
	if c.Gateway.OpenAIWS.StickyResponseIDTTLSeconds <= 0 {
		return fmt.Errorf("gateway.openai_ws.sticky_response_id_ttl_seconds must be positive")
	}
	if c.Gateway.OpenAIWS.StickyPreviousResponseTTLSeconds < 0 {
		return fmt.Errorf("gateway.openai_ws.sticky_previous_response_ttl_seconds must be non-negative")
	}
	if c.Gateway.OpenAIHTTP2.FallbackErrorThreshold < 0 {
		return fmt.Errorf("gateway.openai_http2.fallback_error_threshold must be non-negative")
	}
	if c.Gateway.OpenAIHTTP2.FallbackWindowSeconds < 0 {
		return fmt.Errorf("gateway.openai_http2.fallback_window_seconds must be non-negative")
	}
	if c.Gateway.OpenAIHTTP2.FallbackTTLSeconds < 0 {
		return fmt.Errorf("gateway.openai_http2.fallback_ttl_seconds must be non-negative")
	}
	weights := c.Gateway.OpenAIWS.SchedulerScoreWeights
	for _, weight := range []float64{
		weights.Priority, weights.Load, weights.Queue, weights.ErrorRate, weights.TTFT,
		weights.Reset, weights.QuotaHeadroom, weights.UpstreamCost,
		weights.PreviousResponse, weights.SessionSticky,
	} {
		if weight < 0 || math.IsNaN(weight) || math.IsInf(weight, 0) {
			return fmt.Errorf("gateway.openai_ws.scheduler_score_weights.* must be non-negative and finite")
		}
	}
	weightSum := weights.BaseWeightSum()
	if weightSum <= 0 {
		return fmt.Errorf("gateway.openai_ws.scheduler_score_weights must not all be zero")
	}
	if math.IsNaN(weightSum) || math.IsInf(weightSum, 0) {
		return fmt.Errorf("gateway.openai_ws.scheduler_score_weights base-weight sum must be finite")
	}
	if totalWeightSum := weights.TotalWeightSum(); math.IsNaN(totalWeightSum) || math.IsInf(totalWeightSum, 0) {
		return fmt.Errorf("gateway.openai_ws.scheduler_score_weights total-weight sum must be finite")
	}
	if c.Gateway.OpenAIScheduler.StickyEscapeTTFTMs <= 0 {
		return fmt.Errorf("gateway.openai_scheduler.sticky_escape_ttft_ms must be positive")
	}
	if c.Gateway.OpenAIScheduler.StickyEscapeErrorRate < 0 || c.Gateway.OpenAIScheduler.StickyEscapeErrorRate > 1 {
		return fmt.Errorf("gateway.openai_scheduler.sticky_escape_error_rate must be between 0 and 1")
	}
	if c.Gateway.MaxLineSize < 0 {
		return fmt.Errorf("gateway.max_line_size must be non-negative")
	}
	if c.Gateway.MaxLineSize != 0 && c.Gateway.MaxLineSize < 1024*1024 {
		return fmt.Errorf("gateway.max_line_size must be at least 1MB")
	}
	if c.Gateway.UsageRecord.WorkerCount <= 0 {
		return fmt.Errorf("gateway.usage_record.worker_count must be positive")
	}
	if c.Gateway.UsageRecord.QueueSize <= 0 {
		return fmt.Errorf("gateway.usage_record.queue_size must be positive")
	}
	if c.Gateway.UsageRecord.TaskTimeoutSeconds <= 0 {
		return fmt.Errorf("gateway.usage_record.task_timeout_seconds must be positive")
	}
	switch strings.ToLower(strings.TrimSpace(c.Gateway.UsageRecord.OverflowPolicy)) {
	case UsageRecordOverflowPolicyDrop, UsageRecordOverflowPolicySample, UsageRecordOverflowPolicySync:
	default:
		return fmt.Errorf("gateway.usage_record.overflow_policy must be one of: %s/%s/%s",
			UsageRecordOverflowPolicyDrop, UsageRecordOverflowPolicySample, UsageRecordOverflowPolicySync)
	}
	if c.Gateway.UsageRecord.OverflowSamplePercent < 0 || c.Gateway.UsageRecord.OverflowSamplePercent > 100 {
		return fmt.Errorf("gateway.usage_record.overflow_sample_percent must be between 0-100")
	}
	if strings.EqualFold(strings.TrimSpace(c.Gateway.UsageRecord.OverflowPolicy), UsageRecordOverflowPolicySample) &&
		c.Gateway.UsageRecord.OverflowSamplePercent <= 0 {
		return fmt.Errorf("gateway.usage_record.overflow_sample_percent must be positive when overflow_policy=sample")
	}
	if c.Gateway.UsageRecord.AutoScaleEnabled {
		if c.Gateway.UsageRecord.AutoScaleMinWorkers <= 0 {
			return fmt.Errorf("gateway.usage_record.auto_scale_min_workers must be positive")
		}
		if c.Gateway.UsageRecord.AutoScaleMaxWorkers <= 0 {
			return fmt.Errorf("gateway.usage_record.auto_scale_max_workers must be positive")
		}
		if c.Gateway.UsageRecord.AutoScaleMaxWorkers < c.Gateway.UsageRecord.AutoScaleMinWorkers {
			return fmt.Errorf("gateway.usage_record.auto_scale_max_workers must be >= auto_scale_min_workers")
		}
		if c.Gateway.UsageRecord.WorkerCount < c.Gateway.UsageRecord.AutoScaleMinWorkers ||
			c.Gateway.UsageRecord.WorkerCount > c.Gateway.UsageRecord.AutoScaleMaxWorkers {
			return fmt.Errorf("gateway.usage_record.worker_count must be between auto_scale_min_workers and auto_scale_max_workers")
		}
		if c.Gateway.UsageRecord.AutoScaleUpQueuePercent <= 0 || c.Gateway.UsageRecord.AutoScaleUpQueuePercent > 100 {
			return fmt.Errorf("gateway.usage_record.auto_scale_up_queue_percent must be between 1-100")
		}
		if c.Gateway.UsageRecord.AutoScaleDownQueuePercent < 0 || c.Gateway.UsageRecord.AutoScaleDownQueuePercent >= 100 {
			return fmt.Errorf("gateway.usage_record.auto_scale_down_queue_percent must be between 0-99")
		}
		if c.Gateway.UsageRecord.AutoScaleDownQueuePercent >= c.Gateway.UsageRecord.AutoScaleUpQueuePercent {
			return fmt.Errorf("gateway.usage_record.auto_scale_down_queue_percent must be less than auto_scale_up_queue_percent")
		}
		if c.Gateway.UsageRecord.AutoScaleUpStep <= 0 {
			return fmt.Errorf("gateway.usage_record.auto_scale_up_step must be positive")
		}
		if c.Gateway.UsageRecord.AutoScaleDownStep <= 0 {
			return fmt.Errorf("gateway.usage_record.auto_scale_down_step must be positive")
		}
		if c.Gateway.UsageRecord.AutoScaleCheckIntervalSeconds <= 0 {
			return fmt.Errorf("gateway.usage_record.auto_scale_check_interval_seconds must be positive")
		}
		if c.Gateway.UsageRecord.AutoScaleCooldownSeconds < 0 {
			return fmt.Errorf("gateway.usage_record.auto_scale_cooldown_seconds must be non-negative")
		}
	}
	if c.Gateway.UserGroupRateCacheTTLSeconds <= 0 {
		return fmt.Errorf("gateway.user_group_rate_cache_ttl_seconds must be positive")
	}
	if c.Gateway.ModelsListCacheTTLSeconds < 10 || c.Gateway.ModelsListCacheTTLSeconds > 30 {
		return fmt.Errorf("gateway.models_list_cache_ttl_seconds must be between 10-30")
	}
	if c.Gateway.Scheduling.StickySessionMaxWaiting <= 0 {
		return fmt.Errorf("gateway.scheduling.sticky_session_max_waiting must be positive")
	}
	if c.Gateway.Scheduling.StickySessionWaitTimeout <= 0 {
		return fmt.Errorf("gateway.scheduling.sticky_session_wait_timeout must be positive")
	}
	if c.Gateway.Scheduling.FallbackWaitTimeout <= 0 {
		return fmt.Errorf("gateway.scheduling.fallback_wait_timeout must be positive")
	}
	if c.Gateway.Scheduling.FallbackMaxWaiting <= 0 {
		return fmt.Errorf("gateway.scheduling.fallback_max_waiting must be positive")
	}
	if c.Gateway.Scheduling.LoadBatchCacheTTLMS < 0 {
		return fmt.Errorf("gateway.scheduling.load_batch_cache_ttl_ms must be non-negative")
	}
	if c.Gateway.Scheduling.SnapshotMGetChunkSize <= 0 {
		return fmt.Errorf("gateway.scheduling.snapshot_mget_chunk_size must be positive")
	}
	if c.Gateway.Scheduling.SnapshotWriteChunkSize <= 0 {
		return fmt.Errorf("gateway.scheduling.snapshot_write_chunk_size must be positive")
	}
	if c.Gateway.Scheduling.SlotCleanupInterval < 0 {
		return fmt.Errorf("gateway.scheduling.slot_cleanup_interval must be non-negative")
	}
	if c.Gateway.Scheduling.DbFallbackTimeoutSeconds < 0 {
		return fmt.Errorf("gateway.scheduling.db_fallback_timeout_seconds must be non-negative")
	}
	if c.Gateway.Scheduling.DbFallbackMaxQPS < 0 {
		return fmt.Errorf("gateway.scheduling.db_fallback_max_qps must be non-negative")
	}
	if c.Gateway.Scheduling.OutboxPollIntervalSeconds <= 0 {
		return fmt.Errorf("gateway.scheduling.outbox_poll_interval_seconds must be positive")
	}
	if c.Gateway.Scheduling.OutboxLagWarnSeconds < 0 {
		return fmt.Errorf("gateway.scheduling.outbox_lag_warn_seconds must be non-negative")
	}
	if c.Gateway.Scheduling.OutboxLagRebuildSeconds < 0 {
		return fmt.Errorf("gateway.scheduling.outbox_lag_rebuild_seconds must be non-negative")
	}
	if c.Gateway.Scheduling.OutboxLagRebuildFailures <= 0 {
		return fmt.Errorf("gateway.scheduling.outbox_lag_rebuild_failures must be positive")
	}
	if c.Gateway.Scheduling.OutboxBacklogRebuildRows < 0 {
		return fmt.Errorf("gateway.scheduling.outbox_backlog_rebuild_rows must be non-negative")
	}
	if c.Gateway.Scheduling.FullRebuildIntervalSeconds < 0 {
		return fmt.Errorf("gateway.scheduling.full_rebuild_interval_seconds must be non-negative")
	}
	if c.Gateway.Scheduling.OutboxLagWarnSeconds > 0 &&
		c.Gateway.Scheduling.OutboxLagRebuildSeconds > 0 &&
		c.Gateway.Scheduling.OutboxLagRebuildSeconds < c.Gateway.Scheduling.OutboxLagWarnSeconds {
		return fmt.Errorf("gateway.scheduling.outbox_lag_rebuild_seconds must be >= outbox_lag_warn_seconds")
	}
	if c.Ops.MetricsCollectorCache.TTL < 0 {
		return fmt.Errorf("ops.metrics_collector_cache.ttl must be non-negative")
	}
	if c.Ops.Cleanup.ErrorLogRetentionDays < 0 {
		return fmt.Errorf("ops.cleanup.error_log_retention_days must be non-negative")
	}
	if c.Ops.Cleanup.MinuteMetricsRetentionDays < 0 {
		return fmt.Errorf("ops.cleanup.minute_metrics_retention_days must be non-negative")
	}
	if c.Ops.Cleanup.HourlyMetricsRetentionDays < 0 {
		return fmt.Errorf("ops.cleanup.hourly_metrics_retention_days must be non-negative")
	}
	if c.Ops.Cleanup.Enabled && strings.TrimSpace(c.Ops.Cleanup.Schedule) == "" {
		return fmt.Errorf("ops.cleanup.schedule is required when ops.cleanup.enabled=true")
	}
	if c.Concurrency.PingInterval < 5 || c.Concurrency.PingInterval > 30 {
		return fmt.Errorf("concurrency.ping_interval must be between 5-30 seconds")
	}
	if err := ValidateDingTalkConfig(c.DingTalk); err != nil {
		return fmt.Errorf("dingtalk_connect: %w", err)
	}
	return nil
}

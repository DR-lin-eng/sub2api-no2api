package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"
)

func validateDeployment(c *Config) error {
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
	if len(c.Deployment.NodeID) > 64 {
		return fmt.Errorf("deployment.node_id must not exceed 64 characters")
	}
	if len(c.Deployment.NodeIDFile) > 1024 {
		return fmt.Errorf("deployment.node_id_file must not exceed 1024 characters")
	}
	if c.Deployment.RolloutPollSeconds < 1 || c.Deployment.RolloutPollSeconds > 60 {
		return fmt.Errorf("deployment.rollout_poll_seconds must be between 1 and 60")
	}
	if c.Deployment.RolloutDrainGraceSeconds < 0 || c.Deployment.RolloutDrainGraceSeconds > 300 {
		return fmt.Errorf("deployment.rollout_drain_grace_seconds must be between 0 and 300")
	}
	if c.Deployment.RolloutDrainTimeoutSeconds < 30 || c.Deployment.RolloutDrainTimeoutSeconds > 3600 {
		return fmt.Errorf("deployment.rollout_drain_timeout_seconds must be between 30 and 3600")
	}
	if c.Deployment.RolloutVerifyHeartbeats < 1 || c.Deployment.RolloutVerifyHeartbeats > 10 {
		return fmt.Errorf("deployment.rollout_verify_heartbeats must be between 1 and 10")
	}
	return nil
}

func validateServerRuntime(c *Config) error {
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
	return nil
}

func validateAPIKeyAuth(c *Config) error {
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
	return nil
}

func validateJWTSecret(c *Config) error {
	jwtSecret := strings.TrimSpace(c.JWT.Secret)
	if jwtSecret == "" {
		return fmt.Errorf("jwt.secret is required")
	}
	// NOTE: 按 UTF-8 编码后的字节长度计算。
	// 选择 bytes 而不是 rune 计数，确保二进制/随机串的长度语义更接近“熵”而非“字符数”。
	if len([]byte(jwtSecret)) < 32 {
		return fmt.Errorf("jwt.secret must be at least 32 bytes")
	}
	return nil
}

func validateLogging(c *Config) error {
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
	return nil
}

func validateSubscriptionMaintenance(c *Config) error {
	if c.SubscriptionMaintenance.WorkerCount < 0 {
		return fmt.Errorf("subscription_maintenance.worker_count must be non-negative")
	}
	if c.SubscriptionMaintenance.QueueSize < 0 {
		return fmt.Errorf("subscription_maintenance.queue_size must be non-negative")
	}
	return nil
}

func validateGeminiOAuth(c *Config) error {
	// Gemini OAuth 配置校验：client_id 与 client_secret 必须同时设置或同时留空。
	// 留空时表示使用内置的 Gemini CLI OAuth 客户端（其 client_secret 通过环境变量注入）。
	geminiClientID := strings.TrimSpace(c.Gemini.OAuth.ClientID)
	geminiClientSecret := strings.TrimSpace(c.Gemini.OAuth.ClientSecret)
	if (geminiClientID == "") != (geminiClientSecret == "") {
		return fmt.Errorf("gemini.oauth.client_id and gemini.oauth.client_secret must be both set or both empty")
	}
	return nil
}

func validateServerFrontendURL(c *Config) error {
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
	return nil
}

func validateJWTLifetimes(c *Config) error {
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
	return nil
}

func validateSecurity(c *Config) error {
	if c.Security.CSP.Enabled && strings.TrimSpace(c.Security.CSP.Policy) == "" {
		return fmt.Errorf("security.csp.policy is required when CSP is enabled")
	}
	return nil
}

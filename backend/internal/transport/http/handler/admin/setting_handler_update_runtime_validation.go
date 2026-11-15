package admin

import (
	"net/http"
	"strings"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/response"

	"github.com/gin-gonic/gin"
)

func validateRuntimeSettingsUpdate(c *gin.Context, prepared *preparedSettingsUpdate) bool {
	req := prepared.request

	// Ops metrics collector interval validation (seconds).
	if req.OpsMetricsIntervalSeconds != nil {
		v := *req.OpsMetricsIntervalSeconds
		if v < 60 {
			v = 60
		}
		if v > 3600 {
			v = 3600
		}
		req.OpsMetricsIntervalSeconds = &v
	}
	defaultSubscriptions := make([]service.DefaultSubscriptionSetting, 0, len(req.DefaultSubscriptions))
	for _, sub := range req.DefaultSubscriptions {
		defaultSubscriptions = append(defaultSubscriptions, service.DefaultSubscriptionSetting{
			GroupID:      sub.GroupID,
			ValidityDays: sub.ValidityDays,
		})
	}

	// 验证最低版本号格式（空字符串=禁用，或合法 semver）
	if req.MinClaudeCodeVersion != "" {
		if !semverPattern.MatchString(req.MinClaudeCodeVersion) {
			response.Error(c, http.StatusBadRequest, "min_claude_code_version must be empty or a valid semver (e.g. 2.1.63)")
			return false
		}
	}

	// 验证最高版本号格式（空字符串=禁用，或合法 semver）
	if req.MaxClaudeCodeVersion != "" {
		if !semverPattern.MatchString(req.MaxClaudeCodeVersion) {
			response.Error(c, http.StatusBadRequest, "max_claude_code_version must be empty or a valid semver (e.g. 3.0.0)")
			return false
		}
	}
	if req.AntigravityUserAgentVersion != nil {
		normalized := strings.TrimSpace(*req.AntigravityUserAgentVersion)
		req.AntigravityUserAgentVersion = &normalized
		if normalized != "" && !semverPattern.MatchString(normalized) {
			response.Error(c, http.StatusBadRequest, "antigravity_user_agent_version must be empty or a valid semver (e.g. 1.23.2)")
			return false
		}
	}
	if req.OpenAICodexUserAgent != nil {
		normalized := strings.TrimSpace(*req.OpenAICodexUserAgent)
		req.OpenAICodexUserAgent = &normalized
		// 仅做长度上限保护，不限制具体格式（运维需要可自由调整 codex 版本号）
		if len(normalized) > 512 {
			response.Error(c, http.StatusBadRequest, "openai_codex_user_agent must be at most 512 characters")
			return false
		}
	}
	if req.OpenAICodexClientVersion != nil {
		normalized := strings.TrimSpace(*req.OpenAICodexClientVersion)
		if normalized != "" && service.NormalizeCodexClientVersion(normalized) == "" {
			response.Error(c, http.StatusBadRequest, "openai_codex_client_version must be empty or a valid version (e.g. 0.146.0)")
			return false
		}
		req.OpenAICodexClientVersion = &normalized
	}

	// codex_cli_only 加固：最低/最高 Codex 版本（空=禁用，或合法 semver；max>=min）
	if req.MinCodexVersion != "" && !semverPattern.MatchString(req.MinCodexVersion) {
		response.Error(c, http.StatusBadRequest, "min_codex_version must be empty or a valid semver (e.g. 0.141.0)")
		return false
	}
	if req.MaxCodexVersion != "" && !semverPattern.MatchString(req.MaxCodexVersion) {
		response.Error(c, http.StatusBadRequest, "max_codex_version must be empty or a valid semver (e.g. 0.200.0)")
		return false
	}
	if req.MinCodexVersion != "" && req.MaxCodexVersion != "" && service.CompareVersions(req.MaxCodexVersion, req.MinCodexVersion) < 0 {
		response.Error(c, http.StatusBadRequest, "max_codex_version must be greater than or equal to min_codex_version")
		return false
	}
	// codex_cli_only 黑/白名单：非空须为合法 []AllowedClientEntry JSON。
	// 黑名单 OR 宽 deny（允许 originator-only）；白名单双因子 AND，额外要求每条可命中（非空 originator + ua_contains）。
	if err := service.ValidateCodexClientEntriesJSON(req.CodexCLIOnlyBlacklist); err != nil {
		response.Error(c, http.StatusBadRequest, "codex_cli_only_blacklist "+err.Error())
		return false
	}
	if err := service.ValidateCodexWhitelistEntriesJSON(req.CodexCLIOnlyWhitelist); err != nil {
		response.Error(c, http.StatusBadRequest, "codex_cli_only_whitelist "+err.Error())
		return false
	}
	if err := service.ValidateEngineFingerprintSignalsJSON(req.CodexCLIOnlyEngineFingerprintSignals); err != nil {
		response.Error(c, http.StatusBadRequest, "codex_cli_only_engine_fingerprint_signals "+err.Error())
		return false
	}

	// 交叉验证：如果同时设置了最低和最高版本号，最高版本号必须 >= 最低版本号
	if req.MinClaudeCodeVersion != "" && req.MaxClaudeCodeVersion != "" {
		if service.CompareVersions(req.MaxClaudeCodeVersion, req.MinClaudeCodeVersion) < 0 {
			response.Error(c, http.StatusBadRequest, "max_claude_code_version must be greater than or equal to min_claude_code_version")
			return false
		}
	}

	// cyber 会话屏蔽 TTL 校验：提供时必须 > 0
	if req.CyberSessionBlockTTLSeconds != nil && *req.CyberSessionBlockTTLSeconds <= 0 {
		response.BadRequest(c, "cyber_session_block_ttl_seconds must be > 0")
		return false
	}

	prepared.defaultSubscriptions = defaultSubscriptions
	return true
}

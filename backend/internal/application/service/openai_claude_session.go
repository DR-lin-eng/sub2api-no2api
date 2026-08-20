package service

import (
	"strings"

	"github.com/tidwall/gjson"
)

const claudeCodeSessionHeader = "X-Claude-Code-Session-Id"

// deriveClaudeCodeMetadataSessionSeed returns a stable, tenant-specific seed
// from Claude Code's metadata identity. It deliberately includes the
// device/account components when available so two clients reusing the same
// session UUID do not collapse onto one scheduler key.
func deriveClaudeCodeMetadataSessionSeed(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	userID := strings.TrimSpace(gjson.GetBytes(body, "metadata.user_id").String())
	if userID != "" {
		if parsed := ParseMetadataUserID(userID); parsed != nil && strings.TrimSpace(parsed.SessionID) != "" {
			return strings.Join([]string{
				"claude-code-metadata",
				strings.TrimSpace(parsed.DeviceID),
				strings.TrimSpace(parsed.AccountUUID),
				strings.TrimSpace(parsed.SessionID),
			}, "|")
		}
	}
	if sessionID := strings.TrimSpace(gjson.GetBytes(body, "metadata.session_id").String()); sessionID != "" {
		return "claude-code-metadata|" + sessionID
	}
	return ""
}

// deriveClaudeCodeOpenAIPromptCacheKey is used only when a Claude Code
// session was supplied in metadata but the client did not emit a Responses
// prompt_cache_key. Keep this namespace distinct from the Anthropic Messages
// bridge marker so native Responses requests do not accidentally enter the
// compatibility bridge path.
func deriveClaudeCodeOpenAIPromptCacheKey(body []byte) string {
	seed := deriveClaudeCodeMetadataSessionSeed(body)
	if seed == "" {
		return ""
	}
	return "claude-code-session-" + hashSensitiveValueForLog(seed)
}

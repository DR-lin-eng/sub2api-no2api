package service

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const (
	openCodeSessionHeader         = "X-OpenCode-Session"
	openCodeInboundBodyContextKey = "opencode_inbound_body"
)

func rememberOpenCodeAccountSession(c *gin.Context, account *Account, body []byte) {
	if account != nil && account.Type == AccountTypeAPIKey && shouldSendOpenCodeSessionHeader(account, account.GetOpenAIBaseURL()) {
		rememberOpenCodeInboundBody(c, body)
	} else {
		clearOpenCodeInboundBody(c)
	}
}

// rememberOpenCodeInboundBody keeps just the session identifier across protocol
// conversion, including when it removes prompt_cache_key or metadata.user_id.
func rememberOpenCodeInboundBody(c *gin.Context, body []byte) {
	if c == nil || len(body) == 0 {
		return
	}
	session := sanitizeSessionID(openCodeSessionIDFromPayload(body))
	c.Set(openCodeInboundBodyContextKey, strings.Clone(session))
}

func clearOpenCodeInboundBody(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(openCodeInboundBodyContextKey, nil)
}

// applyOpenCodeSessionHeader sets x-opencode-session on outbound inference
// requests. OpenCode Go requires a per-conversation value from 2026-09-05
// (MissingSessionID). Prefer caller headers, then the documented body session
// fields (OpenAI prompt_cache_key / Anthropic metadata.user_id), then any
// already applied account override. A generated UUID is last-resort only for
// probes and clients that omit every stable identifier — a new UUID each turn
// would miss upstream prompt cache.
func applyOpenCodeSessionHeader(c *gin.Context, account *Account, targetURL string, headers http.Header, bodies ...[]byte) {
	if account == nil || account.Type != AccountTypeAPIKey || headers == nil {
		return
	}
	if !shouldSendOpenCodeSessionHeader(account, targetURL) {
		return
	}

	sessionID := resolveOpenCodeSessionID(c, headers, shouldGenerateOpenCodeSession(account, targetURL), bodies...)
	if sessionID == "" {
		return
	}
	for key := range headers {
		if strings.EqualFold(key, openCodeSessionHeader) {
			delete(headers, key)
		}
	}
	headers.Set(openCodeSessionHeader, sessionID)
}

func shouldSendOpenCodeSessionHeader(account *Account, targetURL string) bool {
	if account != nil && account.IsOpenCodeGoPlan() {
		return true
	}
	return isOfficialOpenCodeHost(targetURL)
}

func shouldGenerateOpenCodeSession(account *Account, targetURL string) bool {
	if account != nil && account.IsOpenCodeGoPlan() {
		return true
	}
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "https") &&
		strings.EqualFold(parsed.Hostname(), "opencode.ai") &&
		strings.Contains(parsed.Path, "/zen/go")
}

func isOfficialOpenCodeHost(targetURL string) bool {
	parsed, err := url.Parse(targetURL)
	if err != nil {
		return false
	}
	return strings.EqualFold(parsed.Scheme, "https") && strings.EqualFold(parsed.Hostname(), "opencode.ai")
}

func resolveOpenCodeSessionID(c *gin.Context, headers http.Header, generate bool, bodies ...[]byte) string {
	if c != nil && c.Request != nil {
		if sessionID := sanitizeSessionID(c.GetHeader(openCodeSessionHeader)); sessionID != "" {
			return sessionID
		}
		if sessionID := sanitizeSessionID(explicitOpenAIHeaderSessionID(c)); sessionID != "" {
			return sessionID
		}
	}
	if c != nil {
		if sessionID := c.GetString(openCodeInboundBodyContextKey); sessionID != "" {
			return sessionID
		}
	}
	for _, body := range bodies {
		if sessionID := sanitizeSessionID(openCodeSessionIDFromPayload(body)); sessionID != "" {
			return sessionID
		}
	}
	if sessionID := sanitizeSessionID(existingOpenCodeSessionHeader(headers)); sessionID != "" {
		return sessionID
	}
	if generate {
		return uuid.NewString()
	}
	return ""
}

// openCodeSessionIDFromPayload reads the stable conversation id from documented
// request-body fields. Kimi Code / OpenAI clients send prompt_cache_key on
// Chat Completions and Responses; Anthropic clients send metadata.user_id on
// Messages. Neither field changes model behavior.
func openCodeSessionIDFromPayload(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	view := openAIRequestPayloadView(body)
	if sessionID := strings.TrimSpace(view.Get("prompt_cache_key").String()); sessionID != "" {
		return sessionID
	}
	return openCodeSessionIDFromMetadataUserID(view.Get("metadata.user_id").String())
}

func openCodeSessionIDFromMetadataUserID(userID string) string {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return ""
	}
	if strings.HasPrefix(userID, "{") {
		if sessionID := strings.TrimSpace(gjson.Get(userID, "session_id").String()); sessionID != "" {
			return sessionID
		}
	}
	return userID
}

func openCodeSessionHintBody(promptCacheKey string) []byte {
	key := strings.TrimSpace(promptCacheKey)
	if key == "" {
		return nil
	}
	encoded, _ := json.Marshal(key)
	hint := append([]byte(`{"prompt_cache_key":`), encoded...)
	return append(hint, '}')
}

func existingOpenCodeSessionHeader(headers http.Header) string {
	if headers == nil {
		return ""
	}
	for key, values := range headers {
		if strings.EqualFold(key, openCodeSessionHeader) && len(values) > 0 {
			return values[0]
		}
	}
	return ""
}

package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	codexOutboundSessionSeedContextKey = "openai_codex_outbound_session_seed"
	codexOutboundSessionBodyContextKey = "openai_codex_outbound_session_body"
)

// codexOutboundSessionIDs is the isolated projection of one downstream
// conversation. Raw header values are never copied into the upstream request.
type codexOutboundSessionIDs struct {
	installationID  string
	sessionID       string
	threadID        string
	clientRequestID string
	parentThreadID  string
	subagent        string
}

// resolveCodexOutboundSessionIDs follows the same precedence as the Codex
// client: explicit protocol headers first, then client_metadata, then the
// prompt-cache signal. Values are namespaced by the account's stable virtual
// client key so spoofed device/session projections remain stable across API-key
// entry points while still diverging across OAuth accounts.
func resolveCodexOutboundSessionIDs(
	c *gin.Context,
	account *Account,
	body []byte,
	promptCacheKey string,
) *codexOutboundSessionIDs {
	if account == nil || !account.IsOpenAIOAuth() {
		return nil
	}

	sessionRaw := codexInboundHeaderValue(c, "session-id", "session_id", claudeCodeSessionHeader, "x-session-id", "conversation_id")
	threadRaw := codexInboundHeaderValue(c, "thread-id", "thread_id")
	clientRequestRaw := codexInboundHeaderValue(c, "x-client-request-id")
	parentThreadRaw := codexBodyMetadataValue(body, "x-codex-parent-thread-id")

	if sessionRaw == "" {
		sessionRaw = codexBodyMetadataValue(body, "session_id", "session-id")
	}
	if threadRaw == "" {
		threadRaw = codexBodyMetadataValue(body, "thread_id", "thread-id")
	}
	if clientRequestRaw == "" {
		clientRequestRaw = codexBodyMetadataValue(body, "turn_id", "x-client-request-id")
	}
	turnMetadata := codexBodyMetadataValue(body, "x-codex-turn-metadata")
	if parentThreadRaw == "" && gjson.Valid(turnMetadata) {
		parentThreadRaw = strings.TrimSpace(gjson.Get(turnMetadata, "parent_thread_id").String())
	}
	if parentThreadRaw == "" {
		parentThreadRaw = codexInboundHeaderValue(c, "x-codex-parent-thread-id")
	}
	subagent := strings.TrimSpace(gjson.GetBytes(body, "client_metadata.x-openai-subagent").String())
	if subagent == "" && gjson.Valid(turnMetadata) {
		subagent = strings.TrimSpace(gjson.Get(turnMetadata, "subagent_kind").String())
	}
	if !validCodexSubagentValue(subagent) {
		subagent = ""
	}

	cacheRaw := strings.TrimSpace(promptCacheKey)
	if cacheRaw == "" {
		cacheRaw = strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
	}
	if sessionRaw == "" {
		sessionRaw = cacheRaw
	}
	if threadRaw == "" {
		// x-client-request-id is a useful legacy thread signal, but the
		// upstream contract always emits x-client-request-id from the final
		// thread ID, so it is never forwarded verbatim.
		threadRaw = clientRequestRaw
	}
	if threadRaw == "" {
		threadRaw = sessionRaw
	}
	if sessionRaw == "" {
		sessionRaw = threadRaw
	}
	if sessionRaw == "" && threadRaw == "" {
		// Keep the spoofed device/session projection stable per OAuth account
		// instead of generating a new identity on every request with no client
		// session header. Explicit client sessions still receive account-scoped
		// derived IDs above.
		stableSession := resolveConvergedSessionID(account)
		if stableSession == "" {
			stableSession = codexOutboundRequestSeed(c)
		}
		sessionRaw = stableSession
		threadRaw = stableSession
	}

	namespace := codexOutboundSessionNamespace(c, account)
	threadID := deriveCodexOutboundSessionUUID("thread", namespace, threadRaw)
	sessionID := deriveCodexOutboundSessionUUID("session", namespace, sessionRaw)
	ids := &codexOutboundSessionIDs{
		installationID:  resolveConvergedInstallationID(account),
		sessionID:       sessionID,
		threadID:        threadID,
		clientRequestID: threadID,
		subagent:        subagent,
	}
	if parentThreadRaw != "" {
		ids.parentThreadID = deriveCodexOutboundSessionUUID("thread", namespace, parentThreadRaw)
	}
	return ids
}

func codexInboundHeaderValue(c *gin.Context, names ...string) string {
	if c == nil || c.Request == nil {
		return ""
	}
	for _, name := range names {
		if value := strings.TrimSpace(c.Request.Header.Get(name)); value != "" {
			return value
		}
	}
	return ""
}

func codexBodyMetadataValue(body []byte, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(gjson.GetBytes(body, "client_metadata."+name).String()); value != "" {
			return value
		}
	}
	return ""
}

func codexOutboundRequestSeed(c *gin.Context) string {
	if c != nil {
		if value, exists := c.Get(codexOutboundSessionSeedContextKey); exists {
			if seed, ok := value.(string); ok && strings.TrimSpace(seed) != "" {
				return strings.TrimSpace(seed)
			}
		}
		seed := uuid.NewString()
		c.Set(codexOutboundSessionSeedContextKey, seed)
		return seed
	}
	return uuid.NewString()
}

func codexOutboundSessionNamespace(c *gin.Context, account *Account) string {
	if account != nil {
		if virtualKey := strings.TrimSpace(account.CodexVirtualClientKey()); virtualKey != "" {
			return "account:" + virtualKey
		}
	}
	// Keep a deterministic fallback for synthetic tests or incomplete account
	// fixtures that do not expose a virtual-client key yet.
	return fmt.Sprintf("api_key:%d:account:%d", getAPIKeyIDFromContext(c), accountIDForCodexNamespace(account))
}

func accountIDForCodexNamespace(account *Account) int64 {
	if account == nil {
		return 0
	}
	return account.ID
}

func deriveCodexOutboundSessionUUID(kind, namespace, raw string) string {
	seed := fmt.Sprintf(
		"sub2api:codex:%s:v1:%s:raw:%d:%s",
		kind,
		namespace,
		len(raw),
		raw,
	)
	return deriveCodexStableUUID(seed)
}

// applyCodexOutboundSessionHeaders is called before fingerprint headers are
// staged.  A non-off fingerprint plan therefore remains the final authority;
// this function only removes untrusted raw aliases in that case.
func applyCodexOutboundSessionHeaders(
	c *gin.Context,
	account *Account,
	body []byte,
	promptCacheKey string,
	headers http.Header,
	fingerprintIDs *codexFingerprintIDs,
) {
	if account == nil || !account.IsOpenAIOAuth() || headers == nil {
		return
	}
	if isDistillationGroupRequest(c, account) {
		if sessionID, ok := distillationSessionIDFromContext(c, account); ok {
			apiKeyID := getAPIKeyIDFromContext(c)
			for _, name := range []string{"session-id", "session_id", "thread-id", "thread_id", "x-client-request-id", "conversation_id"} {
				headers.Del(name)
			}
			headers.Set("session-id", sessionID)
			headers.Set("thread-id", sessionID)
			headers.Set("x-client-request-id", sessionID)
			headers.Set("session_id", isolateOpenAISessionID(apiKeyID, sessionID))
			headers.Set("conversation_id", isolateOpenAISessionID(apiKeyID, sessionID))
		}
		return
	}
	if len(body) == 0 && c != nil {
		if value, exists := c.Get(codexOutboundSessionBodyContextKey); exists {
			if staged, ok := value.([]byte); ok {
				body = staged
			}
		}
	}
	ids := resolveCodexOutboundSessionIDs(c, account, body, promptCacheKey)
	applyResolvedCodexOutboundSessionHeaders(c, account, headers, fingerprintIDs, ids)
}

func stageCodexOutboundSessionBody(c *gin.Context, body []byte) {
	if c == nil {
		return
	}
	if body == nil {
		c.Set(codexOutboundSessionBodyContextKey, []byte(nil))
		return
	}
	// The payload is immutable for the header-building phase. Keep a slice
	// reference instead of copying potentially multi-megabyte Responses input.
	c.Set(codexOutboundSessionBodyContextKey, body)
}

func applyResolvedCodexOutboundSessionHeaders(
	c *gin.Context,
	account *Account,
	headers http.Header,
	fingerprintIDs *codexFingerprintIDs,
	ids *codexOutboundSessionIDs,
) {
	if account == nil || !account.IsOpenAIOAuth() || headers == nil {
		return
	}
	// Keep the legacy underscore projection used by the gateway's sticky-session
	// and billing code.  It is not the official Codex header, but removing it
	// would break existing compatibility callers and tests.
	legacySessionID := strings.TrimSpace(headers.Get("session_id"))
	legacyConversationID := strings.TrimSpace(headers.Get("conversation_id"))
	for _, name := range []string{
		"session-id",
		"session_id",
		"thread-id",
		"thread_id",
		"x-client-request-id",
		"x-codex-parent-thread-id",
		"x-openai-subagent",
	} {
		headers.Del(name)
	}
	if fingerprintIDs != nil && fingerprintIDs.mode != codexFingerprintOff {
		return
	}
	if ids == nil {
		return
	}
	headers.Set("session-id", ids.sessionID)
	headers.Set("thread-id", ids.threadID)
	headers.Set("x-client-request-id", ids.clientRequestID)
	if ids.parentThreadID != "" {
		headers.Set("x-codex-parent-thread-id", ids.parentThreadID)
	}
	if ids.subagent != "" {
		headers.Set("x-openai-subagent", ids.subagent)
	}
	if legacySessionID != "" {
		headers.Set("session_id", legacySessionID)
	} else {
		headers.Set("session_id", isolateOpenAISessionID(getAPIKeyIDFromContext(c), ids.sessionID))
	}
	if legacyConversationID != "" {
		headers.Set("conversation_id", legacyConversationID)
	}
}

// rewriteCodexOutboundSessionMetadata applies the OAuth client_metadata boundary
// and keeps body/header projections on the same isolated IDs. Opaque turn state
// remains untouched; identity, workspace, and approval fields are normalized.
func rewriteCodexOutboundSessionMetadata(body []byte, account *Account, ids *codexOutboundSessionIDs) ([]byte, error) {
	if len(body) == 0 || account == nil || !account.IsOpenAIOAuth() {
		return body, nil
	}
	metadata := gjson.GetBytes(body, "client_metadata")
	if !metadata.Exists() || strings.TrimSpace(metadata.Raw) == "null" {
		return body, nil
	}
	if metadata.Type != gjson.JSON || !metadata.IsObject() {
		rewritten, err := sjson.DeleteBytes(body, "client_metadata")
		if err != nil {
			return body, fmt.Errorf("remove invalid Codex client_metadata: %w", err)
		}
		return rewritten, nil
	}

	clientMetadata := make(map[string]any)
	if err := json.Unmarshal([]byte(metadata.Raw), &clientMetadata); err != nil {
		return body, fmt.Errorf("decode Codex client_metadata: %w", err)
	}
	if !sanitizeOpenAICodexClientMetadataMap(clientMetadata, account, ids) {
		return body, nil
	}
	encoded, err := json.Marshal(clientMetadata)
	if err != nil {
		return body, fmt.Errorf("encode Codex client_metadata: %w", err)
	}
	return sjson.SetRawBytes(body, "client_metadata", encoded)
}

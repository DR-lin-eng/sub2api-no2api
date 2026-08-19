package service

import (
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
	sessionID       string
	threadID        string
	clientRequestID string
}

// resolveCodexOutboundSessionIDs follows the same precedence as the Codex
// client: explicit protocol headers first, then client_metadata, then the
// prompt-cache signal.  Values are namespaced by both the API key and the
// selected upstream account so a failover cannot replay one tenant's opaque
// IDs into another account.
func resolveCodexOutboundSessionIDs(
	c *gin.Context,
	account *Account,
	body []byte,
	promptCacheKey string,
) *codexOutboundSessionIDs {
	if account == nil || !account.IsOpenAIOAuth() {
		return nil
	}

	sessionRaw := codexInboundHeaderValue(c, "session-id", "session_id", "x-session-id", "conversation_id")
	threadRaw := codexInboundHeaderValue(c, "thread-id", "thread_id")
	clientRequestRaw := codexInboundHeaderValue(c, "x-client-request-id")

	if sessionRaw == "" {
		sessionRaw = codexBodyMetadataValue(body, "session_id", "session-id")
	}
	if threadRaw == "" {
		threadRaw = codexBodyMetadataValue(body, "thread_id", "thread-id")
	}
	if clientRequestRaw == "" {
		clientRequestRaw = codexBodyMetadataValue(body, "turn_id", "x-client-request-id")
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
		seed := codexOutboundRequestSeed(c)
		sessionRaw = seed
		threadRaw = seed
	}

	namespace := codexOutboundSessionNamespace(c, account)
	threadID := deriveCodexOutboundSessionUUID("thread", namespace, threadRaw)
	sessionID := deriveCodexOutboundSessionUUID("session", namespace, sessionRaw)
	return &codexOutboundSessionIDs{
		sessionID:       sessionID,
		threadID:        threadID,
		clientRequestID: threadID,
	}
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
	apiKeyID := getAPIKeyIDFromContext(c)
	accountID := int64(0)
	if account != nil {
		accountID = account.ID
	}
	return fmt.Sprintf("api_key:%d:account:%d", apiKeyID, accountID)
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
	if legacySessionID != "" {
		headers.Set("session_id", legacySessionID)
	} else {
		headers.Set("session_id", isolateOpenAISessionID(getAPIKeyIDFromContext(c), ids.sessionID))
	}
	if legacyConversationID != "" {
		headers.Set("conversation_id", legacyConversationID)
	}
}

// rewriteCodexOutboundSessionMetadata keeps body and header projections on the
// same isolated IDs when a client_metadata object is present. Other metadata,
// including opaque turn state, is preserved byte-for-byte by sjson.
func rewriteCodexOutboundSessionMetadata(body []byte, ids *codexOutboundSessionIDs) ([]byte, error) {
	if len(body) == 0 || ids == nil {
		return body, nil
	}
	metadata := gjson.GetBytes(body, "client_metadata")
	if !metadata.Exists() || metadata.Type != gjson.JSON || !metadata.IsObject() {
		return body, nil
	}
	rewritten, err := sjson.SetBytes(body, "client_metadata.session_id", ids.sessionID)
	if err != nil {
		return body, fmt.Errorf("rewrite Codex client_metadata session_id: %w", err)
	}
	rewritten, err = sjson.SetBytes(rewritten, "client_metadata.thread_id", ids.threadID)
	if err != nil {
		return body, fmt.Errorf("rewrite Codex client_metadata thread_id: %w", err)
	}
	return rewritten, nil
}

package service

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type codexFingerprintMode string

const (
	codexFingerprintOff     codexFingerprintMode = "off"
	codexFingerprintDevice  codexFingerprintMode = "device"
	codexFingerprintSession codexFingerprintMode = "session"
	codexFingerprintFull    codexFingerprintMode = "full"

	codexFingerprintModeExtraKey = "codex_fingerprint_mode"
	codexFingerprintWSKeyHeader  = "x-sub2api-internal-codex-fingerprint-key"
	// codexFingerprintIDsContextKey stores the per-attempt identity plan. The
	// request body and outbound headers must consume the same plan so turn IDs
	// and timestamps cannot drift between the two representations.
	codexFingerprintIDsContextKey = "codex_fingerprint_ids"
)

// stageCodexFingerprintIDs always overwrites the attempt value, including nil.
// Account failover can move from an opted-in OAuth account to an account with
// convergence disabled; leaving the previous plan in the context would leak
// the old account's identity into the next request.
func stageCodexFingerprintIDs(c *gin.Context, ids *codexFingerprintIDs) {
	if c != nil {
		c.Set(codexFingerprintIDsContextKey, ids)
	}
}

// applyStagedCodexFingerprintHeaders applies the plan staged by the current
// request attempt. The account/type guard prevents stale context values from
// affecting API-key or non-OAuth failover attempts.
func applyStagedCodexFingerprintHeaders(c *gin.Context, account *Account, headers http.Header) {
	if c == nil || account == nil || !account.IsOpenAIOAuth() || headers == nil {
		return
	}
	value, ok := c.Get(codexFingerprintIDsContextKey)
	if !ok {
		return
	}
	ids, ok := value.(*codexFingerprintIDs)
	if !ok || ids == nil {
		return
	}
	applyCodexFingerprintHeaders(headers, ids)
}

// GetCodexFingerprintMode keeps existing accounts opt-out unless an
// administrator explicitly selects a convergence mode.
func (a *Account) GetCodexFingerprintMode() codexFingerprintMode {
	if a == nil || !a.IsOpenAIOAuth() {
		return codexFingerprintOff
	}

	switch codexFingerprintMode(strings.ToLower(strings.TrimSpace(a.GetExtraString(codexFingerprintModeExtraKey)))) {
	case codexFingerprintDevice:
		return codexFingerprintDevice
	case codexFingerprintSession:
		return codexFingerprintSession
	case codexFingerprintFull:
		return codexFingerprintFull
	default:
		return codexFingerprintOff
	}
}

func deriveCodexStableUUID(seed string) string {
	digest := sha256.Sum256([]byte(seed))
	var id uuid.UUID
	copy(id[:], digest[:len(id)])
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return id.String()
}

func resolveConvergedInstallationID(account *Account) string {
	if account == nil {
		return ""
	}
	if deviceID := strings.TrimSpace(account.GetOpenAIDeviceID()); deviceID != "" {
		return deviceID
	}
	return deriveCodexStableUUID("sub2api:codex-install-id:v1:" + codexFingerprintAccountSeed(account))
}

func resolveConvergedSessionID(account *Account) string {
	if account == nil {
		return ""
	}
	return deriveCodexStableUUID("sub2api:codex-session-id:v1:" + codexFingerprintAccountSeed(account))
}

func resolveConvergedThreadID(account *Account, clientSessionID string) string {
	clientSessionID = strings.TrimSpace(clientSessionID)
	if account == nil || clientSessionID == "" {
		return ""
	}
	return deriveCodexStableUUID("sub2api:codex-thread-id:v1:" + codexFingerprintAccountSeed(account) + ":" + clientSessionID)
}

func codexFingerprintAccountSeed(account *Account) string {
	if account == nil {
		return ""
	}
	if upstreamAccountID := strings.TrimSpace(account.GetCredential("chatgpt_account_id")); upstreamAccountID != "" {
		return "chatgpt:" + upstreamAccountID
	}
	if deviceID := strings.TrimSpace(account.GetOpenAIDeviceID()); deviceID != "" {
		return "device:" + deviceID
	}
	return "local:" + accountIDString(account.ID)
}

func accountIDString(accountID int64) string {
	return strconv.FormatInt(accountID, 10)
}

type codexFingerprintIDs struct {
	mode            codexFingerprintMode
	installationID  string
	sessionID       string
	threadID        string
	turnID          string
	windowID        string
	turnStartedAtMS int64
}

func resolveCodexFingerprintIDs(account *Account, clientSessionID string, mode codexFingerprintMode) *codexFingerprintIDs {
	if account == nil || mode == codexFingerprintOff {
		return nil
	}

	ids := &codexFingerprintIDs{
		mode:           mode,
		installationID: resolveConvergedInstallationID(account),
	}
	if ids.installationID == "" || mode == codexFingerprintDevice {
		return ids
	}

	ids.sessionID = resolveConvergedSessionID(account)
	if mode == codexFingerprintFull {
		ids.threadID = ids.sessionID
	} else {
		ids.threadID = resolveConvergedThreadID(account, clientSessionID)
		if ids.threadID == "" {
			ids.threadID = ids.sessionID
		}
	}
	ids.turnID = uuid.Must(uuid.NewV7()).String()
	ids.windowID = ids.threadID + ":0"
	ids.turnStartedAtMS = time.Now().UnixMilli()
	return ids
}

func extractClientSessionID(headers http.Header) string {
	if headers == nil {
		return ""
	}
	if value := strings.TrimSpace(headers.Get("session-id")); value != "" {
		return value
	}
	return strings.TrimSpace(headers.Get("session_id"))
}

func resolveCodexFingerprintIDsFromRequest(account *Account, headers http.Header) *codexFingerprintIDs {
	if account == nil {
		return nil
	}
	mode := account.GetCodexFingerprintMode()
	if mode == codexFingerprintOff {
		return nil
	}
	return resolveCodexFingerprintIDs(account, extractClientSessionID(headers), mode)
}

func resolveCodexFingerprintIDsFromGinContext(account *Account, c *gin.Context) *codexFingerprintIDs {
	var headers http.Header
	if c != nil && c.Request != nil {
		headers = c.Request.Header
	}
	return resolveCodexFingerprintIDsFromRequest(account, headers)
}

func nextCodexFingerprintTurn(ids *codexFingerprintIDs) *codexFingerprintIDs {
	if ids == nil || ids.mode == codexFingerprintDevice {
		return ids
	}
	turn := *ids
	turn.turnID = uuid.Must(uuid.NewV7()).String()
	turn.turnStartedAtMS = time.Now().UnixMilli()
	return &turn
}

func applyCodexFingerprintHeaders(headers http.Header, ids *codexFingerprintIDs) {
	if headers == nil || ids == nil || ids.installationID == "" {
		return
	}

	headers.Set("x-codex-installation-id", ids.installationID)
	if ids.mode == codexFingerprintDevice {
		rewriteCodexTurnMetadataHeader(headers, ids)
		return
	}

	headers.Set("x-codex-window-id", ids.windowID)
	// x-client-request-id remains unique per request; using the stable thread ID
	// here would collapse upstream request tracing and idempotency signals.
	headers.Set("x-client-request-id", ids.turnID)
	headers.Set("session-id", ids.sessionID)
	headers.Set("session_id", ids.sessionID)
	headers.Set("thread-id", ids.threadID)
	rewriteCodexTurnMetadataHeader(headers, ids)
}

func (s *OpenAIGatewayService) codexIdentityOverrideUA(account *Account) string {
	if s != nil && s.cfg != nil && s.cfg.Gateway.ForceCodexCLI {
		return ""
	}
	if account == nil {
		return ""
	}
	return account.GetOpenAIUserAgent()
}

func applyCodexFingerprintWSHeaders(headers http.Header, ids *codexFingerprintIDs) {
	if headers == nil || ids == nil || ids.installationID == "" {
		return
	}
	headers.Set("x-codex-installation-id", ids.installationID)
	headers.Set(codexFingerprintWSKeyHeader, codexFingerprintWSCompatibilityKey(ids))
	rewriteCodexTurnMetadataHeader(headers, ids)
	if ids.mode == codexFingerprintDevice {
		return
	}
	headers.Set("x-codex-window-id", ids.windowID)
	headers.Set("session-id", ids.sessionID)
	headers.Set("session_id", ids.sessionID)
	headers.Set("thread-id", ids.threadID)
}

func codexFingerprintWSCompatibilityKey(ids *codexFingerprintIDs) string {
	if ids == nil {
		return ""
	}
	return strings.Join([]string{
		string(ids.mode),
		ids.installationID,
		ids.sessionID,
		ids.threadID,
	}, "\x00")
}

func rewriteCodexTurnMetadataHeader(headers http.Header, ids *codexFingerprintIDs) {
	raw := strings.TrimSpace(headers.Get("x-codex-turn-metadata"))
	if rewritten := rewriteCodexTurnMetadataValue(raw, ids); rewritten != raw {
		headers.Set("x-codex-turn-metadata", rewritten)
	}
}

func rewriteCodexTurnMetadataValue(raw string, ids *codexFingerprintIDs) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || ids == nil {
		return raw
	}
	metadata := make(map[string]any)
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return raw
	}
	applyCodexTurnMetadataFields(metadata, ids)
	rebuilt, err := json.Marshal(metadata)
	if err != nil {
		return raw
	}
	return string(rebuilt)
}

func applyCodexTurnMetadataFields(metadata map[string]any, ids *codexFingerprintIDs) {
	if metadata == nil || ids == nil {
		return
	}
	metadata["installation_id"] = ids.installationID
	if ids.mode == codexFingerprintDevice {
		return
	}
	metadata["session_id"] = ids.sessionID
	metadata["thread_id"] = ids.threadID
	metadata["turn_id"] = ids.turnID
	metadata["window_id"] = ids.windowID
	metadata["turn_started_at_unix_ms"] = ids.turnStartedAtMS
}

func applyCodexFingerprintClientMetadata(reqBody map[string]any, ids *codexFingerprintIDs) bool {
	if reqBody == nil || ids == nil || ids.installationID == "" {
		return false
	}

	metadata, ok := mutableCodexClientMetadata(reqBody["client_metadata"])
	if !ok {
		return false
	}
	if !applyCodexFingerprintClientMetadataMap(metadata, ids) {
		return false
	}
	reqBody["client_metadata"] = metadata
	return true
}

// applyCodexFingerprintClientMetadataMap is the shared mutation core used by
// decoded and raw-body paths. Keeping the field policy in one place prevents
// passthrough and normal forwarding from diverging.
func applyCodexFingerprintClientMetadataMap(metadata map[string]any, ids *codexFingerprintIDs) bool {
	if metadata == nil || ids == nil || ids.installationID == "" {
		return false
	}
	metadata["x-codex-installation-id"] = ids.installationID
	if ids.mode != codexFingerprintDevice {
		metadata["session_id"] = ids.sessionID
		metadata["thread_id"] = ids.threadID
		metadata["turn_id"] = ids.turnID
		metadata["x-codex-window-id"] = ids.windowID
	}
	rewriteEmbeddedCodexTurnMetadata(metadata, ids)
	return true
}

// applyCodexFingerprintClientMetadataToBody rewrites only client_metadata and
// copies the outer request once. Large Responses input/tool payloads stay raw.
func applyCodexFingerprintClientMetadataToBody(body []byte, ids *codexFingerprintIDs) ([]byte, bool, error) {
	if len(body) == 0 || ids == nil || ids.installationID == "" {
		return body, false, nil
	}

	metadataResult := gjson.GetBytes(body, "client_metadata")
	metadata := make(map[string]any)
	if metadataResult.Exists() && strings.TrimSpace(metadataResult.Raw) != "null" {
		if metadataResult.Type != gjson.JSON || !metadataResult.IsObject() {
			return body, false, nil
		}
		if err := json.Unmarshal([]byte(metadataResult.Raw), &metadata); err != nil {
			return body, false, fmt.Errorf("decode codex client_metadata: %w", err)
		}
	}

	if !applyCodexFingerprintClientMetadataMap(metadata, ids) {
		return body, false, nil
	}
	rebuiltMetadata, err := json.Marshal(metadata)
	if err != nil {
		return body, false, fmt.Errorf("encode codex client_metadata: %w", err)
	}
	rewritten, err := sjson.SetRawBytes(body, "client_metadata", rebuiltMetadata)
	if err != nil {
		return body, false, fmt.Errorf("rewrite codex client_metadata: %w", err)
	}
	return rewritten, true, nil
}

func mutableCodexClientMetadata(value any) (map[string]any, bool) {
	switch existing := value.(type) {
	case nil:
		return make(map[string]any), true
	case map[string]any:
		return existing, true
	case map[string]string:
		converted := make(map[string]any, len(existing)+5)
		for key, item := range existing {
			converted[key] = item
		}
		return converted, true
	default:
		return nil, false
	}
}

func rewriteEmbeddedCodexTurnMetadata(clientMetadata map[string]any, ids *codexFingerprintIDs) {
	raw, ok := clientMetadata["x-codex-turn-metadata"].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return
	}

	metadata := make(map[string]any)
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return
	}
	applyCodexTurnMetadataFields(metadata, ids)
	rebuilt, err := json.Marshal(metadata)
	if err == nil {
		clientMetadata["x-codex-turn-metadata"] = string(rebuilt)
	}
}

package service

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/codexsimulation"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type codexFingerprintMode string

const (
	codexRequestKindTurn       = "turn"
	codexRequestKindPrewarm    = "prewarm"
	codexRequestKindCompaction = "compaction"
)

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

var codexReservedIdentityHeaders = []string{
	"conversation_id",
	"originator",
	"session-id",
	"session_id",
	"thread-id",
	"user-agent",
	"version",
	"x-client-request-id",
	"x-codex-installation-id",
	"x-codex-parent-thread-id",
	"x-codex-turn-metadata",
	"x-codex-turn-state",
	"x-codex-window-id",
	"x-codex-host-device-kind",
	"x-openai-internal-codex-residency",
	"x-oai-attestation",
	"x-openai-subagent",
	CodexProjectIDHeader,
}

// codexOfficialClientMetadataKeys is the flat projection emitted by the
// Rust client. Full simulation keeps these keys at the top level and moves
// caller-defined metadata into bounded flattened turn-metadata extra keys,
// matching the source client's separation between compatibility projections
// and turn metadata.
var codexOfficialClientMetadataKeys = map[string]struct{}{
	"x-codex-installation-id":  {},
	"session_id":               {},
	"thread_id":                {},
	"turn_id":                  {},
	"x-codex-window-id":        {},
	"x-codex-turn-metadata":    {},
	"x-codex-turn-state":       {},
	"x-openai-subagent":        {},
	"x-codex-parent-thread-id": {},
	"parent_turn_id":           {},
	"root_turn_id":             {},
	"ws_request_header_x_openai_internal_codex_responses_lite": {},
	"ws_request_header_traceparent":                            {},
	"ws_request_header_tracestate":                             {},
}

var codexOfficialTurnMetadataKeys = map[string]struct{}{
	"installation_id":                {},
	"session_id":                     {},
	"thread_id":                      {},
	"agent_name":                     {},
	"turn_id":                        {},
	"window_id":                      {},
	"request_kind":                   {},
	"compaction":                     {},
	"forked_from_thread_id":          {},
	"parent_thread_id":               {},
	"parent_turn_id":                 {},
	"root_turn_id":                   {},
	"subagent_kind":                  {},
	"thread_source":                  {},
	"sandbox":                        {},
	"sandbox_mode":                   {},
	"auto_review_enabled":            {},
	"node_repl_auto_review_required": {},
	"node_repl_disabled":             {},
	"workspaces":                     {},
	"tool_namespaces_info":           {},
	"turn_started_at_unix_ms":        {},
}

var codexForbiddenTurnMetadataKeys = map[string]struct{}{
	"x-codex-installation-id":  {},
	"x-codex-window-id":        {},
	"x-codex-turn-metadata":    {},
	"x-codex-parent-thread-id": {},
	"x-openai-subagent":        {},
	"code_mode_tool_names":     {},
}

const (
	codexExtraMetadataMaxEntries    = 16
	codexExtraMetadataMaxKeyBytes   = 64
	codexExtraMetadataMaxValueBytes = 128
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
	return account.CodexVirtualClientKey()
}

type codexFingerprintIDs struct {
	mode                     codexFingerprintMode
	fullSimulation           bool
	rootKey                  string
	principalKey             string
	installationID           string
	sessionID                string
	threadID                 string
	turnID                   string
	windowID                 string
	promptCacheKey           string
	generation               uint64
	turnStartedAtMS          int64
	profile                  codexSimulationProfile
	requestKind              string
	identitySecret           string
	directInstallationHeader bool
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
	if value := strings.TrimSpace(headers.Get(claudeCodeSessionHeader)); value != "" {
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
	if attempt, ok := codexSimulationAttemptFromGin(c); ok && attempt.fingerprint != nil {
		if account != nil && attempt.principal.key != "" {
			return attempt.fingerprint
		}
	}
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
	turnMetadata := headers.Get("x-codex-turn-metadata")
	if ids.fullSimulation {
		stripCodexReservedIdentityHeaders(headers)
		if strings.TrimSpace(turnMetadata) != "" {
			headers.Set("x-codex-turn-metadata", turnMetadata)
		}
	}

	if !ids.fullSimulation || ids.directInstallationHeader {
		headers.Set("x-codex-installation-id", ids.installationID)
	} else {
		headers.Del("x-codex-installation-id")
	}
	if ids.mode == codexFingerprintDevice {
		rewriteCodexTurnMetadataHeader(headers, ids)
		if ids.fullSimulation {
			restoreCodexTurnCompatibilityHeaders(headers)
		}
		return
	}

	headers.Set("x-codex-window-id", ids.windowID)
	if ids.fullSimulation {
		// Pinned Codex source uses the thread ID for x-client-request-id.
		headers.Set("x-client-request-id", ids.threadID)
	} else {
		headers.Set("x-client-request-id", ids.turnID)
	}
	headers.Set("session-id", ids.sessionID)
	headers.Set("thread-id", ids.threadID)
	if !ids.fullSimulation {
		headers.Set("session_id", ids.sessionID)
	}
	rewriteCodexTurnMetadataHeader(headers, ids)
	if ids.fullSimulation {
		restoreCodexTurnCompatibilityHeaders(headers)
	}
	applyCodexSimulationProfileHeaders(headers, ids)
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
	turnMetadata := headers.Get("x-codex-turn-metadata")
	if ids.fullSimulation {
		stripCodexReservedIdentityHeaders(headers)
		if strings.TrimSpace(turnMetadata) != "" {
			headers.Set("x-codex-turn-metadata", turnMetadata)
		}
	}
	if !ids.fullSimulation || ids.directInstallationHeader {
		headers.Set("x-codex-installation-id", ids.installationID)
	} else {
		headers.Del("x-codex-installation-id")
	}
	headers.Set(codexFingerprintWSKeyHeader, codexFingerprintWSCompatibilityKey(ids))
	rewriteCodexTurnMetadataHeader(headers, ids)
	if ids.fullSimulation {
		restoreCodexTurnCompatibilityHeaders(headers)
	}
	if ids.mode == codexFingerprintDevice {
		return
	}
	headers.Set("x-codex-window-id", ids.windowID)
	headers.Set("session-id", ids.sessionID)
	headers.Set("thread-id", ids.threadID)
	if ids.fullSimulation {
		headers.Set("x-client-request-id", ids.threadID)
	} else {
		headers.Set("session_id", ids.sessionID)
	}
	applyCodexSimulationProfileHeaders(headers, ids)
}

func restoreCodexTurnCompatibilityHeaders(headers http.Header) {
	if headers == nil {
		return
	}
	var metadata map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(headers.Get("x-codex-turn-metadata"))), &metadata); err != nil {
		return
	}
	if parent, ok := metadata["parent_thread_id"].(string); ok && strings.TrimSpace(parent) != "" {
		headers.Set("x-codex-parent-thread-id", parent)
	}
	if subagent, ok := metadata["subagent_kind"].(string); ok && validCodexSubagentValue(subagent) {
		headers.Set("x-openai-subagent", subagent)
	}
}

// applyCodexFullSimulationWSHeaders is used only by the new reconnect-header
// refresh path. Legacy device/session convergence must retain its pre-A/B
// behavior when the simulation switches are off.
func applyCodexFullSimulationWSHeaders(headers http.Header, ids *codexFingerprintIDs) {
	if ids == nil || !ids.fullSimulation {
		return
	}
	applyCodexFingerprintWSHeaders(headers, ids)
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
	if ids == nil || (raw == "" && !ids.fullSimulation) {
		return raw
	}
	metadata := make(map[string]any)
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil && !ids.fullSimulation {
			return raw
		}
	}
	if ids.fullSimulation {
		projectCodexFullSimulationTurnMetadata(metadata)
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
	if ids.requestKind != "" {
		metadata["request_kind"] = ids.requestKind
	}
	if ids.mode == codexFingerprintDevice {
		return
	}
	metadata["session_id"] = ids.sessionID
	metadata["thread_id"] = ids.threadID
	metadata["turn_id"] = ids.turnID
	metadata["window_id"] = ids.windowID
	metadata["turn_started_at_unix_ms"] = ids.turnStartedAtMS
	if ids.fullSimulation {
		for _, key := range []string{"forked_from_thread_id", "parent_thread_id", "parent_turn_id", "root_turn_id"} {
			if raw, ok := metadata[key].(string); ok && strings.TrimSpace(raw) != "" {
				metadata[key] = rewriteCodexSimulationMetadataID(ids, key, raw)
			}
		}
		if subagent, ok := metadata["subagent_kind"].(string); ok && !validCodexSubagentValue(subagent) {
			delete(metadata, "subagent_kind")
		}
	}
}

func rewriteCodexSimulationMetadataID(ids *codexFingerprintIDs, domain, raw string) string {
	if ids == nil || !ids.fullSimulation || strings.TrimSpace(ids.identitySecret) == "" {
		return raw
	}
	return codexSimulationUUID(ids.identitySecret, "metadata:"+domain, ids.principalKey, strings.TrimSpace(raw))
}

func validCodexSubagentValue(value string) bool {
	switch strings.TrimSpace(value) {
	case "review", "compact", "memory_consolidation", "collab_spawn":
		return true
	default:
		return strings.TrimSpace(value) != "" && validCodexExtraMetadataKey(value)
	}
}

func applyCodexFingerprintClientMetadata(reqBody map[string]any, ids *codexFingerprintIDs) bool {
	if reqBody == nil || ids == nil || ids.installationID == "" {
		return false
	}

	metadata, ok := mutableCodexClientMetadata(reqBody["client_metadata"])
	if !ok && ids.fullSimulation {
		metadata, ok = make(map[string]any), true
	}
	if !ok {
		return false
	}
	if !applyCodexFingerprintClientMetadataMap(metadata, ids) {
		return false
	}
	reqBody["client_metadata"] = metadata
	if ids.fullSimulation {
		reqBody["prompt_cache_key"] = ids.promptCacheKey
	}
	return true
}

// applyCodexFingerprintClientMetadataMap is the shared mutation core used by
// decoded and raw-body paths. Keeping the field policy in one place prevents
// passthrough and normal forwarding from diverging.
func applyCodexFingerprintClientMetadataMap(metadata map[string]any, ids *codexFingerprintIDs) bool {
	if metadata == nil || ids == nil || ids.installationID == "" {
		return false
	}
	if ids.fullSimulation {
		projectCodexFullSimulationMetadata(metadata)
	}
	metadata["x-codex-installation-id"] = ids.installationID
	if ids.fullSimulation {
		if parent, ok := metadata["x-codex-parent-thread-id"].(string); ok && strings.TrimSpace(parent) != "" {
			metadata["x-codex-parent-thread-id"] = rewriteCodexSimulationMetadataID(ids, "parent_thread_id", parent)
		}
		if subagent, ok := metadata["x-openai-subagent"].(string); ok && !validCodexSubagentValue(subagent) {
			delete(metadata, "x-openai-subagent")
		}
	}
	if ids.mode != codexFingerprintDevice {
		metadata["session_id"] = ids.sessionID
		metadata["thread_id"] = ids.threadID
		metadata["turn_id"] = ids.turnID
		metadata["x-codex-window-id"] = ids.windowID
	}
	rewriteEmbeddedCodexTurnMetadata(metadata, ids)
	return true
}

// projectCodexFullSimulationMetadata keeps the official flat metadata surface
// stable while preserving non-identity caller metadata as source-compatible
// flattened turn-metadata extra keys. Values are stringified and bounded so an
// arbitrary downstream map cannot create a new unbounded wire fingerprint.
func projectCodexFullSimulationMetadata(metadata map[string]any) {
	if len(metadata) == 0 {
		return
	}
	extra := make(map[string]string)
	for _, key := range sortedCodexMetadataKeys(metadata) {
		value := metadata[key]
		if _, official := codexOfficialClientMetadataKeys[key]; official {
			continue
		}
		if len(extra) >= codexExtraMetadataMaxEntries || !validCodexExtraMetadataKey(key) {
			delete(metadata, key)
			continue
		}
		encoded, err := codexMetadataExtraString(value)
		if err != nil || len(encoded) > codexExtraMetadataMaxValueBytes {
			delete(metadata, key)
			continue
		}
		extra[key] = encoded
		delete(metadata, key)
	}
	if len(extra) == 0 {
		normalizeCodexOfficialClientMetadataTypes(metadata)
		return
	}

	turnMetadata := make(map[string]any)
	if raw, ok := metadata["x-codex-turn-metadata"].(string); ok && strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &turnMetadata)
	}
	for key, value := range extra {
		turnMetadata[key] = value
	}
	projectCodexFullSimulationTurnMetadata(turnMetadata)
	if encoded, err := json.Marshal(turnMetadata); err == nil {
		metadata["x-codex-turn-metadata"] = string(encoded)
	}
	normalizeCodexOfficialClientMetadataTypes(metadata)
}

func normalizeCodexOfficialClientMetadataTypes(metadata map[string]any) {
	for key := range codexOfficialClientMetadataKeys {
		value, exists := metadata[key]
		if !exists {
			continue
		}
		if _, ok := value.(string); ok {
			continue
		}
		encoded, err := codexMetadataExtraString(value)
		if err != nil || len(encoded) > codexExtraMetadataMaxValueBytes {
			delete(metadata, key)
			continue
		}
		metadata[key] = encoded
	}
}

func projectCodexFullSimulationTurnMetadata(metadata map[string]any) {
	if metadata == nil {
		return
	}
	count := 0
	for _, key := range sortedCodexMetadataKeys(metadata) {
		value := metadata[key]
		if _, official := codexOfficialTurnMetadataKeys[key]; official {
			continue
		}
		if _, forbidden := codexForbiddenTurnMetadataKeys[key]; forbidden {
			delete(metadata, key)
			continue
		}
		if count >= codexExtraMetadataMaxEntries || !validCodexExtraMetadataKey(key) {
			delete(metadata, key)
			continue
		}
		encoded, err := codexMetadataExtraString(value)
		if err != nil || len(encoded) > codexExtraMetadataMaxValueBytes {
			delete(metadata, key)
			continue
		}
		metadata[key] = encoded
		count++
	}
}

func sortedCodexMetadataKeys(metadata map[string]any) []string {
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func validCodexExtraMetadataKey(key string) bool {
	if len(key) == 0 || len(key) > codexExtraMetadataMaxKeyBytes {
		return false
	}
	for index, char := range []byte(key) {
		if index == 0 {
			if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') {
				return false
			}
			continue
		}
		if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') &&
			(char < '0' || char > '9') && char != '_' && char != '.' && char != '-' {
			return false
		}
	}
	return true
}

func codexMetadataExtraString(value any) (string, error) {
	if text, ok := value.(string); ok {
		return text, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
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
			if !ids.fullSimulation {
				return body, false, nil
			}
			metadata = make(map[string]any)
		} else if err := json.Unmarshal([]byte(metadataResult.Raw), &metadata); err != nil {
			if !ids.fullSimulation {
				return body, false, fmt.Errorf("decode codex client_metadata: %w", err)
			}
			metadata = make(map[string]any)
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
	if ids.fullSimulation {
		rewritten, err = sjson.SetBytes(rewritten, "prompt_cache_key", ids.promptCacheKey)
		if err != nil {
			return body, false, fmt.Errorf("rewrite codex prompt_cache_key: %w", err)
		}
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
	if (!ok || strings.TrimSpace(raw) == "") && !ids.fullSimulation {
		return
	}

	metadata := make(map[string]any)
	if strings.TrimSpace(raw) != "" {
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil && !ids.fullSimulation {
			return
		}
	}
	if ids.fullSimulation {
		projectCodexFullSimulationTurnMetadata(metadata)
	}
	applyCodexTurnMetadataFields(metadata, ids)
	rebuilt, err := json.Marshal(metadata)
	if err == nil {
		clientMetadata["x-codex-turn-metadata"] = string(rebuilt)
	}
}

func stripCodexReservedIdentityHeaders(headers http.Header) {
	if headers == nil {
		return
	}
	for _, header := range codexReservedIdentityHeaders {
		headers.Del(header)
	}
	codexsimulation.StripUntrustedPlatformSignals(headers)
}

func applyCodexSimulationProfileHeaders(headers http.Header, ids *codexFingerprintIDs) {
	if headers == nil || ids == nil || !ids.fullSimulation {
		return
	}
	headers.Set("user-agent", ids.profile.userAgent)
	headers.Set("originator", ids.profile.originator)
	headers.Set("version", ids.profile.version)
}

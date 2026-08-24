package service

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type codexContinuationMode string

const (
	codexContinuationOff     codexContinuationMode = "off"
	codexContinuationShadow  codexContinuationMode = "shadow"
	codexContinuationEnforce codexContinuationMode = "enforce"
)

type codexContinuationBodyKind string

const (
	codexContinuationFull        codexContinuationBodyKind = "full"
	codexContinuationIncremental codexContinuationBodyKind = "incremental"
)

type codexContinuationOwnerKind string

const (
	codexContinuationOwnerUnknown  codexContinuationOwnerKind = "unknown"
	codexContinuationOwnerOwned    codexContinuationOwnerKind = "owned"
	codexContinuationOwnerExternal codexContinuationOwnerKind = "external"
)

type codexContinuationClassification struct {
	kind               codexContinuationBodyKind
	previousResponseID string
	reasons            []string
	parseErr           error
}

type codexContinuationOwnership struct {
	kind      codexContinuationOwnerKind
	principal string
}

type codexContinuationAttempt struct {
	mode                   codexContinuationMode
	classification         codexContinuationClassification
	ownership              codexContinuationOwnership
	requireExactConnection bool
	sanitized              bool
}

// CodexContinuationTerminalError is returned directly to the handler. It is
// deliberately not an UpstreamFailoverError because changing accounts cannot
// repair an incremental principal or connection mismatch.
type CodexContinuationTerminalError struct {
	HTTPStatus int
	ErrorType  string
	Message    string
	Cause      error
}

func (e *CodexContinuationTerminalError) Error() string {
	if e == nil {
		return "codex continuation rejected"
	}
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

func (e *CodexContinuationTerminalError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func newCodexContinuationTerminalError(message string, cause error) *CodexContinuationTerminalError {
	return &CodexContinuationTerminalError{
		HTTPStatus: http.StatusConflict,
		ErrorType:  "invalid_request_error",
		Message:    message,
		Cause:      cause,
	}
}

func (s *OpenAIGatewayService) codexContinuationModeForRequest(ctx context.Context, c *gin.Context) codexContinuationMode {
	settings := s.codexSimulationSettingsSnapshot(ctx, c)
	if !settings.configured() {
		return codexContinuationOff
	}
	return settings.continuationMode()
}

func (s *OpenAIGatewayService) prepareCodexContinuationAttempt(
	ctx context.Context,
	c *gin.Context,
	request *codexSimulationRequestState,
	principal codexSimulationPrincipal,
	body []byte,
) (*codexContinuationAttempt, []byte, error) {
	mode := request.settings.continuationMode()
	if mode == codexContinuationOff {
		return &codexContinuationAttempt{mode: mode}, body, nil
	}
	classification := classifyCodexContinuationBody(body)
	ownership := s.lookupCodexContinuationOwnership(ctx, request, classification.previousResponseID)
	attempt := &codexContinuationAttempt{
		mode:           mode,
		classification: classification,
		ownership:      ownership,
	}

	crossPrincipal := ownership.kind == codexContinuationOwnerExternal ||
		(ownership.kind == codexContinuationOwnerOwned && ownership.principal != principal.key)
	hypothetical := "allow"
	if crossPrincipal && classification.kind == codexContinuationIncremental {
		hypothetical = "reject_incremental_migration"
	} else if crossPrincipal {
		hypothetical = "sanitize_full_migration"
	} else if ownership.kind == codexContinuationOwnerOwned && ownership.principal == principal.key && classification.kind == codexContinuationIncremental {
		hypothetical = "require_exact_connection"
	}
	if mode == codexContinuationShadow {
		slog.DebugContext(ctx, "codex continuation shadow decision",
			"body_kind", classification.kind,
			"owner_kind", ownership.kind,
			"decision", hypothetical,
			"reason_count", len(classification.reasons),
		)
		return attempt, body, nil
	}

	if crossPrincipal && classification.parseErr != nil {
		return attempt, body, newCodexContinuationTerminalError(
			"Codex continuation body could not be safely sanitized for principal migration",
			classification.parseErr,
		)
	}
	if crossPrincipal && classification.kind == codexContinuationIncremental {
		return attempt, body, newCodexContinuationTerminalError(
			"Codex incremental continuation cannot migrate to the selected upstream principal",
			nil,
		)
	}
	if crossPrincipal {
		sanitized, changed, err := SanitizeCodexCrossPrincipalBody(body)
		if err != nil {
			return attempt, body, newCodexContinuationTerminalError(
				"Codex continuation body could not be safely sanitized for principal migration",
				err,
			)
		}
		attempt.sanitized = changed
		body = sanitized
	}
	if ownership.kind == codexContinuationOwnerOwned && ownership.principal == principal.key &&
		classification.kind == codexContinuationIncremental {
		attempt.requireExactConnection = true
	}
	return attempt, body, nil
}

func classifyCodexContinuationBody(body []byte) codexContinuationClassification {
	result := codexContinuationClassification{kind: codexContinuationFull}
	result.previousResponseID = strings.TrimSpace(gjson.GetBytes(body, "previous_response_id").String())
	if result.previousResponseID != "" {
		result.kind = codexContinuationIncremental
		result.reasons = append(result.reasons, "previous_response_id")
	}

	var decoded map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		result.parseErr = fmt.Errorf("decode continuation body: %w", err)
		return result
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		result.parseErr = fmt.Errorf("decode continuation body: %w", err)
		return result
	}

	items := codexContinuationInputItems(decoded["input"])
	fullItemIDs := make(map[string]struct{}, len(items))
	callIDs := make(map[string]struct{})
	for _, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemType := strings.TrimSpace(firstNonEmptyString(itemMap["type"]))
		if itemType != "item_reference" {
			if id := strings.TrimSpace(firstNonEmptyString(itemMap["id"])); id != "" {
				fullItemIDs[id] = struct{}{}
			}
		}
		if isCodexToolCallContextItemType(itemType) {
			if callID := strings.TrimSpace(firstNonEmptyString(itemMap["call_id"])); callID != "" {
				callIDs[callID] = struct{}{}
			}
		}
	}
	for _, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemType := strings.TrimSpace(firstNonEmptyString(itemMap["type"]))
		switch {
		case itemType == "item_reference":
			id := strings.TrimSpace(firstNonEmptyString(itemMap["id"]))
			if _, found := fullItemIDs[id]; id == "" || !found {
				result.kind = codexContinuationIncremental
				result.reasons = append(result.reasons, "unresolved_item_reference")
			}
		case isCodexToolCallOutputItemType(itemType):
			callID := strings.TrimSpace(firstNonEmptyString(itemMap["call_id"]))
			if _, found := callIDs[callID]; callID == "" || !found {
				result.kind = codexContinuationIncremental
				result.reasons = append(result.reasons, "orphan_tool_output")
			}
		}
	}
	return result
}

func codexContinuationInputItems(value any) []any {
	switch input := value.(type) {
	case []any:
		return input
	case map[string]any:
		return []any{input}
	default:
		return nil
	}
}

func (s *OpenAIGatewayService) lookupCodexContinuationOwnership(
	ctx context.Context,
	request *codexSimulationRequestState,
	previousResponseID string,
) codexContinuationOwnership {
	store := s.getCodexSimulationStateStore()
	if store == nil || request == nil {
		return codexContinuationOwnership{kind: codexContinuationOwnerUnknown}
	}
	if previousResponseID != "" {
		if owner, found := s.readCodexContinuationOwner(ctx, store, s.codexResponseOwnerStateKeyForRequest(request, previousResponseID), request.settings.stateTTL()); found {
			return owner
		}
	}
	if owner, found := s.readCodexContinuationOwner(ctx, store, codexRootOwnerStateKey(request.root.rootKey), request.settings.stateTTL()); found {
		return owner
	}
	return codexContinuationOwnership{kind: codexContinuationOwnerUnknown}
}

func (s *OpenAIGatewayService) readCodexContinuationOwner(
	ctx context.Context,
	store *codexSimulationStateStore,
	key string,
	ttl time.Duration,
) (codexContinuationOwnership, bool) {
	value, found, err := store.getWithTTL(ctx, key, ttl)
	if err != nil {
		slog.WarnContext(ctx, "failed to read Codex continuation owner state", "error", err)
	}
	if !found {
		return codexContinuationOwnership{}, false
	}
	value = strings.TrimSpace(value)
	if value == "external" {
		return codexContinuationOwnership{kind: codexContinuationOwnerExternal}, true
	}
	if principal, ok := strings.CutPrefix(value, "owned:"); ok && principal != "" {
		return codexContinuationOwnership{kind: codexContinuationOwnerOwned, principal: principal}, true
	}
	return codexContinuationOwnership{}, false
}

func codexRootOwnerStateKey(rootKey string) string {
	if rootKey == "" {
		return ""
	}
	return codexSimulationStatePrefix + "owner:root:" + rootKey
}

func (s *OpenAIGatewayService) codexResponseOwnerStateKey(responseID string) string {
	settings := s.codexSimulationSettingsSnapshot(context.Background(), nil)
	return codexResponseOwnerStateKeyWithSecret(settings.IdentitySecret, responseID)
}

func (s *OpenAIGatewayService) codexResponseOwnerStateKeyForRequest(request *codexSimulationRequestState, responseID string) string {
	responseID = strings.TrimSpace(responseID)
	if request == nil || responseID == "" {
		return ""
	}
	return codexResponseOwnerStateKeyWithSecret(request.settings.IdentitySecret, responseID)
}

func codexResponseOwnerStateKeyWithSecret(secret, responseID string) string {
	responseID = strings.TrimSpace(responseID)
	if responseID == "" {
		return ""
	}
	digest := codexSimulationHMAC(secret, "response-owner-key:v1", responseID)
	return codexSimulationStatePrefix + "owner:response:" + hex.EncodeToString(digest[:])
}

// SanitizeCodexCrossPrincipalBody produces a self-contained migration body.
// It never mutates the caller's byte slice.
func SanitizeCodexCrossPrincipalBody(body []byte) ([]byte, bool, error) {
	var decoded map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return body, false, fmt.Errorf("decode Codex migration body: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return body, false, fmt.Errorf("decode Codex migration body: %w", err)
	}

	changed := false
	for _, key := range []string{"previous_response_id", "turn_state"} {
		if _, exists := decoded[key]; exists {
			delete(decoded, key)
			changed = true
		}
	}
	if metadata, ok := decoded["client_metadata"].(map[string]any); ok {
		for _, key := range []string{"previous_response_id", "turn_state", "x-codex-turn-state"} {
			if _, exists := metadata[key]; exists {
				delete(metadata, key)
				changed = true
			}
		}
	}

	inputValue, hasInput := decoded["input"]
	items := codexContinuationInputItems(inputValue)
	if hasInput && len(items) > 0 {
		callIDs := make(map[string]struct{})
		outputIDs := make(map[string]struct{})
		for _, item := range items {
			itemMap, ok := item.(map[string]any)
			if !ok || shouldDropCodexAccountBoundItem(itemMap) {
				continue
			}
			itemType := strings.TrimSpace(firstNonEmptyString(itemMap["type"]))
			callID := strings.TrimSpace(firstNonEmptyString(itemMap["call_id"]))
			switch {
			case isCodexToolCallContextItemType(itemType) && callID != "":
				callIDs[callID] = struct{}{}
			case isCodexToolCallOutputItemType(itemType) && callID != "":
				outputIDs[callID] = struct{}{}
			}
		}

		completeCalls := make(map[string]struct{})
		for callID := range callIDs {
			if _, ok := outputIDs[callID]; ok {
				completeCalls[callID] = struct{}{}
			}
		}
		provableIDs := make(map[string]struct{})
		for _, item := range items {
			itemMap, ok := item.(map[string]any)
			if !ok || shouldDropCodexAccountBoundItem(itemMap) || !codexMigrationItemHasCompletePair(itemMap, completeCalls) {
				continue
			}
			itemType := strings.TrimSpace(firstNonEmptyString(itemMap["type"]))
			id := strings.TrimSpace(firstNonEmptyString(itemMap["id"]))
			if itemType != "item_reference" && id != "" && !isCodexAccountBoundItemID(id) {
				provableIDs[id] = struct{}{}
			}
		}

		filtered := make([]any, 0, len(items))
		for _, item := range items {
			itemMap, ok := item.(map[string]any)
			if !ok {
				filtered = append(filtered, item)
				continue
			}
			if shouldDropCodexAccountBoundItem(itemMap) || !codexMigrationItemHasCompletePair(itemMap, completeCalls) {
				changed = true
				continue
			}
			itemType := strings.TrimSpace(firstNonEmptyString(itemMap["type"]))
			if itemType == "item_reference" {
				id := strings.TrimSpace(firstNonEmptyString(itemMap["id"]))
				if _, ok := provableIDs[id]; id == "" || !ok {
					changed = true
					continue
				}
			}
			if id := strings.TrimSpace(firstNonEmptyString(itemMap["id"])); isCodexAccountBoundItemID(id) {
				delete(itemMap, "id")
				changed = true
			}
			filtered = append(filtered, itemMap)
		}
		if _, wasArray := inputValue.([]any); wasArray {
			decoded["input"] = filtered
		} else if len(filtered) == 1 {
			decoded["input"] = filtered[0]
		} else if len(filtered) == 0 {
			delete(decoded, "input")
		} else {
			decoded["input"] = filtered
		}
	}

	if !changed {
		return body, false, nil
	}
	rebuilt, err := marshalOpenAIUpstreamJSON(decoded)
	if err != nil {
		return body, false, fmt.Errorf("serialize Codex migration body: %w", err)
	}
	return rebuilt, true, nil
}

func shouldDropCodexAccountBoundItem(item map[string]any) bool {
	if item == nil {
		return false
	}
	itemType := strings.ToLower(strings.TrimSpace(firstNonEmptyString(item["type"])))
	switch itemType {
	case "compaction", "compaction_summary", "compaction-summary":
		return true
	case "reasoning":
		_, encrypted := item["encrypted_content"]
		return encrypted
	default:
		return false
	}
}

func codexMigrationItemHasCompletePair(item map[string]any, completeCalls map[string]struct{}) bool {
	itemType := strings.TrimSpace(firstNonEmptyString(item["type"]))
	if !isCodexToolCallContextItemType(itemType) && !isCodexToolCallOutputItemType(itemType) {
		return true
	}
	callID := strings.TrimSpace(firstNonEmptyString(item["call_id"]))
	_, ok := completeCalls[callID]
	return callID != "" && ok
}

func isCodexAccountBoundItemID(id string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	return strings.HasPrefix(id, "rs_") || strings.HasPrefix(id, "msg_") || strings.HasPrefix(id, "fc_")
}

func codexContinuationAttemptFromGin(c *gin.Context) (*codexContinuationAttempt, bool) {
	attempt, ok := codexSimulationAttemptFromGin(c)
	if !ok || attempt.continuation == nil {
		return nil, false
	}
	return attempt.continuation, true
}

func codexContinuationRecoveryForbidden(c *gin.Context) bool {
	attempt, ok := codexContinuationAttemptFromGin(c)
	return ok && attempt.mode == codexContinuationEnforce &&
		attempt.classification.kind == codexContinuationIncremental
}

func codexContinuationRequiresExactConnection(c *gin.Context) bool {
	attempt, ok := codexContinuationAttemptFromGin(c)
	return ok && attempt.mode == codexContinuationEnforce && attempt.requireExactConnection
}

func (s *OpenAIGatewayService) codexContinuationPreferredConnection(
	c *gin.Context,
	stateStore OpenAIWSStateStore,
	previousResponseID string,
) string {
	if !codexContinuationRequiresExactConnection(c) || stateStore == nil {
		return ""
	}
	if previousResponseID != "" {
		if connID, ok := stateStore.GetResponseConn(previousResponseID); ok {
			return connID
		}
	}
	attempt, ok := codexSimulationAttemptFromGin(c)
	if !ok || attempt.request == nil {
		return ""
	}
	if connID, found := stateStore.GetSessionConn(getOpenAIGroupIDFromContext(c), attempt.request.root.rootKey); found {
		return connID
	}
	return ""
}

func (s *OpenAIGatewayService) markCodexContinuationExternalOnInvalidEncryptedContent(ctx context.Context, c *gin.Context) {
	attempt, ok := codexSimulationAttemptFromGin(c)
	if !ok || attempt.request == nil || attempt.continuation == nil ||
		attempt.continuation.mode != codexContinuationEnforce ||
		attempt.continuation.ownership.kind != codexContinuationOwnerUnknown {
		return
	}
	store := s.getCodexSimulationStateStore()
	if store == nil {
		return
	}
	if responseID := attempt.continuation.classification.previousResponseID; responseID != "" {
		if err := store.setWithTTL(ctx, s.codexResponseOwnerStateKeyForRequest(attempt.request, responseID), "external", attempt.request.settings.stateTTL()); err != nil {
			slog.WarnContext(ctx, "failed to persist Codex external response tombstone", "error", err)
		}
	}
	if err := store.setWithTTL(ctx, codexRootOwnerStateKey(attempt.request.root.rootKey), "external", attempt.request.settings.stateTTL()); err != nil {
		slog.WarnContext(ctx, "failed to persist Codex external root tombstone", "error", err)
	}
}

func (s *OpenAIGatewayService) completeCodexSimulationSuccess(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	responseID string,
	connID string,
) {
	attempt, ok := codexSimulationAttemptFromGin(c)
	if !ok || attempt.request == nil || attempt.principal.key == "" || account == nil || !account.IsOpenAIOAuth() {
		return
	}
	if principal := s.codexSimulationPrincipalForAccountWithSecret(account, attempt.request.settings.IdentitySecret); principal.key != attempt.principal.key {
		return
	}
	store := s.getCodexSimulationStateStore()
	if store == nil {
		return
	}
	if attempt.continuation != nil && attempt.continuation.mode == codexContinuationEnforce {
		ownerValue := "owned:" + attempt.principal.key
		if err := store.setWithTTL(ctx, codexRootOwnerStateKey(attempt.request.root.rootKey), ownerValue, attempt.request.settings.stateTTL()); err != nil {
			slog.WarnContext(ctx, "failed to persist Codex root owner", "error", err)
		}
		if strings.TrimSpace(responseID) != "" {
			if err := store.setWithTTL(ctx, s.codexResponseOwnerStateKeyForRequest(attempt.request, responseID), ownerValue, attempt.request.settings.stateTTL()); err != nil {
				slog.WarnContext(ctx, "failed to persist Codex response owner", "error", err)
			}
		}
	}
	if attempt.fingerprint != nil && attempt.fingerprint.fullSimulation &&
		(isOpenAIResponsesCompactPath(c) || isOpenAINativeCompactionV2(c)) {
		expectedGeneration := attempt.fingerprint.generation
		nextGeneration := expectedGeneration + 1
		key := codexSimulationGenerationStateKey(attempt.request.root.rootKey, attempt.principal.key)
		if err := store.advanceGenerationWithTTL(ctx, key, expectedGeneration, nextGeneration, attempt.request.settings.stateTTL()); err != nil {
			// A lost acknowledgement can drift window metadata but must not turn a
			// successful upstream response into a client-visible failure.
			slog.WarnContext(ctx, "failed to persist Codex window generation", "error", err)
		}
	}
	if connID != "" && attempt.continuation != nil && attempt.continuation.mode == codexContinuationEnforce {
		wsState := s.getOpenAIWSStateStore()
		if wsState != nil {
			ttl := attempt.request.settings.stateTTL()
			wsState.BindSessionConn(getOpenAIGroupIDFromContext(c), attempt.request.root.rootKey, connID, ttl)
			if responseID != "" {
				wsState.BindResponseConn(responseID, connID, ttl)
			}
		}
	}
}

package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestClassifyCodexContinuationBodyHardSignals(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantKind    codexContinuationBodyKind
		wantReasons []string
	}{
		{name: "full plaintext", body: `{"input":[{"type":"message","id":"stable","content":"hello"}]}`, wantKind: codexContinuationFull},
		{name: "previous response", body: `{"previous_response_id":"resp_1","input":"next"}`, wantKind: codexContinuationIncremental, wantReasons: []string{"previous_response_id"}},
		{name: "resolved item reference", body: `{"input":[{"type":"message","id":"stable","content":"hello"},{"type":"item_reference","id":"stable"}]}`, wantKind: codexContinuationFull},
		{name: "unresolved item reference", body: `{"input":[{"type":"item_reference","id":"missing"}]}`, wantKind: codexContinuationIncremental, wantReasons: []string{"unresolved_item_reference"}},
		{name: "paired tool output", body: `{"input":[{"type":"function_call","call_id":"call_1"},{"type":"function_call_output","call_id":"call_1","output":"ok"}]}`, wantKind: codexContinuationFull},
		{name: "orphan tool output", body: `{"input":[{"type":"function_call_output","call_id":"call_1","output":"orphan"}]}`, wantKind: codexContinuationIncremental, wantReasons: []string{"orphan_tool_output"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			classification := classifyCodexContinuationBody([]byte(test.body))
			require.NoError(t, classification.parseErr)
			require.Equal(t, test.wantKind, classification.kind)
			for _, reason := range test.wantReasons {
				require.Contains(t, classification.reasons, reason)
			}
		})
	}
}

func TestSanitizeCodexCrossPrincipalBody(t *testing.T) {
	original := []byte(`{
		"previous_response_id":"resp_old",
		"turn_state":"state_old",
		"client_metadata":{"preserved":true,"turn_state":"state_old","x-codex-turn-state":"state_old"},
		"input":[
			{"type":"message","id":"msg_account_bound","role":"user","content":"keep plaintext"},
			{"type":"message","id":"stable-item","role":"user","content":"stable reference target"},
			{"type":"item_reference","id":"stable-item"},
			{"type":"item_reference","id":"missing-item"},
			{"type":"reasoning","id":"rs_encrypted","encrypted_content":"ciphertext","summary":"drop me"},
			{"type":"reasoning","id":"rs_plain","summary":"keep plaintext reasoning"},
			{"type":"compaction","id":"cmp_1","encrypted_content":"ciphertext"},
			{"type":"function_call","id":"fc_call","call_id":"call-ok","name":"tool","arguments":"{}"},
			{"type":"function_call_output","id":"msg_output","call_id":"call-ok","output":"ok"},
			{"type":"function_call","id":"fc_orphan","call_id":"call-without-output","name":"tool","arguments":"{}"},
			{"type":"function_call_output","id":"msg_orphan","call_id":"output-without-call","output":"bad"}
		]
	}`)
	originalCopy := append([]byte(nil), original...)

	sanitized, changed, err := SanitizeCodexCrossPrincipalBody(original)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, originalCopy, original, "sanitization must not mutate the canonical input bytes")
	require.False(t, gjson.GetBytes(sanitized, "previous_response_id").Exists())
	require.False(t, gjson.GetBytes(sanitized, "turn_state").Exists())
	require.True(t, gjson.GetBytes(sanitized, "client_metadata.preserved").Bool())
	require.False(t, gjson.GetBytes(sanitized, "client_metadata.turn_state").Exists())
	require.NotContains(t, string(sanitized), "ciphertext")
	require.NotContains(t, string(sanitized), "missing-item")
	require.NotContains(t, string(sanitized), "call-without-output")
	require.NotContains(t, string(sanitized), "output-without-call")
	require.Contains(t, string(sanitized), "keep plaintext")
	require.Contains(t, string(sanitized), "keep plaintext reasoning")
	require.Contains(t, string(sanitized), "stable-item")
	require.Contains(t, string(sanitized), "call-ok")
	require.Equal(t, 2, countCodexCallIDItems(sanitized, "call-ok"))

	items := gjson.GetBytes(sanitized, "input").Array()
	for _, item := range items {
		id := strings.TrimSpace(item.Get("id").String())
		require.False(t, isCodexAccountBoundItemID(id), "account-bound item IDs must be stripped")
	}
}

func TestCodexContinuationEnforceOwnershipMatrix(t *testing.T) {
	tests := []struct {
		name             string
		owner            codexContinuationOwnerKind
		samePrincipal    bool
		incremental      bool
		wantError        bool
		wantSanitized    bool
		wantExactConnect bool
	}{
		{name: "unknown full", owner: codexContinuationOwnerUnknown},
		{name: "unknown incremental", owner: codexContinuationOwnerUnknown, incremental: true},
		{name: "same principal full", owner: codexContinuationOwnerOwned, samePrincipal: true},
		{name: "same principal incremental", owner: codexContinuationOwnerOwned, samePrincipal: true, incremental: true, wantExactConnect: true},
		{name: "other principal full", owner: codexContinuationOwnerOwned, wantSanitized: true},
		{name: "other principal incremental", owner: codexContinuationOwnerOwned, incremental: true, wantError: true},
		{name: "external full", owner: codexContinuationOwnerExternal, wantSanitized: true},
		{name: "external incremental", owner: codexContinuationOwnerExternal, incremental: true, wantError: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			svc := newCodexSimulationTestService(false, codexContinuationEnforce)
			c := newCodexSimulationTestContext("/v1/responses")
			c.Request.Header.Set("thread-id", "matrix-thread")
			body := []byte(`{"turn_state":"bound-state","input":[{"type":"reasoning","encrypted_content":"ciphertext"},{"type":"message","content":"plaintext"}]}`)
			if test.incremental {
				body = []byte(`{"previous_response_id":"resp_matrix","input":"next"}`)
			}
			svc.PrepareCodexSimulationRequest(c, 1, nil, body)
			request, ok := codexSimulationRequestStateFromGin(c)
			require.True(t, ok)
			account := openAIFingerprintAccount(51, map[string]any{codexFingerprintModeExtraKey: "full"})
			account.Credentials = map[string]any{"chatgpt_account_id": "matrix-principal"}
			principal := svc.resolveCodexSimulationPrincipal(account)
			store := svc.getCodexSimulationStateStore()
			switch test.owner {
			case codexContinuationOwnerOwned:
				ownerPrincipal := strings.Repeat("f", 64)
				if test.samePrincipal {
					ownerPrincipal = principal.key
				}
				require.NoError(t, store.set(context.Background(), codexRootOwnerStateKey(request.root.rootKey), "owned:"+ownerPrincipal))
			case codexContinuationOwnerExternal:
				require.NoError(t, store.set(context.Background(), codexRootOwnerStateKey(request.root.rootKey), "external"))
			}

			attempt, prepared, err := svc.prepareCodexContinuationAttempt(context.Background(), c, request, principal, body)
			if test.wantError {
				require.Error(t, err)
				var terminalErr *CodexContinuationTerminalError
				require.ErrorAs(t, err, &terminalErr)
				var failoverErr *UpstreamFailoverError
				require.False(t, errors.As(err, &failoverErr), "principal mismatch must not enter account failover")
				return
			}
			require.NoError(t, err)
			require.Equal(t, test.wantSanitized, attempt.sanitized)
			require.Equal(t, test.wantExactConnect, attempt.requireExactConnection)
			if test.wantSanitized {
				require.False(t, gjson.GetBytes(prepared, "turn_state").Exists())
				require.NotContains(t, string(prepared), "ciphertext")
			} else {
				require.Equal(t, body, prepared)
			}
		})
	}
}

func TestCodexContinuationParseFailureClosesOnlyCrossPrincipalEnforce(t *testing.T) {
	invalidBody := []byte(`{"input":`)
	svc := newCodexSimulationTestService(false, codexContinuationEnforce)
	c := newCodexSimulationTestContext("/v1/responses")
	c.Request.Header.Set("thread-id", "parse-thread")
	svc.PrepareCodexSimulationRequest(c, 1, nil, invalidBody)
	request, ok := codexSimulationRequestStateFromGin(c)
	require.True(t, ok)
	account := openAIFingerprintAccount(61, nil)
	account.Credentials = map[string]any{"chatgpt_account_id": "parse-principal"}
	principal := svc.resolveCodexSimulationPrincipal(account)

	_, prepared, err := svc.prepareCodexContinuationAttempt(context.Background(), c, request, principal, invalidBody)
	require.NoError(t, err)
	require.Equal(t, invalidBody, prepared, "unknown ownership may pass an unparsable body to existing validation")

	require.NoError(t, svc.getCodexSimulationStateStore().set(context.Background(), codexRootOwnerStateKey(request.root.rootKey), "external"))
	_, _, err = svc.prepareCodexContinuationAttempt(context.Background(), c, request, principal, invalidBody)
	require.Error(t, err)
	var terminalErr *CodexContinuationTerminalError
	require.ErrorAs(t, err, &terminalErr)
}

func TestCodexContinuationShadowDoesNotMutateOrReject(t *testing.T) {
	svc := newCodexSimulationTestService(false, codexContinuationShadow)
	c := newCodexSimulationTestContext("/v1/responses")
	c.Request.Header.Set("thread-id", "shadow-thread")
	body := []byte(`{"previous_response_id":"resp_shadow","input":"next"}`)
	account := openAIFingerprintAccount(71, nil)
	account.Credentials = map[string]any{"chatgpt_account_id": "shadow-principal"}

	svc.PrepareCodexSimulationRequest(c, 1, nil, body)
	prepared, err := svc.PrepareCodexSimulationAttempt(context.Background(), c, account, body)
	require.NoError(t, err)
	require.Equal(t, body, prepared)
	svc.completeCodexSimulationSuccess(context.Background(), c, account, "resp_shadow_new", "conn-shadow")

	request, ok := codexSimulationRequestStateFromGin(c)
	require.True(t, ok)
	store := svc.getCodexSimulationStateStore()
	_, found, err := store.get(context.Background(), codexRootOwnerStateKey(request.root.rootKey))
	require.NoError(t, err)
	require.False(t, found)
	_, found, err = store.get(context.Background(), svc.codexResponseOwnerStateKey("resp_shadow_new"))
	require.NoError(t, err)
	require.False(t, found)
}

func TestCodexContinuationSuccessAndInvalidEncryptedState(t *testing.T) {
	svc := newCodexSimulationTestService(false, codexContinuationEnforce)
	account := openAIFingerprintAccount(81, nil)
	account.Credentials = map[string]any{"chatgpt_account_id": "owner-principal"}
	c := newCodexSimulationTestContext("/v1/responses")
	c.Request.Header.Set("thread-id", "owner-thread")
	fullBody := []byte(`{"input":"hello"}`)

	svc.PrepareCodexSimulationRequest(c, 1, nil, fullBody)
	_, err := svc.PrepareCodexSimulationAttempt(context.Background(), c, account, fullBody)
	require.NoError(t, err)
	svc.completeCodexSimulationSuccess(context.Background(), c, account, "resp_owner", "conn-owner")
	request, ok := codexSimulationRequestStateFromGin(c)
	require.True(t, ok)
	principal := svc.resolveCodexSimulationPrincipal(account)
	store := svc.getCodexSimulationStateStore()
	requireCodexOwnerValue(t, store, codexRootOwnerStateKey(request.root.rootKey), "owned:"+principal.key)
	requireCodexOwnerValue(t, store, svc.codexResponseOwnerStateKey("resp_owner"), "owned:"+principal.key)
	require.Len(t, store.entries, 2, "only response and root ownership belong in the simulation store")
	rootStateKey := codexRootOwnerStateKey(request.root.rootKey)
	firstExpiry := store.entries[rootStateKey].expiresAt
	time.Sleep(time.Millisecond)
	svc.completeCodexSimulationSuccess(context.Background(), c, account, "", "")
	require.True(t, store.entries[rootStateKey].expiresAt.After(firstExpiry), "a successful turn must refresh root ownership TTL")

	incremental := []byte(`{"previous_response_id":"resp_owner","input":"next"}`)
	_, err = svc.PrepareCodexSimulationAttempt(context.Background(), c, account, incremental)
	require.NoError(t, err)
	require.True(t, codexContinuationRequiresExactConnection(c))
	require.Equal(t, "conn-owner", svc.codexContinuationPreferredConnection(c, svc.getOpenAIWSStateStore(), "resp_owner"))

	unknownContext := newCodexSimulationTestContext("/v1/responses")
	unknownContext.Request.Header.Set("thread-id", "unknown-thread")
	unknownBody := []byte(`{"previous_response_id":"resp_external","input":"next"}`)
	svc.PrepareCodexSimulationRequest(unknownContext, 1, nil, unknownBody)
	_, err = svc.PrepareCodexSimulationAttempt(context.Background(), unknownContext, account, unknownBody)
	require.NoError(t, err)
	svc.markCodexContinuationExternalOnInvalidEncryptedContent(context.Background(), unknownContext)
	unknownRequest, ok := codexSimulationRequestStateFromGin(unknownContext)
	require.True(t, ok)
	requireCodexOwnerValue(t, store, codexRootOwnerStateKey(unknownRequest.root.rootKey), "external")
	requireCodexOwnerValue(t, store, svc.codexResponseOwnerStateKey("resp_external"), "external")
}

func TestCodexSimulationStateFallsBackLocallyAndExpires(t *testing.T) {
	shared := &codexSimulationSharedStateStub{setErr: errors.New("redis unavailable"), getErr: errors.New("redis unavailable")}
	store := &codexSimulationStateStore{
		shared:  shared,
		ttl:     20 * time.Millisecond,
		entries: make(map[string]codexSimulationStateBinding),
	}
	store.lastCleanupUnixNano.Store(time.Now().UnixNano())

	err := store.set(context.Background(), "key", "owned:principal")
	require.Error(t, err)
	value, found, err := store.get(context.Background(), "key")
	require.Error(t, err)
	require.True(t, found)
	require.Equal(t, "owned:principal", value)

	require.Eventually(t, func() bool {
		_, found, _ := store.get(context.Background(), "key")
		return !found
	}, time.Second, 10*time.Millisecond)
}

func countCodexCallIDItems(body []byte, callID string) int {
	count := 0
	for _, item := range gjson.GetBytes(body, "input").Array() {
		if item.Get("call_id").String() == callID {
			count++
		}
	}
	return count
}

func requireCodexOwnerValue(t *testing.T, store *codexSimulationStateStore, key, want string) {
	t.Helper()
	value, found, err := store.get(context.Background(), key)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, want, value)
}

type codexSimulationSharedStateStub struct {
	setErr error
	getErr error
}

func (s *codexSimulationSharedStateStub) SetOpenAIWSState(context.Context, string, string, time.Duration) error {
	return s.setErr
}

func (s *codexSimulationSharedStateStub) GetOpenAIWSState(context.Context, string) (string, bool, error) {
	return "", false, s.getErr
}

func (s *codexSimulationSharedStateStub) DeleteOpenAIWSState(context.Context, string) error {
	return nil
}

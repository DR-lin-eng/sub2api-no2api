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

func TestCodexContinuationSchedulingAffinityFiltersToRecordedOwnerAccount(t *testing.T) {
	svc := newCodexSimulationTestService(false, codexContinuationEnforce)
	c := newCodexSimulationTestContext("/v1/responses")
	c.Request.Header.Set("thread-id", "affinity-thread")
	body := []byte(`{"previous_response_id":"resp_affinity","input":"next"}`)
	svc.PrepareCodexSimulationRequest(c, 1, nil, body)
	request, ok := codexSimulationRequestStateFromGin(c)
	require.True(t, ok)

	owner := openAIFingerprintAccount(91, nil)
	owner.Credentials = map[string]any{"chatgpt_account_id": "affinity-principal"}
	ownerPrincipal := svc.resolveCodexSimulationPrincipal(owner)
	ownerKey := svc.codexResponseOwnerStateKeyForRequest(request, "resp_affinity")
	store := svc.getCodexSimulationStateStore()
	require.NoError(t, store.set(
		context.Background(),
		ownerKey,
		"owned:"+ownerPrincipal.key,
	))
	require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(context.Background(), 0, "resp_affinity", owner.ID, time.Hour))

	schedulingCtx, err := svc.WithCodexContinuationSchedulingAffinity(context.Background(), c, body)
	require.NoError(t, err)
	require.True(t, codexContinuationSchedulingMatches(schedulingCtx, owner))
	require.Equal(t, []int64{owner.ID}, schedulerCandidatePriorityIDs(schedulingCtx))
	metadataOwner := *owner
	metadataOwner.Credentials = nil
	metadataOwner.Extra = map[string]any{CodexVirtualClientKeyExtraKey: owner.CodexVirtualClientKey()}
	require.True(t, codexContinuationSchedulingMatches(schedulingCtx, &metadataOwner), "scheduler metadata must preserve the virtual-client namespace")

	samePrincipal := openAIFingerprintAccount(92, nil)
	samePrincipal.Credentials = map[string]any{"chatgpt_account_id": "affinity-principal"}
	require.False(t, codexContinuationSchedulingMatches(schedulingCtx, samePrincipal), "the original WebSocket belongs to the recorded local account pool")

	otherPrincipal := openAIFingerprintAccount(93, nil)
	otherPrincipal.Credentials = map[string]any{"chatgpt_account_id": "other-principal"}
	require.False(t, codexContinuationSchedulingMatches(schedulingCtx, otherPrincipal))

	apiKeyAccount := &Account{ID: 94, Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	require.False(t, codexContinuationSchedulingMatches(schedulingCtx, apiKeyAccount))

	// The process-local connection ID preserves exact account affinity even after
	// the shared response-account binding expires.
	require.NoError(t, svc.getOpenAIWSStateStore().DeleteResponseAccount(context.Background(), 0, "resp_affinity"))
	svc.getOpenAIWSStateStore().BindResponseConn("resp_affinity", "oa_ws_91_7", time.Hour)
	connectionSchedulingCtx, err := svc.WithCodexContinuationSchedulingAffinity(context.Background(), c, body)
	require.NoError(t, err)
	require.Equal(t, []int64{owner.ID}, schedulerCandidatePriorityIDs(connectionSchedulingCtx))
	require.False(t, codexContinuationSchedulingMatches(connectionSchedulingCtx, samePrincipal))

	// Owner state remains compatible with older nodes. When neither account nor
	// local connection affinity exists, scheduling safely falls back to principal.
	svc.getOpenAIWSStateStore().DeleteResponseConn("resp_affinity")
	legacySchedulingCtx, err := svc.WithCodexContinuationSchedulingAffinity(context.Background(), c, body)
	require.NoError(t, err)
	require.True(t, codexContinuationSchedulingMatches(legacySchedulingCtx, samePrincipal))
	require.Empty(t, schedulerCandidatePriorityIDs(legacySchedulingCtx))
}

func TestCodexContinuationSchedulingAffinitySelectsOwnerBeforeLoadBalance(t *testing.T) {
	svc := newCodexSimulationTestService(false, codexContinuationEnforce)
	owner := Account{
		ID: 101, Name: "owner", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 1,
		Credentials: map[string]any{"access_token": "owner-token", "chatgpt_account_id": "affinity-owner"},
	}
	other := Account{
		ID: 102, Name: "other", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0,
		Credentials: map[string]any{"access_token": "other-token", "chatgpt_account_id": "affinity-other"},
	}
	svc.accountRepo = schedulerTestOpenAIAccountRepo{accounts: []Account{other, owner}}

	c := newCodexSimulationTestContext("/v1/responses")
	c.Request.Header.Set("thread-id", "affinity-selection-thread")
	body := []byte(`{"input":[{"type":"item_reference","id":"missing"}]}`)
	svc.PrepareCodexSimulationRequest(c, 1, nil, body)
	request, ok := codexSimulationRequestStateFromGin(c)
	require.True(t, ok)
	ownerPrincipal := svc.codexSimulationPrincipalForAccountWithSecret(&owner, request.settings.IdentitySecret)
	rootOwnerKey := codexRootOwnerStateKey(request.root.rootKey)
	store := svc.getCodexSimulationStateStore()
	require.NoError(t, store.set(
		context.Background(),
		rootOwnerKey,
		"owned:"+ownerPrincipal.key,
	))
	svc.getOpenAIWSStateStore().BindSessionConn(0, request.root.rootKey, "oa_ws_101_9", time.Hour)

	schedulingCtx, err := svc.WithCodexContinuationSchedulingAffinity(context.Background(), c, body)
	require.NoError(t, err)
	require.Equal(t, []int64{owner.ID}, schedulerCandidatePriorityIDs(schedulingCtx))
	selection, _, err := svc.SelectAccountWithSchedulerForCapability(
		schedulingCtx,
		nil,
		"",
		"affinity-selection-session",
		"gpt-5.4",
		nil,
		OpenAIUpstreamTransportAny,
		"",
		false,
		false,
		true,
	)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.NotNil(t, selection.Account)
	require.Equal(t, owner.ID, selection.Account.ID, "load balancing must not select the higher-priority wrong principal")
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}

	advanced := newDefaultOpenAIAccountScheduler(svc, newOpenAIAccountRuntimeStats())
	advancedSelection, _, err := advanced.Select(schedulingCtx, OpenAIAccountScheduleRequest{
		Platform:             PlatformOpenAI,
		SessionHash:          "affinity-advanced-session",
		RequestedModel:       "gpt-5.4",
		RequiredTransport:    OpenAIUpstreamTransportAny,
		UseUpstreamTokenCost: true,
	})
	require.NoError(t, err)
	require.NotNil(t, advancedSelection)
	require.NotNil(t, advancedSelection.Account)
	require.Equal(t, owner.ID, advancedSelection.Account.ID, "advanced scheduler must enforce the same owner-principal constraint")
	if advancedSelection.ReleaseFunc != nil {
		advancedSelection.ReleaseFunc()
	}

	metadataCandidate := owner
	metadataCandidate.Credentials = map[string]any{"model_mapping": map[string]any{"gpt-5.4": "gpt-5.4"}}
	metadataCandidate.Extra = nil
	svc.schedulerSnapshot = &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{
		accountsByID: map[int64]*Account{owner.ID: &owner},
	}}
	require.True(t, svc.codexContinuationCandidateMatches(schedulingCtx, &metadataCandidate), "legacy scheduler metadata should hydrate the full account before rejecting owner affinity")

	mutatedOwner := owner
	mutatedOwner.Credentials = map[string]any{"access_token": "rotated-token", "chatgpt_account_id": "different-principal"}
	svc.accountRepo = schedulerTestOpenAIAccountRepo{accounts: []Account{mutatedOwner}}
	staleOwner := owner
	svc.schedulerSnapshot = &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{
		accountsByID: map[int64]*Account{owner.ID: &staleOwner},
	}}
	_, err = svc.RefreshCodexContinuationSchedulingAccount(schedulingCtx, &owner)
	require.ErrorIs(t, err, ErrAccountSchedulingChanged, "authoritative DB principal changes must beat a stale scheduler snapshot before forward")

	expensiveRate := 0.8
	expensiveOwner := owner
	expensiveOwner.RateMultiplier = &expensiveRate
	expensiveOwner.UpdatedAt = time.Unix(2, 0)
	staleOwner.UpdatedAt = time.Unix(1, 0)
	svc.accountRepo = schedulerTestOpenAIAccountRepo{accounts: []Account{expensiveOwner}}
	svc.schedulerSnapshot = &SchedulerSnapshotService{cache: &openAISnapshotCacheStub{
		accountsByID: map[int64]*Account{owner.ID: &staleOwner},
	}}
	profitCtx := context.WithValue(schedulingCtx, profitControlGateCtxKey{}, &profitControlGate{threshold: 0.5})
	refreshed, err := svc.RefreshCodexContinuationSchedulingAccount(profitCtx, &owner)
	require.NoError(t, err)
	latest, vetoed, reason := svc.ProfitControlVetoLatest(profitCtx, refreshed)
	require.Same(t, refreshed, latest)
	require.True(t, vetoed, "profit control must evaluate the authoritative refreshed account")
	require.Equal(t, profitControlFilterReasonThreshold, reason)
}

func TestCodexContinuationSchedulingAffinityLeavesFullAndUnknownRequestsUnrestricted(t *testing.T) {
	svc := newCodexSimulationTestService(false, codexContinuationEnforce)
	otherPrincipal := openAIFingerprintAccount(95, nil)
	otherPrincipal.Credentials = map[string]any{"chatgpt_account_id": "other-principal"}

	fullContext := newCodexSimulationTestContext("/v1/responses")
	fullContext.Request.Header.Set("thread-id", "affinity-full-thread")
	fullBody := []byte(`{"input":"complete history"}`)
	svc.PrepareCodexSimulationRequest(fullContext, 1, nil, fullBody)
	fullRequest, ok := codexSimulationRequestStateFromGin(fullContext)
	require.True(t, ok)
	require.NoError(t, svc.getCodexSimulationStateStore().set(
		context.Background(),
		codexRootOwnerStateKey(fullRequest.root.rootKey),
		"owned:"+strings.Repeat("a", 64),
	))
	fullSchedulingCtx, err := svc.WithCodexContinuationSchedulingAffinity(context.Background(), fullContext, fullBody)
	require.NoError(t, err)
	require.True(t, codexContinuationSchedulingMatches(fullSchedulingCtx, otherPrincipal), "self-contained full history may migrate through the sanitizer")

	unknownContext := newCodexSimulationTestContext("/v1/responses")
	unknownContext.Request.Header.Set("thread-id", "affinity-unknown-thread")
	unknownBody := []byte(`{"previous_response_id":"resp_unknown","input":"next"}`)
	svc.PrepareCodexSimulationRequest(unknownContext, 1, nil, unknownBody)
	unknownSchedulingCtx, err := svc.WithCodexContinuationSchedulingAffinity(context.Background(), unknownContext, unknownBody)
	require.NoError(t, err)
	require.True(t, codexContinuationSchedulingMatches(unknownSchedulingCtx, otherPrincipal), "unknown ownership keeps the existing first-attempt behavior")
}

func TestCodexContinuationSchedulingAffinityRejectsExternalIncrementalState(t *testing.T) {
	svc := newCodexSimulationTestService(false, codexContinuationEnforce)
	c := newCodexSimulationTestContext("/v1/responses")
	c.Request.Header.Set("thread-id", "affinity-external-thread")
	body := []byte(`{"previous_response_id":"resp_external_affinity","input":"next"}`)
	svc.PrepareCodexSimulationRequest(c, 1, nil, body)
	request, ok := codexSimulationRequestStateFromGin(c)
	require.True(t, ok)
	require.NoError(t, svc.getCodexSimulationStateStore().set(
		context.Background(),
		svc.codexResponseOwnerStateKeyForRequest(request, "resp_external_affinity"),
		"external",
	))

	_, err := svc.WithCodexContinuationSchedulingAffinity(context.Background(), c, body)
	var terminalErr *CodexContinuationTerminalError
	require.ErrorAs(t, err, &terminalErr)
	require.Contains(t, terminalErr.Message, "cannot migrate")
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
	rootStateKey := codexRootOwnerStateKey(request.root.rootKey)
	requireCodexOwnerValue(t, store, rootStateKey, "owned:"+principal.key)
	requireCodexOwnerValue(t, store, svc.codexResponseOwnerStateKey("resp_owner"), "owned:"+principal.key)
	require.Len(t, store.entries, 2, "only response and root ownership belong in the simulation store")
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

func TestCodexContinuationAccountIDFromConnID(t *testing.T) {
	require.Equal(t, int64(91), codexContinuationAccountIDFromConnID("oa_ws_91_7"))
	for _, invalid := range []string{"", "oa_ws_", "oa_ws_0_1", "oa_ws_91_0", "oa_ws_bad_1", "other_91_1"} {
		require.Zero(t, codexContinuationAccountIDFromConnID(invalid), invalid)
	}
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

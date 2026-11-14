package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestCodexFullSimulationCacheIdentitySurvivesNewRequests(t *testing.T) {
	account := openAIFingerprintAccount(601, map[string]any{codexFingerprintModeExtraKey: "full"})
	account.Credentials = map[string]any{"chatgpt_account_id": "cache-principal"}
	for _, source := range []string{"thread-id", "session-id", "prompt_cache_key"} {
		t.Run(source, func(t *testing.T) {
			shared := &stubOpenAIWSSharedCache{}
			var first *codexFingerprintIDs
			var firstWSHeaders http.Header
			for index, path := range []string{"/v1/responses", "/v1/responses", "/v1/responses/compact"} {
				// Independent services and request times model reconnects and other
				// replicas sharing Redis without relying on sleeps or local state.
				svc := newCodexSimulationTestService(true, codexContinuationOff)
				svc.cache = shared
				c := newCodexSimulationTestContext(path)
				body := []byte(`{"model":"gpt-5.5","input":"hello","prompt_cache_key":"client-cache"}`)
				if source != "prompt_cache_key" {
					c.Request.Header.Set(source, "same-conversation")
				}
				if index > 0 {
					body = []byte(`{"model":"gpt-5.5","input":[{"role":"user","content":"hello"},{"role":"assistant","content":"hi"},{"role":"user","content":"continue"}],"prompt_cache_key":"client-cache"}`)
				}
				svc.PrepareCodexSimulationRequest(c, 77, nil, body)
				state, ok := codexSimulationRequestStateFromGin(c)
				require.True(t, ok)
				state.root.createdAtMS = 1793577600000 + int64(index)*60000
				_, err := svc.PrepareCodexSimulationAttempt(context.Background(), c, account, body)
				require.NoError(t, err)
				ids := resolveCodexFingerprintIDsFromGinContext(account, c)
				require.NotNil(t, ids)
				if first == nil {
					first = ids
				} else {
					require.Equal(t, first.sessionID, ids.sessionID, "new requests must keep the upstream cache partition")
					require.Equal(t, first.threadID, ids.threadID)
					require.Equal(t, first.windowID, ids.windowID)
					require.Equal(t, first.promptCacheKey, ids.promptCacheKey)
					require.NotEqual(t, first.turnID, ids.turnID, "turn tracing remains request scoped")
				}

				// Decoded forwarding and raw passthrough consume the same plan.
				raw, changed, err := applyCodexFingerprintClientMetadataToBody(body, ids, c)
				require.NoError(t, err)
				require.True(t, changed)
				var decoded map[string]any
				require.NoError(t, json.Unmarshal(body, &decoded))
				require.True(t, applyCodexFingerprintClientMetadata(decoded, ids, c))
				require.Equal(t, ids.sessionID, decoded["prompt_cache_key"])
				require.Equal(t, ids.sessionID, gjson.GetBytes(raw, "prompt_cache_key").String())
				req, err := svc.buildUpstreamRequestWithFingerprint(context.Background(), c, account, raw, "token", true, ids.promptCacheKey, true, ids)
				require.NoError(t, err)
				require.Equal(t, ids.sessionID, req.Header.Get("session-id"))
				require.Equal(t, ids.threadID, req.Header.Get("thread-id"))

				wsHeaders := make(http.Header)
				applyCodexFingerprintWSHeaders(wsHeaders, ids)
				require.Equal(t, req.Header.Get("session-id"), wsHeaders.Get("session-id"))
				switch index {
				case 0:
					firstWSHeaders = wsHeaders
				case 1:
					require.Equal(t, firstWSHeaders.Get(codexFingerprintWSKeyHeader), wsHeaders.Get(codexFingerprintWSKeyHeader), "WS reconnects must keep the same session compatibility key")
				}
			}
		})
	}
}

func TestCodexFullSimulationCacheIdentityForward(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("passthrough=%t/stream=%t", passthrough, stream), func(t *testing.T) {
				shared := &stubOpenAIWSSharedCache{}
				partitions := make(map[string]bool)
				var cacheKey string
				for index := range 2 {
					svc := newCodexSimulationTestService(true, codexContinuationOff)
					svc.cache = shared
					upstream := &httpUpstreamRecorder{}
					upstream.onDo = func() {
						partition := gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String()
						cached := 0
						if partitions[partition] {
							cached = 1024
						}
						partitions[partition] = true
						wire := fmt.Sprintf(`data: {"type":"response.completed","response":{"id":"resp_cache","model":"gpt-5.5","status":"completed","output":[],"usage":{"input_tokens":2048,"output_tokens":1,"input_tokens_details":{"cached_tokens":%d}}}}`, cached) + "\n\n"
						upstream.resp = &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Type": {"text/event-stream"}},
							Body:       io.NopCloser(strings.NewReader(wire)),
						}
					}
					svc.httpUpstream = upstream
					account := openAIFingerprintAccount(604, map[string]any{codexFingerprintModeExtraKey: "full", "openai_passthrough": passthrough})
					account.Credentials = map[string]any{"access_token": "test-token", "chatgpt_account_id": "cache-forward-principal"}
					c := newCodexSimulationTestContext("/v1/responses")
					c.Request.Header.Set("thread-id", "same-forward-conversation")
					body := []byte(fmt.Sprintf(`{"model":"gpt-5.5","stream":%t,"input":"hello"}`, stream))
					svc.PrepareCodexSimulationRequest(c, 77, nil, body)
					state, ok := codexSimulationRequestStateFromGin(c)
					require.True(t, ok)
					state.root.createdAtMS = 1793577600000 + int64(index)*60000
					prepared, err := svc.PrepareCodexSimulationAttempt(context.Background(), c, account, body)
					require.NoError(t, err)
					result, err := svc.Forward(context.Background(), c, account, prepared)
					require.NoError(t, err)
					require.NotNil(t, upstream.lastReq)
					require.Equal(t, index*1024, result.Usage.CacheReadInputTokens, "the second request must reuse the simulated upstream cache through SSE and buffered JSON")
					key := gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String()
					require.NotEmpty(t, key)
					require.Equal(t, key, upstream.lastReq.Header.Get("session-id"))
					require.Equal(t, key, upstream.lastReq.Header.Get("thread-id"))
					if index == 0 {
						cacheKey = key
					} else {
						require.Equal(t, cacheKey, key)
					}
				}
			})
		}
	}
}

func TestCodexSessionIdentityLocalFallbackAndRecovery(t *testing.T) {
	shared := &stubOpenAIWSSharedCache{getErr: errors.New("Redis unavailable")}
	store := newCodexSimulationStateStore(shared, time.Minute)
	first := uuid.Must(uuid.NewV7()).String()
	second := uuid.Must(uuid.NewV7()).String()
	key := codexSimulationSessionStateKey("root", "principal")
	require.Equal(t, first, store.sessionIDWithTTL(context.Background(), key, first, time.Minute))
	require.Equal(t, first, store.sessionIDWithTTL(context.Background(), key, second, time.Minute))
	shared.getErr = nil
	require.Equal(t, first, store.sessionIDWithTTL(context.Background(), key, second, time.Minute), "recovery must seed Redis with the warm local session")
	otherReplica := newCodexSimulationStateStore(shared, time.Minute)
	require.Equal(t, first, otherReplica.sessionIDWithTTL(context.Background(), key, second, time.Minute))
	// A shared winner remains authoritative after this replica recovers.
	otherKey := codexSimulationSessionStateKey("other-root", "principal")
	store.setLocalWithTTL(otherKey, second, time.Minute)
	require.NoError(t, shared.SetOpenAIWSState(context.Background(), otherKey, first, time.Minute))
	require.Equal(t, first, store.sessionIDWithTTL(context.Background(), otherKey, second, time.Minute))
}

func TestCodexSessionIdentityLocalFallbackSerializesConcurrentCreation(t *testing.T) {
	store := newCodexSimulationStateStore(nil, time.Minute)
	const workers = 32
	results := make(chan string, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			results <- store.sessionIDWithTTL(context.Background(), "session", uuid.Must(uuid.NewV7()).String(), time.Minute)
		})
	}
	wg.Wait()
	close(results)
	var first string
	for result := range results {
		if first == "" {
			first = result
		}
		require.Equal(t, first, result)
	}
	store.mu.Lock()
	binding := store.entries["session"]
	binding.expiresAt = time.Now().Add(-time.Second)
	store.entries["session"] = binding
	store.mu.Unlock()
	next := uuid.Must(uuid.NewV7()).String()
	require.Equal(t, next, store.sessionIDWithTTL(context.Background(), "session", next, time.Minute), "expired sessions may start a fresh cache partition")
}

func (c *stubOpenAIWSSharedCache) GetOrCreateCodexSessionID(_ context.Context, key, candidate string, ttl time.Duration) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.getErr != nil {
		return "", c.getErr
	}
	if c.setErr != nil {
		return "", c.setErr
	}
	if c.states == nil {
		c.states = make(map[string]openAIWSSharedStateValue)
	}
	if value, found := c.states[key]; found && time.Now().Before(value.expiresAt) {
		candidate = value.value
	}
	c.states[key] = openAIWSSharedStateValue{value: candidate, expiresAt: time.Now().Add(ttl)}
	return candidate, nil
}

func TestCodexFullSimulationCacheIdentityKeepsNamespacesIsolated(t *testing.T) {
	resolve := func(apiKeyID, groupID int64, project, conversation, principal, secret string) *codexFingerprintIDs {
		t.Helper()
		svc := newCodexSimulationTestService(true, codexContinuationOff)
		svc.cfg.Gateway.CodexSimulation.IdentitySecret = secret
		account := openAIFingerprintAccount(602, map[string]any{codexFingerprintModeExtraKey: "full"})
		account.Credentials = map[string]any{"chatgpt_account_id": principal}
		c := newCodexSimulationTestContext("/v1/responses")
		c.Request.Header.Set("thread-id", conversation)
		c.Request.Header.Set(CodexProjectIDHeader, project)
		body := []byte(`{}`)
		svc.PrepareCodexSimulationRequest(c, apiKeyID, &groupID, body)
		_, err := svc.PrepareCodexSimulationAttempt(context.Background(), c, account, body)
		require.NoError(t, err)
		return resolveCodexFingerprintIDsFromGinContext(account, c)
	}
	first := resolve(1, 2, "project", "conversation", "principal", codexSimulationTestSecret)
	for _, other := range []*codexFingerprintIDs{
		resolve(2, 2, "project", "conversation", "principal", codexSimulationTestSecret),
		resolve(1, 3, "project", "conversation", "principal", codexSimulationTestSecret),
		resolve(1, 2, "other-project", "conversation", "principal", codexSimulationTestSecret),
		resolve(1, 2, "project", "other-conversation", "principal", codexSimulationTestSecret),
		resolve(1, 2, "project", "conversation", "other-principal", codexSimulationTestSecret),
		resolve(1, 2, "project", "conversation", "principal", codexSimulationTestSecret+"-rotated"),
	} {
		require.NotEqual(t, first.sessionID, other.sessionID)
		require.NotEqual(t, first.promptCacheKey, other.promptCacheKey)
	}
	requestLocal := resolve(1, 2, "project", "", "principal", codexSimulationTestSecret)
	otherRequest := resolve(1, 2, "project", "", "principal", codexSimulationTestSecret)
	require.NotEqual(t, requestLocal.sessionID, otherRequest.sessionID, "requests without a conversation signal must not be merged")
}

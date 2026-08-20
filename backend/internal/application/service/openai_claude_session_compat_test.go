package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const claudeCodeCompatMetadataUserID = `{"device_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","account_uuid":"","session_id":"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"}`

func newOpenAIClaudeCompatSessionContext(t *testing.T) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set("api_key", &APIKey{ID: 41})
	return c
}

func claudeCodeCompatResponsesBody(firstUser string) []byte {
	return []byte(`{"model":"gpt-5.6-luna","metadata":{"user_id":` + strconv.Quote(claudeCodeCompatMetadataUserID) + `},"instructions":"turn-specific instructions","input":[{"role":"user","content":"` + firstUser + `"}]}`)
}

func TestGenerateSessionHashForRequestUsesClaudeMetadataBeforeContentFallback(t *testing.T) {
	svc := &OpenAIGatewayService{}
	groupID := int64(7)
	first := newOpenAIClaudeCompatSessionContext(t)
	second := newOpenAIClaudeCompatSessionContext(t)

	hash1 := svc.GenerateSessionHashForRequest(first, &groupID, claudeCodeCompatResponsesBody("first turn"))
	hash2 := svc.GenerateSessionHashForRequest(second, &groupID, claudeCodeCompatResponsesBody("different turn"))

	require.NotEmpty(t, hash1)
	require.Equal(t, hash1, hash2, "Claude metadata session must remain stable as the conversation body evolves")
	require.False(t, openAISessionHashMetadataFromContext(first.Request.Context()).contentDerived)
	require.False(t, openAISessionHashMetadataFromContext(second.Request.Context()).contentDerived)
}

func TestGenerateSessionHashForRequestAcceptsClaudeCodeSessionHeader(t *testing.T) {
	svc := &OpenAIGatewayService{}
	groupID := int64(7)
	first := newOpenAIClaudeCompatSessionContext(t)
	second := newOpenAIClaudeCompatSessionContext(t)
	first.Request.Header.Set(claudeCodeSessionHeader, "claude-header-session")
	second.Request.Header.Set(claudeCodeSessionHeader, "claude-header-session")

	hash1 := svc.GenerateSessionHashForRequest(first, &groupID, []byte(`{"model":"gpt-5.6-luna","input":"one"}`))
	hash2 := svc.GenerateSessionHashForRequest(second, &groupID, []byte(`{"model":"gpt-5.6-luna","input":"two"}`))

	require.NotEmpty(t, hash1)
	require.Equal(t, hash1, hash2)
	require.Equal(t, "claude-header-session", svc.ExtractSessionID(first, nil))
}

func TestDeriveClaudeCodeOpenAIPromptCacheKeyIsStableAndBridgeIndependent(t *testing.T) {
	body := claudeCodeCompatResponsesBody("hello")
	key := deriveClaudeCodeOpenAIPromptCacheKey(body)
	require.NotEmpty(t, key)
	require.Equal(t, key, deriveClaudeCodeOpenAIPromptCacheKey(claudeCodeCompatResponsesBody("changed")))
	require.NotContains(t, key, "anthropic-metadata-")
	require.False(t, isOpenAICompatMessagesBridgePromptCacheKey(key))
}

func TestResolveCodexOutboundSessionIDsUsesClaudeCodeSessionHeader(t *testing.T) {
	c := newOpenAIClaudeCompatSessionContext(t)
	c.Request.Header.Set(claudeCodeSessionHeader, "claude-header-session")
	account := &Account{
		ID:          12,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"chatgpt_account_id": "chatgpt-account"},
	}

	ids := resolveCodexOutboundSessionIDs(c, account, []byte(`{"model":"gpt-5.6-luna","input":"hello"}`), "")
	require.NotNil(t, ids)
	require.NotEmpty(t, ids.sessionID)
	require.NotEmpty(t, ids.threadID)
	require.NotEqual(t, "claude-header-session", ids.sessionID)
}

func TestForwardOAuthClaudeMetadataMaterializesStablePromptCacheKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := claudeCodeCompatResponsesBody("hello")
	upstream := &httpUpstreamRecorder{resp: openAICompatSSECompletedResponse("resp_claude_metadata", "gpt-5.6-luna")}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg: &config.Config{Security: config.SecurityConfig{
			URLAllowlist: config.URLAllowlistConfig{Enabled: false},
		}},
	}
	account := &Account{
		ID:          12,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"},
	}
	c := newOpenAIClaudeCompatSessionContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	key := deriveClaudeCodeOpenAIPromptCacheKey(body)
	require.Equal(t, key, gjson.GetBytes(upstream.lastBody, "prompt_cache_key").String())
	require.Equal(t, isolateOpenAISessionID(41, key), upstream.lastReq.Header.Get("session_id"))
}

func TestOpenAIRequestScopedFailoverKeepsOriginalStickyOwner(t *testing.T) {
	for _, tc := range []struct {
		name          string
		loadBatch     bool
		initialSticky bool
	}{
		{name: "load_batch_with_existing_sticky", loadBatch: true, initialSticky: true},
		{name: "load_batch_disabled_with_existing_sticky", loadBatch: false, initialSticky: true},
		{name: "first_request_binds_fallback", loadBatch: true, initialSticky: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			groupID := int64(77)
			accounts := []Account{
				{ID: 901, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID}, Credentials: map[string]any{"api_key": "old"}},
				{ID: 902, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, GroupIDs: []int64{groupID}, Credentials: map[string]any{"api_key": "new"}},
			}
			bindings := map[string]int64{}
			if tc.initialSticky {
				bindings["openai:claude-session"] = 901
			}
			cache := &schedulerTestGatewayCache{sessionBindings: bindings}
			svc := &OpenAIGatewayService{
				accountRepo: schedulerTestOpenAIAccountRepo{accounts: accounts},
				cache:       cache,
				cfg:         &config.Config{Gateway: config.GatewayConfig{Scheduling: config.GatewaySchedulingConfig{LoadBatchEnabled: tc.loadBatch}}},
				concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{
					loadMap: map[int64]*AccountLoadInfo{902: {AccountID: 902, LoadRate: 0}},
				}),
			}
			ctx := svc.PreserveOpenAIStickyBindingForFailover(context.Background(), &groupID, "claude-session", 901)
			selection, err := svc.selectAccountWithLoadAwareness(
				ctx, &groupID, PlatformOpenAI, "claude-session", "gpt-5.6-luna",
				map[int64]struct{}{901: {}}, false, "", true, false,
			)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, int64(902), selection.Account.ID)
			if tc.initialSticky {
				require.Equal(t, int64(901), cache.sessionBindings["openai:claude-session"], "request-scoped failover must not permanently move the conversation cache owner")
			} else {
				require.Equal(t, int64(902), cache.sessionBindings["openai:claude-session"], "a first request must bind its successful fallback account")
			}
			if selection.ReleaseFunc != nil {
				selection.ReleaseFunc()
			}
		})
	}
}

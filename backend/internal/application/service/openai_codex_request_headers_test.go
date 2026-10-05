package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestApplyOpenAICodexSemanticRequestHeaders(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		wantSubagent string
		wantGuardian string
		wantMemgen   string
	}{
		{
			name:         "guardian reviewer",
			body:         `{"client_metadata":{"x-openai-subagent":"guardian","x-codex-turn-metadata":"{\"thread_source\":\"guardian_review\",\"turn_trigger\":\"approval\"}"}}`,
			wantSubagent: "guardian",
			wantGuardian: "reviewer",
		},
		{
			name:         "guardian classifier",
			body:         `{"client_metadata":{"x-openai-subagent":"guardian","x-codex-turn-metadata":"{\"thread_source\":\"guardian_classifier\",\"turn_trigger\":\"guardian_classifier\"}"}}`,
			wantSubagent: "guardian",
			wantGuardian: "classifier",
		},
		{
			name:         "memory consolidation",
			body:         `{"client_metadata":{"x-openai-subagent":"memory_consolidation","x-codex-turn-metadata":"{\"request_kind\":\"memory\",\"thread_source\":\"memory_consolidation\",\"turn_trigger\":\"memory_consolidation\"}"}}`,
			wantSubagent: "memory_consolidation",
			wantMemgen:   "true",
		},
		{
			name:         "ordinary compact subagent",
			body:         `{"client_metadata":{"x-openai-subagent":"compact"}}`,
			wantSubagent: "compact",
		},
		{
			name:         "mismatched spoof is cleared",
			body:         `{"client_metadata":{"x-openai-subagent":"guardian","x-codex-turn-metadata":"{\"thread_source\":\"ordinary\",\"turn_trigger\":\"guardian_classifier\"}"}}`,
			wantSubagent: "guardian",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			headers := http.Header{
				"X-Codex-Guardian":        []string{"forged"},
				"X-Openai-Memgen-Request": []string{"forged"},
				"X-Openai-Subagent":       []string{"forged"},
			}
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			if tt.wantGuardian != "" {
				c.Request.Header.Set(openAICodexGuardianHeader, tt.wantGuardian)
			}
			if tt.wantMemgen != "" {
				c.Request.Header.Set(openAIMemgenRequestHeader, tt.wantMemgen)
			}
			applyOpenAICodexSemanticRequestHeaders(headers, c, &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, []byte(tt.body))
			require.Equal(t, tt.wantSubagent, headers.Get("x-openai-subagent"))
			require.Equal(t, tt.wantGuardian, headers.Get(openAICodexGuardianHeader))
			require.Equal(t, tt.wantMemgen, headers.Get(openAIMemgenRequestHeader))
		})
	}
}

func TestOpenAICodexRawTurnMetadataHeaderIsBoundedAndAccountScoped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("session-id", "client-session")

	headers := make(http.Header)
	headers.Set(openAIWSTurnMetadataHeader, `{"turn_id":"forged-turn","window_id":"forged-window","sandbox":"danger-full-access","guardian_credits_requested":true,"turn_trigger":"guardian_classifier","subagent_kind":"guardian","workspaces":[{"path":"/Users/alice/private/repo","associated_remote_urls":["https://example.com/org/repo.git?token=secret#fragment"],"access_token":"drop-me"}],"custom":{"nested":true}}`)
	account := &Account{ID: 92, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	applyOpenAICodexSemanticRequestHeaders(headers, c, account, nil)
	metadata := headers.Get(openAIWSTurnMetadataHeader)
	require.True(t, gjson.Valid(metadata))
	require.Empty(t, gjson.Get(metadata, "turn_id").String())
	require.Empty(t, gjson.Get(metadata, "window_id").String())
	require.Empty(t, gjson.Get(metadata, "sandbox").String())
	require.False(t, gjson.Get(metadata, "guardian_credits_requested").Exists())
	require.Equal(t, "guardian_classifier", gjson.Get(metadata, "turn_trigger").String())
	require.NotEqual(t, "client-session", gjson.Get(metadata, "session_id").String())
	require.NotEqual(t, "forged-window", gjson.Get(metadata, "window_id").String())
	require.Equal(t, resolveConvergedInstallationID(account), gjson.Get(metadata, "installation_id").String())
	require.Equal(t, "workspace:redacted", gjson.Get(metadata, "workspaces.0.path").String())
	require.Equal(t, "https://example.com/org/repo.git", gjson.Get(metadata, "workspaces.0.associated_remote_urls.0").String())
	require.False(t, gjson.Get(metadata, "workspaces.0.access_token").Exists())
	require.False(t, strings.Contains(metadata, "/Users/alice"))
	require.False(t, strings.Contains(metadata, "token=secret"))

	invalidHeaders := make(http.Header)
	invalidHeaders.Set(openAIWSTurnMetadataHeader, "not-json")
	applyOpenAICodexSemanticRequestHeaders(invalidHeaders, c, account, nil)
	require.Empty(t, invalidHeaders.Get(openAIWSTurnMetadataHeader))

	bodyHeaders := make(http.Header)
	bodyHeaders.Set(openAIWSTurnMetadataHeader, "not-json")
	body := []byte(`{"client_metadata":{"x-codex-turn-metadata":"{\"thread_source\":\"body\"}"}}`)
	applyOpenAICodexSemanticRequestHeaders(bodyHeaders, c, account, body)
	require.Equal(t, "body", gjson.Get(bodyHeaders.Get(openAIWSTurnMetadataHeader), "thread_source").String())
}

func TestOpenAIRequestBuildReconstructsIsolatedParentAndGuardianHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("x-codex-parent-thread-id", "forged-direct-parent")
	c.Request.Header.Set(openAICodexGuardianHeader, "reviewer")
	c.Request.Header.Set(openAIMemgenRequestHeader, "true")

	body := []byte(`{
		"model":"gpt-5.6-sol",
		"client_metadata":{
			"session_id":"client-session",
			"thread_id":"client-thread",
			"x-codex-parent-thread-id":"client-parent",
			"x-openai-subagent":"guardian",
			"x-codex-turn-metadata":"{\"parent_thread_id\":\"client-parent\",\"thread_source\":\"guardian_review\"}"
		}
	}`)
	account := &Account{
		ID:       81,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"chatgpt_account_id": "principal-81",
		},
	}

	req, err := (&OpenAIGatewayService{}).buildUpstreamRequest(
		context.Background(), c, account, body, "token", false, "", true,
	)
	require.NoError(t, err)
	parent := req.Header.Get("x-codex-parent-thread-id")
	require.NotEmpty(t, parent)
	require.NotEqual(t, "client-parent", parent)
	require.NotEqual(t, "forged-direct-parent", parent)
	require.Equal(t, "guardian", req.Header.Get("x-openai-subagent"))
	require.Equal(t, "reviewer", req.Header.Get(openAICodexGuardianHeader))
	require.Empty(t, req.Header.Get(openAIMemgenRequestHeader))

	rebuiltBody, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	if req.Header.Get("Content-Encoding") == "zstd" {
		rebuiltBody = decodeZstdBody(t, rebuiltBody)
	}
	require.Equal(t, parent, gjson.GetBytes(rebuiltBody, "client_metadata.x-codex-parent-thread-id").String())
	turnMetadata := gjson.GetBytes(rebuiltBody, "client_metadata.x-codex-turn-metadata").String()
	require.Equal(t, parent, gjson.Get(turnMetadata, "parent_thread_id").String())
	require.NotContains(t, string(rebuiltBody), "forged-direct-parent")
}

func TestOpenAIRequestBuildAddsMemgenOnlyForConsistentMemoryMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set(openAIMemgenRequestHeader, "true")
	body := []byte(`{
		"model":"gpt-5.6-sol",
		"client_metadata":{
			"x-openai-subagent":"memory_consolidation",
			"x-codex-turn-metadata":"{\"request_kind\":\"memory\",\"thread_source\":\"memory_consolidation\",\"turn_trigger\":\"memory_consolidation\"}"
		}
	}`)
	account := &Account{ID: 82, Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	req, err := (&OpenAIGatewayService{}).buildUpstreamRequest(
		context.Background(), c, account, body, "token", false, "", true,
	)
	require.NoError(t, err)
	require.Equal(t, "memory_consolidation", req.Header.Get("x-openai-subagent"))
	require.Equal(t, "true", req.Header.Get(openAIMemgenRequestHeader))
	require.Empty(t, req.Header.Get(openAICodexGuardianHeader))
}

func TestOpenAIWSCompatibilityIncludesRouteAuthAndSemanticHeaders(t *testing.T) {
	baseHeaders := http.Header{
		"Authorization":                         []string{"Bearer token-a"},
		"X-Openai-Account-Routing-Override":     []string{"us"},
		"X-Openai-Subagent":                     []string{"guardian"},
		"X-Codex-Guardian":                      []string{"reviewer"},
		"X-Responsesapi-Include-Timing-Metrics": []string{"true"},
	}
	base := openAIWSAcquireRequest{WSURL: "wss://workspace.example.com/backend-api/codex/responses", Headers: baseHeaders}
	baseKey := openAIWSAcquireCompatibility(base)

	mutations := []struct {
		name   string
		mutate func(*openAIWSAcquireRequest)
	}{
		{name: "target URL", mutate: func(req *openAIWSAcquireRequest) { req.WSURL = "wss://other.example.com/backend-api/codex/responses" }},
		{name: "authorization", mutate: func(req *openAIWSAcquireRequest) { req.Headers.Set("Authorization", "Bearer token-b") }},
		{name: "routing override", mutate: func(req *openAIWSAcquireRequest) { req.Headers.Set(openAIAccountRoutingOverrideHeader, "us_cr") }},
		{name: "responses lite", mutate: func(req *openAIWSAcquireRequest) { req.Headers.Set(responsesLiteHeaderKey, "true") }},
		{name: "websocket beta", mutate: func(req *openAIWSAcquireRequest) { req.Headers.Set("OpenAI-Beta", "responses_websockets=2026-02-04") }},
		{name: "subagent", mutate: func(req *openAIWSAcquireRequest) { req.Headers.Set("x-openai-subagent", "compact") }},
		{name: "guardian", mutate: func(req *openAIWSAcquireRequest) { req.Headers.Set(openAICodexGuardianHeader, "classifier") }},
		{name: "memgen", mutate: func(req *openAIWSAcquireRequest) { req.Headers.Set(openAIMemgenRequestHeader, "true") }},
		{name: "timing metrics", mutate: func(req *openAIWSAcquireRequest) { req.Headers.Del("x-responsesapi-include-timing-metrics") }},
	}

	for _, tt := range mutations {
		t.Run(tt.name, func(t *testing.T) {
			req := openAIWSAcquireRequest{WSURL: base.WSURL, Headers: base.Headers.Clone()}
			tt.mutate(&req)
			require.NotEqual(t, baseKey, openAIWSAcquireCompatibility(req))
		})
	}
}

func TestOpenAIWSHeadersReconstructResponsesLiteFromHandshakeOrPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/responses", nil)
	c.Request.Header.Set(responsesLiteHeader, "true")

	service := &OpenAIGatewayService{}
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	headers, _, err := service.buildOpenAIWSHeaders(
		context.Background(),
		c,
		account,
		"token",
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		true,
		"",
		"",
		"",
		"gpt-5.6-sol",
		"",
	)
	require.NoError(t, err)
	require.Equal(t, "true", headers.Get(responsesLiteHeader))

	// A client may carry the signal only in the WS response.create metadata.
	c.Request.Header.Del(responsesLiteHeader)
	stageCodexOutboundSessionBody(c, []byte(`{"client_metadata":{"ws_request_header_x_openai_internal_codex_responses_lite":"true"}}`))
	headers, _, err = service.buildOpenAIWSHeaders(
		context.Background(),
		c,
		account,
		"token",
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		true,
		"",
		"",
		"",
		"gpt-5.6-sol",
		"",
	)
	require.NoError(t, err)
	require.Equal(t, "true", headers.Get(responsesLiteHeader))
}

func TestOpenAICodexLatestMetadataKeysRemainTypedInFullProjection(t *testing.T) {
	metadata := map[string]any{
		"x-codex-ws-stream-request-start-ms": int64(123),
		"guardian_credits_requested":         true,
		"parent_response_id":                 "resp-parent",
		"x-codex-turn-metadata":              `{"forked_from_ordinal_exclusive":7,"turn_trigger":"guardian_classifier","history_ingest_requested":true,"analytics_enabled":false}`,
	}
	ids := &codexFingerprintIDs{
		mode:           codexFingerprintFull,
		fullSimulation: true,
		installationID: "11111111-1111-4111-8111-111111111111",
		sessionID:      "22222222-2222-7222-8222-222222222222",
		threadID:       "33333333-3333-7333-8333-333333333333",
		turnID:         "44444444-4444-7444-8444-444444444444",
		windowID:       "33333333-3333-7333-8333-333333333333:1",
	}

	require.True(t, applyCodexFingerprintClientMetadataMap(metadata, ids))
	require.Equal(t, "123", metadata["x-codex-ws-stream-request-start-ms"])
	require.Equal(t, "true", metadata["guardian_credits_requested"])
	require.Equal(t, "resp-parent", metadata["parent_response_id"])
	turnMetadata, ok := metadata["x-codex-turn-metadata"].(string)
	require.True(t, ok)
	require.Equal(t, int64(7), gjson.Get(turnMetadata, "forked_from_ordinal_exclusive").Int())
	require.True(t, gjson.Get(turnMetadata, "history_ingest_requested").Bool())
	require.Equal(t, gjson.False, gjson.Get(turnMetadata, "analytics_enabled").Type)
	require.Equal(t, "guardian_classifier", gjson.Get(turnMetadata, "turn_trigger").String())
	require.False(t, strings.Contains(turnMetadata, `"history_ingest_requested":"true"`))
}

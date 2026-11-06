package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSIWCForwardPreservesDownstreamStreamingContract(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("passthrough=%v/stream=%v", passthrough, stream), func(t *testing.T) {
				body := []byte(fmt.Sprintf(`{"model":"gpt-5.4","instructions":"fixture","input":"hello","stream":%v}`, stream))
				recorder := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
				sse := "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-siwc\",\"object\":\"response\",\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}],\"usage\":{\"input_tokens\":5,\"output_tokens\":16,\"total_tokens\":21}}}\n\n"
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(sse))}}
				cfg := &config.Config{}
				svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, responseHeaderFilter: compileResponseHeaderFilter(cfg)}
				account := &Account{ID: 981, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
					Credentials: map[string]any{"auth_mode": "siwc", "access_token": "fixture-token"},
					Extra:       map[string]any{"openai_passthrough": passthrough}, Status: StatusActive, Schedulable: true}
				result, err := svc.Forward(context.Background(), c, account, body)
				require.NoError(t, err)
				require.Equal(t, stream, result.Stream)
				require.Equal(t, "https://api.openai.com/v1/responses", upstream.lastReq.URL.String())
				require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
				require.Equal(t, 5, result.Usage.InputTokens)
				require.Equal(t, 16, result.Usage.OutputTokens)
				if stream {
					require.Contains(t, recorder.Header().Get("Content-Type"), "text/event-stream")
					require.Contains(t, recorder.Body.String(), "response.completed")
				} else {
					require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
					require.True(t, gjson.ValidBytes(recorder.Body.Bytes()))
					require.Equal(t, "completed", gjson.GetBytes(recorder.Body.Bytes(), "status").String())
				}
			})
		}
	}
}

func TestSIWCGatewayHTTPPassthroughAndWebSocket(t *testing.T) {
	svc := openAIOAuthCustomRelayTestService()
	account := &Account{ID: 981, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "siwc-at", "auth_mode": "siwc", "chatgpt_account_id": "acct-siwc"}}
	for _, stream := range []bool{false, true} {
		for _, passthrough := range []bool{false, true} {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set("originator", "codex_cli_rs")
			c.Request.Header.Set("x-codex-turn-state", "must-not-leak")
			body := []byte(`{"model":"gpt-6-sol","input":[{"role":"user","content":"hello","integer":9007199254740993}],"temperature":0.7,"store":true,"stream":false}`)
			var req *http.Request
			var err error
			if passthrough {
				req, err = svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), c, account, body, "siwc-at")
			} else {
				req, err = svc.buildUpstreamRequest(context.Background(), c, account, body, "siwc-at", stream, "isolated-key", false)
			}
			require.NoError(t, err)
			require.Equal(t, "https://api.openai.com/v1/responses", req.URL.String())
			require.Equal(t, "Bearer siwc-at", req.Header.Get("Authorization"))
			require.Empty(t, req.Header.Get("Content-Encoding"))
			require.Empty(t, req.Header.Get("chatgpt-account-id"))
			require.Empty(t, req.Header.Get("originator"))
			require.Empty(t, req.Header.Get("x-codex-turn-state"))
			require.Contains(t, req.Header.Get("User-Agent"), "Sub2API")
			wireBody, err := io.ReadAll(req.Body)
			require.NoError(t, err)
			require.True(t, gjson.GetBytes(wireBody, "stream").Bool())
			require.False(t, gjson.GetBytes(wireBody, "store").Bool())
			require.False(t, gjson.GetBytes(wireBody, "temperature").Exists())
			require.Equal(t, "9007199254740993", gjson.GetBytes(wireBody, "input.0.integer").Raw)
		}
	}
	wsURL, err := svc.buildOpenAIResponsesWSURL(account)
	require.NoError(t, err)
	require.Equal(t, "wss://api.openai.com/v1/responses", wsURL)
	wsBody := []byte(`{"type":"response.create","model":"gpt-6-sol","stream":true,"background":true,"input":[{"type":"configuration_update","reasoning":{"effort":"high"}},{"role":"user","content":"hi"}]}`)
	normalized, _, err := normalizeOpenAIOAuthReplayHistoryForAccount(wsBody, account, false, false)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(normalized, "stream").Exists())
	require.False(t, gjson.GetBytes(normalized, "background").Exists())
	require.Equal(t, "high", gjson.GetBytes(normalized, "reasoning.effort").String())
	require.Equal(t, int64(1), gjson.GetBytes(normalized, "input.#").Int())
	headers := http.Header{"Originator": {"codex_cli_rs"}, "Chatgpt-Account-Id": {"acct"}, "Session-Id": {"isolated"}, "Openai-Beta": {openAIWSBetaV2Value}}
	applyOpenAICodexSemanticRequestHeaders(headers, nil, account, wsBody)
	require.Empty(t, headers.Get("Originator"))
	require.Empty(t, headers.Get("Chatgpt-Account-Id"))
	require.Equal(t, "isolated", headers.Get("session_id"))
	require.Equal(t, openAIWSBetaV2Value, headers.Get("OpenAI-Beta"))
}

func TestSIWCCompactBodyAndLegacyAccountIsolation(t *testing.T) {
	body := []byte(`{"model":"gpt-6-sol","stream":true,"store":true,"input":"hi"}`)
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"auth_mode": "siwc"}}
	normalized, _, err := normalizeOpenAIOAuthReplayHistoryForAccount(body, account, true, false)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(normalized, "stream").Exists())
	require.False(t, gjson.GetBytes(normalized, "store").Exists())
	account.Credentials = nil
	legacy, _, err := normalizeOpenAIOAuthReplayHistoryForAccount(body, account, false, false)
	require.NoError(t, err)
	require.JSONEq(t, string(body), string(legacy))
	account.Type = AccountTypeAPIKey
	account.Credentials = map[string]any{"auth_mode": "siwc"}
	require.False(t, account.IsOpenAISIWC(), "SIWC is only an OAuth credential protocol")
}

package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestApplyCodexPrewarmContinuationReasoningOverride(t *testing.T) {
	gin.SetMode(gin.TestMode)

	enabledAccount := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{CodexPrewarmContinuationExtraKey: true},
	}
	disabledAccount := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}

	tests := []struct {
		name    string
		account *Account
		header  string
		body    string
		want    string
		changed bool
	}{
		{name: "enabled exact opt in", account: enabledAccount, header: "none", body: `{"reasoning":{"effort":"high","summary":"auto"}}`, want: "none", changed: true},
		{name: "enabled canonicalizes opt in", account: enabledAccount, header: " NONE ", body: `{"reasoning":{"effort":"HIGH"}}`, want: "none", changed: true},
		{name: "enabled adds missing effort", account: enabledAccount, header: "none", body: `{"model":"gpt-5.5"}`, want: "none", changed: true},
		{name: "already none is zero work", account: enabledAccount, header: "none", body: `{"reasoning":{"effort":"none"}}`, want: "none", changed: false},
		{name: "switch off ignores header", account: disabledAccount, header: "none", body: `{"reasoning":{"effort":"high"}}`, want: "high", changed: false},
		{name: "other header value ignored", account: enabledAccount, header: "low", body: `{"reasoning":{"effort":"high"}}`, want: "high", changed: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
			c.Request.Header.Set(codexPrewarmContinuationReasoningHeader, tt.header)

			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(tt.body), &body))
			changed, err := applyCodexPrewarmContinuationReasoningOverride(c, tt.account, body)
			require.NoError(t, err)
			require.Equal(t, tt.changed, changed)
			got := payloadAsJSONBytes(body)
			require.Equal(t, tt.want, gjson.GetBytes(got, "reasoning.effort").String())
			if gjson.Get(tt.body, "reasoning.summary").Exists() {
				require.Equal(t, "auto", gjson.GetBytes(got, "reasoning.summary").String())
			}
		})
	}
}

func TestApplyCodexPrewarmContinuationPayloadPreservesStructuredHistory(t *testing.T) {
	largeArguments := strings.Repeat("a", 4096) + "-arguments-tail"
	largeOutput := strings.Repeat("o", 4096) + "-output-tail"
	input := []any{
		map[string]any{"type": "message", "role": "user", "content": "first"},
		map[string]any{"type": "message", "role": "assistant", "content": "answer"},
		map[string]any{"type": "message", "role": "developer", "content": "policy"},
		map[string]any{"type": "function_call", "call_id": "fc_plan", "name": "update_plan", "arguments": largeArguments},
		map[string]any{"type": "function_call_output", "call_id": "fc_plan", "output": largeOutput},
		map[string]any{"type": "web_search_call", "id": "ws_1", "status": "completed", "action": map[string]any{"type": "search", "query": "sub2api"}},
		map[string]any{"type": "tool_search_call", "call_id": "fc_search", "execution": "client", "arguments": map[string]any{"query": "github"}},
		map[string]any{"type": "tool_search_output", "call_id": "fc_search", "output": map[string]any{"groups": []any{"github"}}},
		map[string]any{"type": "image_generation_call", "id": "ig_1", "status": "completed", "result": largeOutput},
		map[string]any{"type": "message", "role": "user", "content": "last"},
	}
	payload := map[string]any{
		"instructions": "keep top-level instructions",
		"input":        input,
		"generate":     false,
	}

	rewritten := applyCodexPrewarmContinuationPayload(payload, " resp_native ")

	require.Equal(t, 2, rewritten)
	require.Equal(t, "resp_native", payload["previous_response_id"])
	require.Equal(t, "keep top-level instructions", payload["instructions"])
	require.NotContains(t, payload, "generate")
	require.Len(t, input, 10)
	requireInputMap := func(index int) map[string]any {
		item, ok := input[index].(map[string]any)
		require.True(t, ok, "input[%d] must remain an object", index)
		return item
	}
	require.Equal(t, "developer", requireInputMap(0)["role"])
	require.Equal(t, "assistant", requireInputMap(1)["role"])
	require.Equal(t, "developer", requireInputMap(2)["role"])
	require.Equal(t, "developer", requireInputMap(9)["role"])
	require.Equal(t, "function_call", requireInputMap(3)["type"])
	require.Equal(t, largeArguments, requireInputMap(3)["arguments"])
	require.Equal(t, "function_call_output", requireInputMap(4)["type"])
	require.Equal(t, largeOutput, requireInputMap(4)["output"])
	require.Equal(t, "web_search_call", requireInputMap(5)["type"])
	require.Equal(t, "tool_search_call", requireInputMap(6)["type"])
	require.Equal(t, "tool_search_output", requireInputMap(7)["type"])
	require.Equal(t, "image_generation_call", requireInputMap(8)["type"])
	require.Equal(t, largeOutput, requireInputMap(8)["result"])
}

func TestApplyCodexPrewarmContinuationPayloadKeepsImageMessagesAsUser(t *testing.T) {
	input := []any{
		map[string]any{"type": "message", "role": "user", "content": "text only"},
		map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_image", "image_url": "https://example.com/image.png"},
			},
		},
		map[string]any{
			"type": "message",
			"role": "user",
			"content": []any{
				map[string]any{"type": "input_text", "text": "inspect"},
				map[string]any{"type": "input_image", "image_url": "data:image/png;base64,AA=="},
			},
		},
	}
	payload := map[string]any{"input": input, "generate": false}

	rewritten := applyCodexPrewarmContinuationPayload(payload, "resp_prewarm")

	require.Equal(t, 1, rewritten)
	requireInputMap := func(index int) map[string]any {
		item, ok := input[index].(map[string]any)
		require.True(t, ok, "input[%d] must remain an object", index)
		return item
	}
	require.Equal(t, "developer", requireInputMap(0)["role"])
	require.Equal(t, "user", requireInputMap(1)["role"])
	require.Equal(t, "user", requireInputMap(2)["role"])
	require.Equal(t, "resp_prewarm", payload["previous_response_id"])
	require.NotContains(t, payload, "generate")
}

func TestHasCodexPrewarmBusinessContinuation(t *testing.T) {
	selfContained := map[string]any{"input": []any{
		map[string]any{"type": "function_call", "call_id": "fc_1"},
		map[string]any{"type": "function_call_output", "call_id": "fc_1", "output": "ok"},
		map[string]any{"type": "tool_search_call", "call_id": "fc_2"},
		map[string]any{"type": "tool_search_output", "call_id": "fc_2", "output": "ok"},
		map[string]any{"type": "web_search_call", "id": "ws_1", "status": "completed"},
	}}
	require.False(t, hasCodexPrewarmBusinessContinuation(selfContained))

	for name, body := range map[string]map[string]any{
		"previous response":      {"previous_response_id": "resp_1", "input": []any{}},
		"item reference":         {"input": []any{map[string]any{"type": "item_reference", "id": "fc_1"}}},
		"orphan output":          {"input": []any{map[string]any{"type": "function_call_output", "call_id": "fc_missing", "output": "ok"}}},
		"unpaired call":          {"input": []any{map[string]any{"type": "function_call", "call_id": "fc_pending", "name": "shell"}}},
		"missing output call id": {"input": []any{map[string]any{"type": "function_call_output", "output": "ok"}}},
	} {
		t.Run(name, func(t *testing.T) {
			require.True(t, hasCodexPrewarmBusinessContinuation(body))
		})
	}
}

func TestOpenAIGatewayServiceForwardWSv2CodexPrewarmContinuationStream(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.98.0")
	c.Request.Header.Set(codexPrewarmContinuationReasoningHeader, "none")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1

	captureConn := &openAIWSCaptureConn{events: [][]byte{
		[]byte(`{"type":"response.completed","response":{"id":"resp_stream_prewarm","model":"gpt-5.5","usage":{"input_tokens":0,"output_tokens":0}}}`),
		[]byte(`{"type":"response.output_text.delta","response_id":"resp_stream_business","delta":"ok"}`),
		[]byte(`{"type":"response.completed","response":{"id":"resp_stream_business","model":"gpt-5.5","usage":{"input_tokens":2,"output_tokens":1}}}`),
	}}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: captureConn})
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}
	account := &Account{
		ID:          591,
		Name:        "openai-codex-prewarm-stream",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token-stream"},
		Extra:       map[string]any{CodexPrewarmContinuationExtraKey: true},
	}

	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.5","stream":true,"instructions":"keep","input":[{"type":"message","role":"developer","content":[{"type":"input_text","text":"skill policy"},{"type":"input_image","image_url":"https://example.com/reference.png"}]},{"type":"message","role":"assistant","content":"answer"},{"type":"message","role":"user","content":"last"}]}`))
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "resp_stream_business", result.RequestID)
	require.True(t, result.OpenAIWSMode)
	require.Len(t, captureConn.writes, 2)
	prewarm := requestToJSONString(captureConn.writes[0])
	business := requestToJSONString(captureConn.writes[1])
	require.True(t, gjson.Get(prewarm, "generate").Exists())
	require.False(t, gjson.Get(prewarm, "generate").Bool())
	require.Empty(t, gjson.Get(prewarm, "input").Array())
	require.Equal(t, "resp_stream_prewarm", gjson.Get(business, "previous_response_id").String())
	require.Equal(t, "none", gjson.Get(business, "reasoning.effort").String())
	require.Equal(t, "developer", gjson.Get(business, "input.0.role").String())
	require.Equal(t, "skill policy", gjson.Get(business, "input.0.content.0.text").String())
	require.Equal(t, "user", gjson.Get(business, "input.1.role").String())
	require.Equal(t, "input_image", gjson.Get(business, "input.1.content.0.type").String())
	require.Equal(t, "https://example.com/reference.png", gjson.Get(business, "input.1.content.0.image_url").String())
	require.Equal(t, "assistant", gjson.Get(business, "input.2.role").String())
	require.Equal(t, "developer", gjson.Get(business, "input.3.role").String())
	require.Contains(t, rec.Header().Get("Content-Type"), "text/event-stream")
	require.Contains(t, rec.Body.String(), "response.completed")
}

var benchmarkCodexPrewarmRolesRewritten int

func BenchmarkApplyCodexPrewarmContinuationPayload211Items(b *testing.B) {
	input := make([]any, 0, 211)
	userMessages := make([]map[string]any, 0, 71)
	for i := 0; i < 71; i++ {
		message := map[string]any{"type": "message", "role": "user", "content": "request"}
		userMessages = append(userMessages, message)
		input = append(input, message)
	}
	for i := 0; i < 69; i++ {
		input = append(input, map[string]any{"type": "message", "role": "assistant", "content": "response"})
	}
	for i := 0; i < 2; i++ {
		input = append(input, map[string]any{"type": "message", "role": "developer", "content": "policy"})
	}
	for len(input) < 211 {
		input = append(input, map[string]any{"type": "reasoning", "summary": []any{}})
	}
	payload := map[string]any{
		"instructions":         "benchmark",
		"input":                input,
		"generate":             false,
		"previous_response_id": "resp_benchmark",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		for _, message := range userMessages {
			message["role"] = "user"
		}
		payload["generate"] = false
		benchmarkCodexPrewarmRolesRewritten = applyCodexPrewarmContinuationPayload(payload, "resp_benchmark")
	}
}

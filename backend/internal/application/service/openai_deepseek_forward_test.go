package service

import (
	"bytes"
	"context"
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

func newDeepSeekForwardTest(t *testing.T, body []byte, compact bool, upstreamResponse string) (*gin.Context, *httptest.ResponseRecorder, *OpenAIGatewayService, *httpUpstreamRecorder, *Account) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	if compact {
		MarkOpenAINativeCompactionV2(c)
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(upstreamResponse)),
	}}
	account := deepSeekCompatAccount()
	account.ID = 123
	account.Credentials["api_key"] = "sk-test"
	account.Credentials["base_url"] = "https://api.deepseek.com"
	svc := &OpenAIGatewayService{cfg: &config.Config{Security: config.SecurityConfig{
		URLAllowlist: config.URLAllowlistConfig{Enabled: false},
	}}, httpUpstream: upstream}
	return c, rec, svc, upstream, account
}

func TestDeepSeekResponsesLiteForwardPromotesAdditionalToolsToChat(t *testing.T) {
	body := []byte(`{"model":"deepseek-reasoner","stream":false,"input":[{"type":"message","role":"user","content":"hello"},{"type":"additional_tools","tools":[{"type":"function","name":"exec","parameters":{"type":"object"}}]}]}`)
	c, rec, svc, upstream, account := newDeepSeekForwardTest(t, body, false,
		`{"id":"chatcmpl_lite","object":"chat.completion","model":"deepseek-reasoner","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, upstream.lastReq.URL.Path, "/chat/completions")
	require.Equal(t, "exec", gjson.GetBytes(upstream.lastBody, "tools.0.function.name").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
	require.Equal(t, "ok", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
}

func TestDeepSeekNativeCompactForwardReturnsCompactionItem(t *testing.T) {
	body := []byte(`{"model":"deepseek-reasoner","stream":false,"input":[{"type":"message","role":"user","content":"context"},{"type":"compaction_trigger"}]}`)
	c, rec, svc, upstream, account := newDeepSeekForwardTest(t, body, true,
		`{"id":"chatcmpl_compact","object":"chat.completion","model":"deepseek-reasoner","choices":[{"index":0,"message":{"role":"assistant","content":"summary"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, upstream.lastReq.URL.Path, "/chat/completions")
	require.Equal(t, "compaction", gjson.Get(rec.Body.String(), "output.0.type").String())
	require.Equal(t, "summary", gjson.Get(rec.Body.String(), "output.0.encrypted_content").String())
}

func TestDeepSeekNativeCompactStreamingForwardReturnsCompactionSSE(t *testing.T) {
	body := []byte(`{"model":"deepseek-reasoner","stream":true,"input":[{"type":"message","role":"user","content":"context"},{"type":"compaction_trigger"}]}`)
	c, rec, svc, upstream, account := newDeepSeekForwardTest(t, body, true,
		`{"id":"chatcmpl_compact_stream","object":"chat.completion","model":"deepseek-reasoner","choices":[{"index":0,"message":{"role":"assistant","content":"summary"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, upstream.lastReq.URL.Path, "/chat/completions")
	require.Contains(t, rec.Body.String(), "event: response.completed")
	require.Contains(t, rec.Body.String(), `"type":"compaction"`)
}

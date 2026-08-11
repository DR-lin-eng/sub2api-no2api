package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai_compat"
)

func TestRequiresBillableGrokChatUsage(t *testing.T) {
	tests := []struct {
		name    string
		account *Account
		models  []string
		want    bool
	}{
		{name: "Grok platform", account: &Account{Platform: PlatformGrok}, models: []string{"alias"}, want: true},
		{name: "OpenAI compatible Grok", account: &Account{Platform: PlatformOpenAI}, models: []string{"grok-4.5"}, want: true},
		{name: "mapped Grok", account: &Account{Platform: PlatformOpenAI}, models: []string{"alias", "GROK-4.5"}, want: true},
		{name: "namespaced Grok", account: &Account{Platform: PlatformOpenAI}, models: []string{"x-ai/grok-4.5"}, want: true},
		{name: "ordinary OpenAI model", account: &Account{Platform: PlatformOpenAI}, models: []string{"gpt-5.4"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, requiresBillableGrokChatUsage(tt.account, tt.models...))
		})
	}
}

func TestHasBillableGrokChatUsageRequiresAggregateTokens(t *testing.T) {
	require.False(t, hasBillableGrokChatUsage(OpenAIUsage{}))
	require.True(t, hasBillableGrokChatUsage(OpenAIUsage{InputTokens: 1}))
	require.True(t, hasBillableGrokChatUsage(OpenAIUsage{OutputTokens: 1}))
	require.True(t, hasBillableGrokChatUsage(OpenAIUsage{CacheReadInputTokens: 1}))
}

func TestForwardAsChatCompletionsOpenAICompatibleGrokMissingUsageFailsBeforeWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"grok-4.5","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"rid-openai-compat-grok-no-usage"},
		},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_missing_usage","object":"chat.completion","model":"grok-4.5","choices":[{"index":0,"message":{"role":"assistant","content":"hello"},"finish_reason":"stop"}]}`,
		)),
	}}
	testConfig := &config.Config{}
	testConfig.Security.URLAllowlist.Enabled = false
	testConfig.Security.URLAllowlist.AllowInsecureHTTP = true
	svc := &OpenAIGatewayService{cfg: testConfig, httpUpstream: upstream}
	account := &Account{
		ID:          101,
		Name:        "raw-openai-apikey",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "sk-test",
			"base_url": "http://upstream.example",
		},
	}
	account.Name = "openai-compatible-grok"
	account.Extra = map[string]any{openai_compat.ExtraKeyResponsesSupported: false}

	result, err := svc.ForwardAsChatCompletions(context.Background(), c, account, body, "", "")

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, grokMissingUsageErrorCode, gjson.GetBytes(failoverErr.ResponseBody, "error.code").String())
	require.False(t, c.Writer.Written())
	require.Empty(t, recorder.Body.String())
}

func TestHandleChatBufferedStreamingResponseGrokMissingUsageFailsBeforeWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"rid-grok-buffered"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"status\":\"completed\",\"model\":\"grok-4.5\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}]}}\n\n",
		)),
	}
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 7, Name: "grok", Platform: PlatformGrok, Type: AccountTypeOAuth}

	result, err := svc.handleChatBufferedStreamingResponse(
		resp,
		c,
		account,
		"grok-4.5",
		"grok-4.5",
		"grok-4.5",
		time.Now(),
	)

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.False(t, c.Writer.Written())
}

var benchmarkRequiresBillableGrokChatUsage bool

func BenchmarkRequiresBillableGrokChatUsage(b *testing.B) {
	account := &Account{Platform: PlatformOpenAI}
	for _, model := range []string{"gpt-5.4", "grok-4.5", "x-ai/grok-4.5"} {
		b.Run(model, func(b *testing.B) {
			b.ReportAllocs()
			guarded := false
			for b.Loop() {
				guarded = requiresBillableGrokChatUsage(account, model)
			}
			benchmarkRequiresBillableGrokChatUsage = guarded
		})
	}
}

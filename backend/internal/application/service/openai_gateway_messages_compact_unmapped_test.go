package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestForwardAsAnthropic_ClaudeCodeCompactFallsBackWithoutCompactModelMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := []byte(`{"model":"gpt-5.5","max_tokens":16,"messages":[{"role":"user","content":"active work"},{"role":"user","content":[{"type":"text","text":` + quoteCompactJSON(testClaudeCodeCompactPrompt()) + `}]}],"stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		testOpenAICompatSSEFailedContextResponse("resp_unmapped_too_big", "gpt-5.5", 280_000),
		testOpenAICompatSSECompletedResponse("resp_unmapped_chunk", "gpt-5.5", "chunk summary", 80, 10),
		testOpenAICompatSSECompletedResponse("resp_unmapped_merge", "gpt-5.5", "unmapped final summary", 90, 12),
	}}
	svc := &OpenAIGatewayService{
		httpUpstream: upstream,
		cfg:          &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}},
	}
	account := &Account{
		ID:          1,
		Name:        "openai-oauth",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account"},
	}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, rec.Body.String(), "unmapped final summary")
	require.Len(t, upstream.bodies, 3, "unmapped compact turn must enter chunk/merge fallback")
}

func quoteCompactJSON(value string) string {
	quoted, _ := json.Marshal(value)
	return string(quoted)
}

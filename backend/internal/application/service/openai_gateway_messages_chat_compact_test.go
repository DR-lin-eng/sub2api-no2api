//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/shared/openai_compat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardAsAnthropic_ForceChatCompletionsCompactsClaudeCodePrompt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	compactPrompt := testClaudeCodeCompactPrompt()
	body := []byte(`{"model":"gpt-5.5","max_tokens":16,"messages":[{"role":"user","content":"active work"},{"role":"user","content":[{"type":"text","text":` + quoteJSON(compactPrompt) + `}]}],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	chunkResponse := `{"id":"chatcmpl_chunk","object":"chat.completion","model":"gpt-5.5","choices":[{"index":0,"message":{"role":"assistant","content":"chunk summary"},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":4,"total_tokens":24}}`
	mergeResponse := `{"id":"chatcmpl_merge","object":"chat.completion","model":"gpt-5.5","choices":[{"index":0,"message":{"role":"assistant","content":"final compact summary"},"finish_reason":"stop"}],"usage":{"prompt_tokens":30,"completion_tokens":6,"total_tokens":36}}`
	upstream := &httpUpstreamRecorder{responses: []*http.Response{
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_chunk"}}, Body: io.NopCloser(strings.NewReader(chunkResponse))},
		{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_merge"}}, Body: io.NopCloser(strings.NewReader(mergeResponse))},
	}}
	account := rawChatCompletionsTestAccount()
	account.Extra = map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions)}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.5")
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "final compact summary", gjson.GetBytes(rec.Body.Bytes(), "content.0.text").String())
	require.Equal(t, 2, len(upstream.bodies), "compact chat path must issue chunk and merge requests")
	require.Equal(t, "system", gjson.GetBytes(upstream.bodies[0], "messages.0.role").String())
	require.Contains(t, gjson.GetBytes(upstream.bodies[0], "messages.1.content").String(), "active work")
}

func TestForwardAsAnthropic_ForceChatCompletionsCompactStreamFramesAnthropicEvents(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	compactPrompt := testClaudeCodeCompactPrompt()
	body := []byte(`{"model":"gpt-5.5","max_tokens":16,"messages":[{"role":"user","content":"active work"},{"role":"user","content":[{"type":"text","text":` + quoteJSON(compactPrompt) + `}]}],"stream":true}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	responses := []*http.Response{}
	for _, item := range []struct {
		id   string
		text string
	}{
		{id: "chunk_stream", text: "chunk summary"},
		{id: "merge_stream", text: "stream compact summary"},
	} {
		responses = append(responses, &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_" + item.id}},
			Body:       io.NopCloser(strings.NewReader(`{"id":"chatcmpl_` + item.id + `","choices":[{"message":{"role":"assistant","content":` + quoteJSON(item.text) + `},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)),
		})
	}
	account := rawChatCompletionsTestAccount()
	account.Extra = map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions)}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: &httpUpstreamRecorder{responses: responses}}

	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.5")
	require.NoError(t, err)
	require.Contains(t, rec.Body.String(), "event: message_start")
	require.Contains(t, rec.Body.String(), "stream compact summary")
	require.Contains(t, rec.Body.String(), "event: message_stop")
}

func TestForwardAsAnthropic_ForceChatCompletionsAppliesContextHintBeforeConversion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	toolMessages := make([]string, 0, 6)
	for i := 0; i < 6; i++ {
		id := "toolu_" + string(rune('a'+i))
		toolMessages = append(toolMessages,
			`{"role":"assistant","content":[{"type":"tool_use","id":"`+id+`","name":"Read","input":{}}]}`,
			`{"role":"user","content":[{"type":"tool_result","tool_use_id":"`+id+`","content":"old result `+string(rune('a'+i))+`"}]}`,
		)
	}
	body := []byte(`{"model":"gpt-5.5","max_tokens":16,"context_hint":{"enabled":true},"messages":[` + strings.Join(toolMessages, ",") + `],"stream":false}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_context_hint"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":"chatcmpl_context_hint","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`)),
	}}
	account := rawChatCompletionsTestAccount()
	account.Extra = map[string]any{openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions)}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	_, err := svc.ForwardAsAnthropic(context.Background(), c, account, body, "", "gpt-5.5")
	require.NoError(t, err)
	require.Contains(t, string(upstream.lastBody), contextHintToolResultPlaceholder)
	require.Equal(t, "old result f", gjson.GetBytes(upstream.lastBody, "messages.11.content").String())
}

func quoteJSON(value string) string {
	quoted, _ := json.Marshal(value)
	return string(quoted)
}

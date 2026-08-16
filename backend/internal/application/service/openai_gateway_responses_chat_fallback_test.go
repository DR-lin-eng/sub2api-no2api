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

func TestForwardResponses_ForceChatCompletionsRoutesNonStreamingToChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_resp_chat_json"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_json","object":"chat.completion","model":"gpt-5.4","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5,"prompt_tokens_details":{"cached_tokens":1}}}`,
		)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))
	require.Equal(t, "hello", gjson.GetBytes(upstream.lastBody, "messages.0.content").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
	require.Equal(t, "response", gjson.Get(rec.Body.String(), "object").String())
	require.Equal(t, "ok", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Equal(t, 1, result.Usage.CacheReadInputTokens)
	require.False(t, result.Stream)
}

func TestForwardResponses_PassthroughFlagWithUnsupportedResponsesUsesAccountMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, path := range []string{"/v1/responses", "/v1/responses/compact"} {
		t.Run(path, func(t *testing.T) {
			body := []byte(`{"model":"gpt-5.4-channel","input":"hello","stream":false}`)
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")

			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(
					`{"id":"chatcmpl_mapping","object":"chat.completion","model":"gpt-5.4-account","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`,
				)),
			}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			account := rawChatCompletionsTestAccount()
			account.Credentials["model_mapping"] = map[string]any{"gpt-5.4-channel": "gpt-5.4-account"}
			account.Credentials["compact_model_mapping"] = map[string]any{"gpt-5.4-account": "gpt-5.4-compact"}
			account.Extra = map[string]any{
				"openai_passthrough":                     true,
				openai_compat.ExtraKeyResponsesSupported: false,
			}

			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "http://upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
			require.Equal(t, "gpt-5.4-account", gjson.GetBytes(upstream.lastBody, "model").String())
		})
	}
}

func TestForwardResponses_ForceChatCompletionsRoutesStreamingToChatCompletions(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":"he"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":"llo"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		"",
		`data: {"id":"chatcmpl_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_resp_chat_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://upstream.example/v1/chat/completions", upstream.lastReq.URL.String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "stream_options.include_usage").Bool())
	require.Contains(t, rec.Body.String(), "event: response.output_text.delta")
	require.Contains(t, rec.Body.String(), `"delta":"he"`)
	require.Contains(t, rec.Body.String(), "event: response.completed")
	require.Contains(t, rec.Body.String(), `"input_tokens":4`)
	require.Contains(t, rec.Body.String(), "data: [DONE]")
	require.Equal(t, 4, result.Usage.InputTokens)
	require.Equal(t, 3, result.Usage.OutputTokens)
	require.True(t, result.Stream)
	require.NotNil(t, result.FirstTokenMs)
}

func TestForwardResponses_ForceChatCompletionsCompactionRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)

	compactBody := []byte(`{
		"model":"gpt-5.4",
		"stream":true,
		"tools":[{"type":"function","name":"exec","parameters":{"type":"object"}}],
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"remember this"}]},
			{"type":"compaction_trigger"}
		]
	}`)
	compactRecorder := httptest.NewRecorder()
	compactContext, _ := gin.CreateTestContext(compactRecorder)
	compactContext.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(compactBody))
	compactContext.Request.Header.Set("Content-Type", "application/json")

	compactUpstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_compat_compact"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"chatcmpl_compact","object":"chat.completion","created":123,"model":"gpt-5.4","choices":[{"index":0,"message":{"role":"assistant","content":"summary from upstream"},"finish_reason":"stop"}],"usage":{"prompt_tokens":20,"completion_tokens":5,"total_tokens":25}}`,
		)),
	}}
	compactService := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: compactUpstream}

	compactResult, err := compactService.Forward(context.Background(), compactContext, forceChatResponsesFallbackAccount(), compactBody)
	require.NoError(t, err)
	require.NotNil(t, compactResult)
	require.Equal(t, "http://upstream.example/v1/chat/completions", compactUpstream.lastReq.URL.String())
	require.False(t, gjson.GetBytes(compactUpstream.lastBody, "stream").Bool())
	require.Equal(t, "none", gjson.GetBytes(compactUpstream.lastBody, "tool_choice").String())
	require.Equal(t, "remember this", gjson.GetBytes(compactUpstream.lastBody, "messages.0.content").String())
	require.Equal(t, grokCompactSummaryPrompt, gjson.GetBytes(compactUpstream.lastBody, "messages.1.content").String())
	require.Equal(t, "text/event-stream", compactRecorder.Header().Get("Content-Type"))
	require.Equal(t, 1, strings.Count(compactRecorder.Body.String(), "event: response.output_item.done"))
	require.Contains(t, compactRecorder.Body.String(), "event: response.completed")
	require.True(t, compactResult.Stream)
	require.Equal(t, 20, compactResult.Usage.InputTokens)
	require.Equal(t, 5, compactResult.Usage.OutputTokens)

	compactEvent := ""
	for _, event := range strings.Split(compactRecorder.Body.String(), "\n\n") {
		if strings.HasPrefix(event, "event: response.output_item.done\n") {
			compactEvent = strings.TrimPrefix(strings.TrimPrefix(event, "event: response.output_item.done\n"), "data: ")
			break
		}
	}
	require.NotEmpty(t, compactEvent)
	require.Equal(t, "compaction", gjson.Get(compactEvent, "item.type").String())
	encryptedSummary := gjson.Get(compactEvent, "item.encrypted_content").String()
	require.True(t, strings.HasPrefix(encryptedSummary, compatCompactEnvelopePrefix))
	var compactItem map[string]any
	require.NoError(t, json.Unmarshal([]byte(gjson.Get(compactEvent, "item").Raw), &compactItem))
	require.NotEmpty(t, compactItem["id"])
	require.Equal(t, "completed", compactItem["status"])

	continuationBody, err := json.Marshal(map[string]any{
		"model":  "gpt-5.4",
		"stream": true,
		"input": []any{
			compactItem,
			map[string]any{
				"type":    "message",
				"role":    "user",
				"content": []any{map[string]any{"type": "input_text", "text": "continue"}},
			},
		},
	})
	require.NoError(t, err)

	continuationRecorder := httptest.NewRecorder()
	continuationContext, _ := gin.CreateTestContext(continuationRecorder)
	continuationContext.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(continuationBody))
	continuationContext.Request.Header.Set("Content-Type", "application/json")
	continuationUpstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_continue","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"role":"assistant"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_continue","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":"continued"},"finish_reason":"stop"}]}`,
		"",
		`data: {"id":"chatcmpl_continue","object":"chat.completion.chunk","model":"gpt-5.4","choices":[],"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	continuationUpstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(continuationUpstreamBody)),
	}}
	continuationService := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: continuationUpstream}

	continuationResult, err := continuationService.Forward(context.Background(), continuationContext, forceChatResponsesFallbackAccount(), continuationBody)
	require.NoError(t, err)
	require.NotNil(t, continuationResult)
	require.True(t, gjson.GetBytes(continuationUpstream.lastBody, "stream").Bool())
	require.True(t, gjson.GetBytes(continuationUpstream.lastBody, "stream_options.include_usage").Bool())
	require.Contains(t, gjson.GetBytes(continuationUpstream.lastBody, "messages.0.content").String(), "<conversation_summary>\nsummary from upstream\n</conversation_summary>")
	require.Equal(t, "continue", gjson.GetBytes(continuationUpstream.lastBody, "messages.1.content").String())
	require.Contains(t, continuationRecorder.Body.String(), `"delta":"continued"`)
	require.Contains(t, continuationRecorder.Body.String(), "event: response.completed")
	require.True(t, continuationResult.Stream)
}

func TestForwardResponses_DeepSeekReasoningOnlyStreamProducesVisibleText(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"deepseek-reasoner","input":"hello","stream":true}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_reasoning","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"role":"assistant","content":null,"reasoning_content":""},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_reasoning","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"reasoning_content":"visible fallback"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_reasoning","object":"chat.completion.chunk","model":"deepseek-reasoner","choices":[{"index":0,"delta":{"content":""},"finish_reason":"length"}],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_deepseek_reasoning_responses_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}

	result, err := svc.Forward(context.Background(), c, forceChatResponsesFallbackAccount(), body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Stream)
	require.Contains(t, rec.Body.String(), "event: response.output_text.delta")
	require.Contains(t, rec.Body.String(), `"delta":"visible fallback"`)
	require.Contains(t, rec.Body.String(), `"status":"incomplete"`)
	require.Contains(t, rec.Body.String(), "data: [DONE]")
}

func TestForwardResponses_LiteralThinkingNormalizationIsAccountScoped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	upstreamResponse := `{"id":"chatcmpl_literal","object":"chat.completion","model":"gpt-5.4","choices":[{"index":0,"message":{"role":"assistant","content":"<thinking>plan</thinking>final"},"finish_reason":"stop"}]}`
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(upstreamResponse)),
			}}
			svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
			account := forceChatResponsesFallbackAccount()
			account.Extra[CodexThinkingTagNormalizationExtraKey] = enabled
			_, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			if enabled {
				require.Equal(t, "reasoning", gjson.Get(rec.Body.String(), "output.0.type").String())
				require.Equal(t, "final", gjson.Get(rec.Body.String(), "output.1.content.0.text").String())
			} else {
				require.Equal(t, "<thinking>plan</thinking>final", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
			}
		})
	}
}

func TestForwardResponses_LiteralThinkingNormalizationStreamingWire(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":true}`)
	upstreamBody := strings.Join([]string{
		`data: {"id":"chatcmpl_literal_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"role":"assistant","content":"<think"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_literal_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":"ing>Starting Docker targeted tests****Checking for compile errors and test failures</thinking>normal"},"finish_reason":null}]}`,
		"",
		`data: {"id":"chatcmpl_literal_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[{"index":0,"delta":{"content":" text"},"finish_reason":"stop"}]}`,
		"",
		`data: {"id":"chatcmpl_literal_stream","object":"chat.completion.chunk","model":"gpt-5.4","choices":[],"usage":{"prompt_tokens":4,"completion_tokens":8,"total_tokens":12}}`,
		"",
		"data: [DONE]",
		"",
	}, "\n")
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "x-request-id": []string{"rid_literal_stream"}},
		Body:       io.NopCloser(strings.NewReader(upstreamBody)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	account := forceChatResponsesFallbackAccount()
	account.Extra[CodexThinkingTagNormalizationExtraKey] = true
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	wire := rec.Body.String()
	require.Contains(t, wire, "event: response.output_item.added")
	require.Contains(t, wire, `"type":"reasoning"`)
	require.Contains(t, wire, "event: response.reasoning_summary_part.added")
	require.Contains(t, wire, "event: response.reasoning_summary_text.delta")
	require.Contains(t, wire, `"delta":"Starting Docker targeted tests****Checking for compile errors and test failures"`)
	require.Contains(t, wire, "event: response.reasoning_summary_text.done")
	require.Contains(t, wire, "event: response.reasoning_summary_part.done")
	require.Contains(t, wire, "event: response.output_text.delta")
	require.Contains(t, wire, `"delta":"normal"`)
	require.Contains(t, wire, `"delta":" text"`)
	require.Contains(t, wire, "event: response.completed")
	require.Contains(t, wire, "data: [DONE]")
	require.NotContains(t, wire, "<thinking>")
}

func TestForwardResponses_AutoSupportedAccountStillUsesResponsesEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)

	body := []byte(`{"model":"gpt-5.4","input":"hello","stream":false}`)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}, "x-request-id": []string{"rid_resp_native"}},
		Body: io.NopCloser(strings.NewReader(
			`{"id":"resp_native","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}],"status":"completed"}],"usage":{"input_tokens":5,"output_tokens":2,"total_tokens":7}}`,
		)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
	}
	account := rawChatCompletionsTestAccount()
	account.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode:      string(openai_compat.ResponsesSupportModeAuto),
		openai_compat.ExtraKeyResponsesSupported: true,
	}

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "http://upstream.example/v1/responses", upstream.lastReq.URL.String())
	require.True(t, gjson.GetBytes(upstream.lastBody, "input").Exists())
	require.False(t, gjson.GetBytes(upstream.lastBody, "messages").Exists())
	require.Equal(t, "ok", gjson.Get(rec.Body.String(), "output.0.content.0.text").String())
}

func forceChatResponsesFallbackAccount() *Account {
	account := rawChatCompletionsTestAccount()
	account.Extra = map[string]any{
		openai_compat.ExtraKeyResponsesMode: string(openai_compat.ResponsesSupportModeForceChatCompletions),
	}
	return account
}

package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesEmptyCompletedFailsOver(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_empty\",\"object\":\"response\",\"status\":\"in_progress\"}}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_empty\",\"object\":\"response\",\"status\":\"completed\"}}\n\n",
		)),
	}}
	svc := newOpenAIImageGenerationControlTestService(upstream)
	c, recorder := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.144.1")
	account := newOpenAIImageGenerationControlTestAccount()
	account.Extra = map[string]any{"openai_passthrough": true}

	body := []byte(`{
		"model":"gpt-5.6-sol",
		"stream":true,
		"input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"}]}]
	}`)

	_, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.Empty(t, recorder.Body.String())
}

func TestOpenAIResponsesEmptyCompletedWithOutputSucceeds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_ok\",\"object\":\"response\",\"status\":\"in_progress\"}}\n\n" +
				"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n" +
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_ok\",\"object\":\"response\",\"status\":\"completed\",\"usage\":{\"input_tokens\":10,\"output_tokens\":5,\"total_tokens\":15}}}\n\n",
		)),
	}}
	svc := newOpenAIImageGenerationControlTestService(upstream)
	c, recorder := newOpenAIImageGenerationControlTestContext(true, "codex_cli_rs/0.144.1")
	account := newOpenAIImageGenerationControlTestAccount()
	account.Extra = map[string]any{"openai_passthrough": true}
	body := []byte(`{"model":"gpt-5.6-sol","stream":true,"input":"continue"}`)

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, recorder.Body.String(), "hello")
	require.NotNil(t, result.Usage)
	require.Equal(t, 10, result.Usage.InputTokens)
	require.Equal(t, 5, result.Usage.OutputTokens)
}

func TestOpenAIResponsesCompletedEventIsEmpty(t *testing.T) {
	tests := []struct {
		name  string
		data  string
		usage *OpenAIUsage
		want  bool
	}{
		{name: "bare completed", data: `{"type":"response.completed"}`, want: true},
		{name: "empty output", data: `{"type":"response.completed","response":{"output":[]}}`, want: true},
		{name: "usage", data: `{"type":"response.completed","response":{"usage":{"input_tokens":1}}}`, want: false},
		{name: "error", data: `{"type":"response.completed","response":{"error":{"code":"x"}}}`, want: false},
		{name: "output", data: `{"type":"response.completed","response":{"output":[{"type":"message"}]}}`, want: false},
		{name: "accumulated usage", data: `{"type":"response.completed"}`, usage: &OpenAIUsage{InputTokens: 7}, want: false},
		{name: "invalid", data: `{"type":`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, openAIResponsesCompletedEventIsEmpty([]byte(tt.data), tt.usage))
		})
	}
}

func TestOpenAIResponsesEmptyCompletedErrorPreservesRequestID(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	err := newOpenAIResponsesEmptyCompletedFailoverError(c, nil, " req-empty ")
	require.Equal(t, "req-empty", err.ResponseHeaders.Get("x-request-id"))
}

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

func TestOAuthResponsesRespectClientStreamAndTerminalStatus(t *testing.T) {
	for _, passthrough := range []bool{false, true} {
		for _, contentType := range []string{"text/event-stream", ""} {
			for _, terminal := range []string{"completed", "incomplete", "cancelled", "canceled"} {
				t.Run(fmt.Sprintf("passthrough=%v/header=%s/%s", passthrough, contentType, terminal), func(t *testing.T) {
					body := []byte(`{"model":"gpt-5.4","instructions":"fixture","input":"hello","stream":false}`)
					recorder := httptest.NewRecorder()
					c, _ := gin.CreateTestContext(recorder)
					c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
					header := http.Header{"X-Request-Id": []string{"rid-sync-7979"}}
					if contentType != "" {
						header.Set("Content-Type", contentType)
					}
					incomplete := ""
					if terminal == "incomplete" {
						incomplete = `,"incomplete_details":{"reason":"max_output_tokens"}`
					}
					sse := fmt.Sprintf("event: response.%s\ndata: {\"type\":\"response.%s\",\"response\":{\"id\":\"resp-sync\",\"object\":\"response\",\"status\":\"%s\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"ok\"}]}],\"usage\":{\"input_tokens\":5,\"output_tokens\":16,\"total_tokens\":21}%s}}\n\n", terminal, terminal, terminal, incomplete)
					upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(sse))}}
					cfg := &config.Config{}
					svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, responseHeaderFilter: compileResponseHeaderFilter(cfg)}
					account := &Account{ID: 7979, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Concurrency: 1,
						Credentials: map[string]any{"access_token": "fixture-token", "chatgpt_account_id": "fixture-account"},
						Extra:       map[string]any{"openai_passthrough": passthrough}, Status: StatusActive, Schedulable: true}
					result, err := svc.Forward(context.Background(), c, account, body)
					require.NoError(t, err)
					require.NotNil(t, result)
					require.False(t, result.Stream)
					require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool(), "OAuth upstream remains streaming")
					require.Contains(t, recorder.Header().Get("Content-Type"), "application/json")
					require.True(t, gjson.ValidBytes(recorder.Body.Bytes()), recorder.Body.String())
					require.Equal(t, terminal, gjson.GetBytes(recorder.Body.Bytes(), "status").String())
					require.Equal(t, "ok", gjson.GetBytes(recorder.Body.Bytes(), "output.0.content.0.text").String())
					require.Equal(t, 5, result.Usage.InputTokens)
					require.Equal(t, 16, result.Usage.OutputTokens)
					if terminal == "incomplete" {
						require.Equal(t, "max_output_tokens", gjson.GetBytes(recorder.Body.Bytes(), "incomplete_details.reason").String())
					}
				})
			}
		}
	}
}

func TestNonStreamingExtractorKeepsCompletionOnlyCallersUnchanged(t *testing.T) {
	sse := "data: {\"type\":\"response.incomplete\",\"response\":{\"id\":\"resp\",\"status\":\"incomplete\"}}\n\n"
	response, ok := extractOpenAINonStreamingFinalResponse(sse)
	require.True(t, ok)
	require.Equal(t, "incomplete", gjson.GetBytes(response, "status").String())
	_, ok = extractCodexFinalResponse(sse)
	require.False(t, ok)
}

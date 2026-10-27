//go:build unit

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func TestOpenAIModelPricingSyncForwardAndSettlement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct{ model, requested, actual string }{
		{"gpt-6.1-terra", "default", "default"},
		{"gpt-6.1-sol", "ultrafast", "ultrafast"},
		{"gpt-6.1-sol", "ultrafast", "default"},
	} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%s/%s/stream=%t", tc.model, tc.requested, tc.actual, stream), func(t *testing.T) {
				body, err := json.Marshal(map[string]any{"model": tc.model, "input": "hello", "stream": stream, "service_tier": tc.requested,
					"reasoning": map[string]any{"effort": "none"}})
				require.NoError(t, err)
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
				c.Request.Header.Set("Content-Type", "application/json")
				response := fmt.Sprintf(`{"id":"resp_sync","object":"response","model":%q,"status":"completed","service_tier":%q,"output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"ok"}],"status":"completed"}],"usage":{"input_tokens":1100,"output_tokens":10,"total_tokens":1110,"input_tokens_details":{"cached_tokens":100}}}`, tc.model, tc.actual)
				contentType := "application/json"
				if stream {
					contentType = "text/event-stream"
					response = "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":" + response + "}\n\n"
				}
				upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: http.StatusOK,
					Header: http.Header{"Content-Type": []string{contentType}, "X-Request-Id": []string{"fixture-sync"}},
					Body:   io.NopCloser(strings.NewReader(response))}}
				svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
				account := rawChatCompletionsTestAccount()
				account.Extra = map[string]any{openai_compat.ExtraKeyResponsesSupported: true}
				result, err := svc.Forward(context.Background(), c, account, body)
				require.NoError(t, err)
				require.Equal(t, http.StatusOK, rec.Code)
				require.Equal(t, tc.model, gjson.GetBytes(upstream.lastBody, "model").String())
				require.Equal(t, tc.requested, gjson.GetBytes(upstream.lastBody, "service_tier").String())
				if tc.model == "gpt-6.1-terra" {
					require.Equal(t, "none", gjson.GetBytes(upstream.lastBody, "reasoning.effort").String())
				}
				require.NotNil(t, result.Usage.ServiceTier)
				require.Equal(t, tc.actual, *result.Usage.ServiceTier)
				logs := &openAIRecordUsageLogRepoStub{inserted: true}
				recorder := newOpenAIRecordUsageServiceForTest(logs, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
				err = recorder.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
					Result: result, APIKey: &APIKey{ID: 1}, User: &User{ID: 2}, Account: account})
				require.NoError(t, err)
				require.NotNil(t, logs.lastLog)
				require.Equal(t, tc.actual, *logs.lastLog.ServiceTier)
				expected, err := recorder.billingService.CalculateCostWithServiceTier(tc.model,
					UsageTokens{InputTokens: 1000, CacheReadTokens: 100, OutputTokens: 10}, 1, tc.actual)
				require.NoError(t, err)
				require.InDelta(t, expected.TotalCost, logs.lastLog.TotalCost, 1e-12)
			})
		}
	}
}

func TestOpenAIUltrafastChatStreamActualTier(t *testing.T) {
	for _, tier := range []string{"default", "ultrafast"} {
		usage := extractCCStreamUsage(fmt.Sprintf(`{"service_tier":%q,"usage":{"prompt_tokens":1100,"completion_tokens":10,"prompt_tokens_details":{"cached_tokens":100}}}`, tier))
		require.NotNil(t, usage)
		require.NotNil(t, usage.ServiceTier)
		require.Equal(t, tier, *usage.ServiceTier)
		require.Equal(t, 1100, usage.InputTokens)
		require.Equal(t, 100, usage.CacheReadInputTokens)
	}
	usage := extractCCStreamUsage(`{"usage":{"prompt_tokens":1,"completion_tokens":1}}`)
	require.NotNil(t, usage)
	require.Nil(t, usage.ServiceTier)
}

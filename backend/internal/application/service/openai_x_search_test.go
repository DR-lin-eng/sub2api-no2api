//go:build unit

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

	"github.com/Wei-Shaw/sub2api/internal/shared/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardGrokXSearchSuccessPreservesBillingContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/x_search", nil)

	account := grokXSearchTestAccount()
	repo := &grokQuotaAccountRepo{}
	responseBody := []byte(`{
		"id":"resp_x_search",
		"output":[{"type":"x_search_call","action":{"sources":[]}}],
		"usage":{"input_tokens":12,"output_tokens":4,"input_tokens_details":{"cached_tokens":3}}
	}`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":                   []string{"application/json"},
			"Xai-Request-Id":                 []string{"upstream-request-id"},
			"X-Ratelimit-Limit-Requests":     []string{"10"},
			"X-Ratelimit-Remaining-Requests": []string{"9"},
		},
		Body: io.NopCloser(bytes.NewReader(responseBody)),
	}}
	svc := &OpenAIGatewayService{
		cfg:          rawChatCompletionsTestConfig(),
		httpUpstream: upstream,
		accountRepo:  repo,
	}

	result, err := svc.ForwardGrokXSearch(
		context.Background(), c, account,
		[]byte(`{"model":"grok-4.6-latest","input":"find this","stream":false}`),
		"x_search:client-request-id",
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, responseBody, result.Body)
	require.Equal(t, "http://upstream.example/v1/responses", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer xai-test-key", upstream.lastReq.Header.Get("Authorization"))
	require.Equal(t, "grok-4.6", gjson.GetBytes(upstream.lastBody, "model").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
	require.Equal(t, "x_search:client-request-id", result.Result.RequestID)
	require.Equal(t, "resp_x_search", result.Result.ResponseID)
	require.Equal(t, "grok-x-search", result.Result.Model)
	require.Equal(t, "grok-4.6", result.Result.UpstreamModel)
	require.Equal(t, grokChatResponsesEndpoint, result.Result.UpstreamEndpoint)
	require.Equal(t, 1, result.Result.WebSearchCalls)
	require.Equal(t, 12, result.Result.Usage.InputTokens)
	require.Equal(t, 4, result.Result.Usage.OutputTokens)
	require.Equal(t, 3, result.Result.Usage.CacheReadInputTokens)
	require.Equal(t, "upstream-request-id", result.Result.ResponseHeaders.Get("xai-request-id"))

	snapshot, ok := repo.updates[account.ID][grokQuotaSnapshotExtraKey].(*xai.QuotaSnapshot)
	require.True(t, ok)
	require.Equal(t, "grok-4.6", snapshot.Model)
}

func TestForwardGrokXSearchInvalidJSONReturnsProtocolFailover(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/x_search", nil)

	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Request-Id": []string{"invalid-json-id"}},
		Body:       io.NopCloser(strings.NewReader(`{"id":`)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.ForwardGrokXSearch(
		context.Background(), c, grokXSearchTestAccount(),
		[]byte(`{"model":"grok-4.5","input":"find this"}`), "request-id",
	)

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadGateway, failoverErr.StatusCode)
	require.JSONEq(t, `{"error":{"type":"upstream_error","message":"Invalid Grok x_search response"}}`, string(failoverErr.ResponseBody))
}

func TestForwardGrokXSearchUpstreamFailureReturnsFailoverMetadata(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/x_search", nil)

	responseBody := []byte(`{"error":{"message":"service temporarily unavailable"}}`)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header: http.Header{
			"Content-Type": []string{"application/json"},
			"X-Request-Id": []string{"failover-request-id"},
		},
		Body: io.NopCloser(bytes.NewReader(responseBody)),
	}}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}

	result, err := svc.ForwardGrokXSearch(
		context.Background(), c, grokXSearchTestAccount(),
		[]byte(`{"model":"grok-4.5","input":"find this"}`), "request-id",
	)

	require.Nil(t, result)
	var failoverErr *UpstreamFailoverError
	require.True(t, errors.As(err, &failoverErr))
	require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
	require.Equal(t, responseBody, failoverErr.ResponseBody)
	require.Equal(t, "failover-request-id", failoverErr.ResponseHeaders.Get("x-request-id"))
}

func grokXSearchTestAccount() *Account {
	return &Account{
		ID:          8601,
		Name:        "grok-x-search-test",
		Platform:    PlatformGrok,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Credentials: map[string]any{
			"api_key":  "xai-test-key",
			"base_url": "http://upstream.example/v1",
		},
	}
}

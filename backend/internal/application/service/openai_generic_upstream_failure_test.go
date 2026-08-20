package service

import (
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

const genericOpenAIUpstreamFailureBody = `{"error":{"message":"Upstream request failed","type":"upstream_error"}}`

func TestIsOpenAIGenericUpstreamFailureBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "top-level sentinel", body: genericOpenAIUpstreamFailureBody, want: true},
		{
			name: "responses nested sentinel",
			body: `{"type":"response.failed","response":{"error":{"type":"upstream_error","message":"Upstream request failed"}}}`,
			want: true,
		},
		{
			name: "actionable client error is not generic",
			body: `{"error":{"type":"invalid_request_error","message":"Upstream request failed"}}`,
			want: false,
		},
		{
			name: "specific message is not generic",
			body: `{"error":{"type":"upstream_error","message":"Invalid input"}}`,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, IsOpenAIGenericUpstreamFailureBody([]byte(tt.body)))
		})
	}
}

func TestHandleErrorResponse_Generic400ReturnsFailoverBeforeCommit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(genericOpenAIUpstreamFailureBody)),
	}
	account := &Account{ID: 7001, Name: "generic-error", Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	_, err := svc.handleErrorResponse(context.Background(), resp, c, account, nil, "gpt-5.5")
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusBadRequest, failoverErr.StatusCode)
	require.False(t, IsResponseCommitted(c))
	require.Empty(t, recorder.Body.String())
}

func TestHandleFailoverErrorResponsePassthrough_GenericSentinelIsNotPreserved(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	svc := &OpenAIGatewayService{cfg: &config.Config{}}
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
	account := &Account{ID: 7002, Name: "generic-passthrough", Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	err := svc.handleFailoverErrorResponsePassthrough(
		context.Background(),
		resp,
		c,
		account,
		[]byte(`{"model":"gpt-5.5","input":"hello"}`),
		[]byte(genericOpenAIUpstreamFailureBody),
	)
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.False(t, failoverErr.PreserveUpstreamResponse)
	require.True(t, IsOpenAIGenericUpstreamFailureBody(failoverErr.ResponseBody))
	require.Empty(t, recorder.Body.String())
}

func TestGenericUpstreamFailurePredicatesTriggerFailover(t *testing.T) {
	service := &OpenAIGatewayService{}
	genericErrorEvent := []byte(`{"type":"error","error":{"type":"upstream_error","message":"Upstream request failed"}}`)
	require.True(t, service.shouldFailoverOpenAIUpstreamResponse(
		http.StatusBadRequest,
		"Upstream request failed",
		[]byte(genericOpenAIUpstreamFailureBody),
	))
	require.True(t, shouldFailoverOpenAIPassthroughResponse(
		&Account{Platform: PlatformOpenAI},
		http.StatusBadRequest,
		[]byte(genericOpenAIUpstreamFailureBody),
	))
	require.True(t, openAIResponseFailureShouldFailover(genericErrorEvent, "Upstream request failed"))
	require.True(t, openAIStreamFailureIsExplicitlyRetryable(genericErrorEvent, "Upstream request failed"))

	require.Equal(t, "upstream_error", gjson.Get(genericOpenAIUpstreamFailureBody, "error.type").String())
}

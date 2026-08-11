package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"

	"github.com/Wei-Shaw/sub2api/internal/domain/model"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
)

const openAIInvalidFunctionParametersBody = `{"error":{` +
	`"message":"Invalid schema for function 'automation_update': schema must be an object.",` +
	`"type":"invalid_request_error",` +
	`"param":"input[8].tools[1].tools[2].parameters",` +
	`"code":"invalid_function_parameters"}}`

func newOpenAIUpstreamErrorTestContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	return c, recorder
}

func newOpenAIUpstreamErrorResponse(statusCode int, body string) *http.Response {
	return &http.Response{
		StatusCode: statusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newOpenAIUpstreamErrorTestAccount() *Account {
	return &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Name: "acct"}
}

func TestHandleErrorResponseDeterministic400PreservesClientError(t *testing.T) {
	c, recorder := newOpenAIUpstreamErrorTestContext(t)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	_, err := svc.handleErrorResponse(
		context.Background(),
		newOpenAIUpstreamErrorResponse(http.StatusBadRequest, openAIInvalidFunctionParametersBody),
		c,
		newOpenAIUpstreamErrorTestAccount(),
		nil,
	)

	require.Error(t, err)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	body := recorder.Body.String()
	require.Equal(t, "invalid_request_error", gjson.Get(body, "error.type").String())
	require.Equal(t, "invalid_function_parameters", gjson.Get(body, "error.code").String())
	require.Equal(t, "input[8].tools[1].tools[2].parameters", gjson.Get(body, "error.param").String())
	require.Contains(t, gjson.Get(body, "error.message").String(), "automation_update")
	require.NotContains(t, body, "Upstream request failed")

	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr))
}

func TestHandleErrorResponseDeterministic400Fallbacks(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		wantMessage string
	}{
		{
			name:        "message only",
			body:        `{"error":{"message":"Invalid 'input': expected an array."}}`,
			wantMessage: "Invalid 'input': expected an array.",
		},
		{
			name:        "non JSON",
			body:        `<html><body>400 Bad Request</body></html>`,
			wantMessage: openAIUpstreamClientErrorFallbackMessage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, recorder := newOpenAIUpstreamErrorTestContext(t)
			svc := &OpenAIGatewayService{cfg: &config.Config{}}

			_, err := svc.handleErrorResponse(
				context.Background(),
				newOpenAIUpstreamErrorResponse(http.StatusBadRequest, tt.body),
				c,
				newOpenAIUpstreamErrorTestAccount(),
				nil,
			)

			require.Error(t, err)
			require.Equal(t, http.StatusBadRequest, recorder.Code)
			responseBody := recorder.Body.String()
			require.Equal(t, "invalid_request_error", gjson.Get(responseBody, "error.type").String())
			require.Equal(t, tt.wantMessage, gjson.Get(responseBody, "error.message").String())
			require.False(t, gjson.Get(responseBody, "error.code").Exists())
			require.False(t, gjson.Get(responseBody, "error.param").Exists())
		})
	}
}

func TestHandleErrorResponseNonDeterministicStatusesRemainGeneric(t *testing.T) {
	tests := []struct {
		status      int
		wantStatus  int
		wantType    string
		wantMessage string
	}{
		{status: http.StatusNotFound, wantStatus: http.StatusBadGateway, wantType: "upstream_error", wantMessage: "Upstream request failed"},
		{status: http.StatusUnprocessableEntity, wantStatus: http.StatusBadGateway, wantType: "upstream_error", wantMessage: "Upstream request failed"},
		{status: http.StatusUnauthorized, wantStatus: http.StatusBadGateway, wantType: "upstream_error", wantMessage: "Upstream authentication failed, please contact administrator"},
		{status: http.StatusTooManyRequests, wantStatus: http.StatusTooManyRequests, wantType: "rate_limit_error", wantMessage: "Upstream rate limit exceeded, please retry later"},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			c, recorder := newOpenAIUpstreamErrorTestContext(t)
			svc := &OpenAIGatewayService{cfg: &config.Config{}}

			_, err := svc.handleErrorResponse(
				context.Background(),
				newOpenAIUpstreamErrorResponse(tt.status, `{"error":{"message":"upstream detail"}}`),
				c,
				newOpenAIUpstreamErrorTestAccount(),
				nil,
			)

			require.Error(t, err)
			require.Equal(t, tt.wantStatus, recorder.Code)
			require.Equal(t, tt.wantType, gjson.Get(recorder.Body.String(), "error.type").String())
			require.Equal(t, tt.wantMessage, gjson.Get(recorder.Body.String(), "error.message").String())
		})
	}
}

func TestHandleErrorResponsePassthroughRulePrecedesDeterministic400(t *testing.T) {
	c, recorder := newOpenAIUpstreamErrorTestContext(t)
	ruleSvc := &ErrorPassthroughService{}
	ruleSvc.setLocalCache([]*model.ErrorPassthroughRule{
		newNonFailoverPassthroughRule(http.StatusBadRequest, "automation_update", http.StatusTeapot, "custom message"),
	})
	BindErrorPassthroughService(c, ruleSvc)
	svc := &OpenAIGatewayService{cfg: &config.Config{}}

	_, err := svc.handleErrorResponse(
		context.Background(),
		newOpenAIUpstreamErrorResponse(http.StatusBadRequest, openAIInvalidFunctionParametersBody),
		c,
		newOpenAIUpstreamErrorTestAccount(),
		nil,
	)

	require.Error(t, err)
	require.Equal(t, http.StatusTeapot, recorder.Code)
	require.Equal(t, "custom message", gjson.Get(recorder.Body.String(), "error.message").String())
}

func TestWriteOpenAIUpstreamClientErrorUsesSanitizedMessage(t *testing.T) {
	c, recorder := newOpenAIUpstreamErrorTestContext(t)

	writeOpenAIUpstreamClientError(
		c,
		http.StatusBadRequest,
		[]byte(`{"error":{"message":"failed for key=secret123"}}`),
		"failed for key=***",
	)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Equal(t, "failed for key=***", gjson.Get(recorder.Body.String(), "error.message").String())
	require.NotContains(t, recorder.Body.String(), "secret123")
}

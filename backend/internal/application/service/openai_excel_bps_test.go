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
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/basispoints"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type excelBPSHTTPUpstreamStub struct {
	response   *http.Response
	err        error
	requests   []*http.Request
	bodyCopies [][]byte
}

func (s *excelBPSHTTPUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	s.capture(req)
	return s.response, s.err
}

func (s *excelBPSHTTPUpstreamStub) DoWithTLS(req *http.Request, _ string, _ int64, _ int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	s.capture(req)
	return s.response, s.err
}

func (s *excelBPSHTTPUpstreamStub) capture(req *http.Request) {
	s.requests = append(s.requests, req)
	if req == nil || req.Body == nil {
		s.bodyCopies = append(s.bodyCopies, nil)
		return
	}
	body, _ := io.ReadAll(req.Body)
	s.bodyCopies = append(s.bodyCopies, body)
	req.Body = io.NopCloser(bytes.NewReader(body))
}

func excelBPSAccount(enabled bool) *Account {
	extra := map[string]any{}
	if enabled {
		extra[ExcelBPSEnabledExtraKey] = true
	}
	return &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       "access-token",
			"chatgpt_account_id": "chatgpt-account",
			"model_mapping":      map[string]any{"client-model": "mapped-model"},
		},
		Extra: extra,
	}
}

func excelBPSUpstreamSSE() string {
	return "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\"}\n\n" +
		"event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_bps_1\",\"model\":\"mapped-model\",\"output\":[],\"usage\":{\"input_tokens\":3,\"output_tokens\":2}}}\n\n"
}

func newExcelBPSTestContext() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	return ctx, recorder
}

func TestNewExcelBPSRequestSetsRequiredEndpointAndHeaders(t *testing.T) {
	req, err := newExcelBPSRequest(context.Background(), []byte(`{"model":"gpt-5"}`), "token", "account")
	require.NoError(t, err)
	require.Equal(t, http.MethodPost, req.Method)
	require.Equal(t, basispoints.ResponsesURL, req.URL.String())
	require.Equal(t, "Bearer token", req.Header.Get("Authorization"))
	require.Equal(t, "account", req.Header.Get("Chatgpt-Account-Id"))
	require.Equal(t, "account", req.Header.Get("X-Openai-Account-Id"))
	require.Equal(t, "chatgpt", req.Header.Get("X-Basispoints-Auth-Mode"))
	require.Equal(t, "basispoints-excel-plugin", req.Header.Get("X-Openai-Internal-Basispoints-Client-Product"))
	require.Equal(t, "excel", req.Header.Get("X-Openai-Internal-Basispoints-Client-Agent-Profile"))
	require.Equal(t, "text/event-stream", req.Header.Get("Accept"))
}

func TestForwardExcelBPSStreamsMappedModelAndUsage(t *testing.T) {
	stub := &excelBPSHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"req_bps_1"}},
		Body:       io.NopCloser(strings.NewReader(excelBPSUpstreamSSE())),
	}}
	svc := &OpenAIGatewayService{httpUpstream: stub}
	ctx, recorder := newExcelBPSTestContext()

	result, err := svc.forwardExcelBPS(
		context.Background(),
		ctx,
		excelBPSAccount(true),
		[]byte(`{"model":"client-model","input":"hello","stream":true}`),
		time.Now(),
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "mapped-model", result.UpstreamModel)
	require.Equal(t, "resp_bps_1", result.ResponseID)
	require.Equal(t, "/basispoints/api/responses", result.UpstreamEndpoint)
	require.Equal(t, 3, result.Usage.InputTokens)
	require.Equal(t, 2, result.Usage.OutputTokens)
	require.Len(t, stub.requests, 1)
	require.Equal(t, basispoints.ResponsesURL, stub.requests[0].URL.String())
	require.Equal(t, "Bearer access-token", stub.requests[0].Header.Get("Authorization"))
	require.Equal(t, "chatgpt-account", stub.requests[0].Header.Get("Chatgpt-Account-Id"))
	require.Contains(t, string(stub.bodyCopies[0]), `"model":"mapped-model"`)
	require.Contains(t, recorder.Body.String(), "response.completed")
	require.Contains(t, recorder.Body.String(), "resp_bps_1")
}

func TestForwardExcelBPSNonStreamReturnsCompletedJSON(t *testing.T) {
	stub := &excelBPSHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:       io.NopCloser(strings.NewReader(excelBPSUpstreamSSE())),
	}}
	svc := &OpenAIGatewayService{httpUpstream: stub}
	ctx, recorder := newExcelBPSTestContext()

	result, err := svc.forwardExcelBPS(
		context.Background(),
		ctx,
		excelBPSAccount(true),
		[]byte(`{"model":"client-model","input":"hello","stream":false}`),
		time.Now(),
	)

	require.NoError(t, err)
	require.Equal(t, "resp_bps_1", result.ResponseID)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
	require.Contains(t, recorder.Body.String(), `"id":"resp_bps_1"`)
}

func TestForwardExcelBPSRejectsDisabledAccountBeforeUpstream(t *testing.T) {
	stub := &excelBPSHTTPUpstreamStub{}
	svc := &OpenAIGatewayService{httpUpstream: stub}
	ctx, recorder := newExcelBPSTestContext()

	result, err := svc.forwardExcelBPS(
		context.Background(),
		ctx,
		excelBPSAccount(false),
		[]byte(`{"model":"client-model","input":"hello"}`),
		time.Now(),
	)

	require.Error(t, err)
	require.Nil(t, result)
	require.Empty(t, stub.requests)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Excel BPS is not enabled")
}

func TestForwardExcelBPSMapsUpstreamError(t *testing.T) {
	stub := &excelBPSHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"error":"forbidden"}`)),
	}}
	svc := &OpenAIGatewayService{httpUpstream: stub}
	ctx, recorder := newExcelBPSTestContext()

	result, err := svc.forwardExcelBPS(
		context.Background(),
		ctx,
		excelBPSAccount(true),
		[]byte(`{"model":"client-model","input":"hello"}`),
		time.Now(),
	)

	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusForbidden, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Excel BPS rejected this request")
}

func TestForwardExcelBPSJSONResponseSupportsBothClientModes(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%t", stream), func(t *testing.T) {
			stub := &excelBPSHTTPUpstreamStub{response: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"resp_json","status":"completed","model":"mapped-model","output":[],"usage":{"input_tokens":3,"output_tokens":2}}`)),
			}}
			svc := &OpenAIGatewayService{httpUpstream: stub}
			ctx, recorder := newExcelBPSTestContext()
			body := fmt.Sprintf(`{"model":"client-model","input":"hello","stream":%t}`, stream)
			result, err := svc.forwardExcelBPS(context.Background(), ctx, excelBPSAccount(true), []byte(body), time.Now())
			require.NoError(t, err)
			require.Equal(t, "resp_json", result.ResponseID)
			require.Equal(t, "response.completed", result.UpstreamTerminalEvent)
			require.Equal(t, "mapped-model", result.UpstreamResponseModel)
			if stream {
				require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
				require.Contains(t, recorder.Body.String(), `"type":"response.completed"`)
				require.Contains(t, recorder.Body.String(), `"response":{"id":"resp_json"`)
			} else {
				require.Equal(t, "application/json", recorder.Header().Get("Content-Type"))
				require.JSONEq(t, `{"id":"resp_json","status":"completed","model":"mapped-model","output":[],"usage":{"input_tokens":3,"output_tokens":2}}`, recorder.Body.String())
			}
		})
	}
}

func TestForwardExcelBPSRejectsUnsupportedNativeToolWithoutFallback(t *testing.T) {
	stub := &excelBPSHTTPUpstreamStub{}
	svc := &OpenAIGatewayService{httpUpstream: stub}
	ctx, recorder := newExcelBPSTestContext()
	result, err := svc.forwardExcelBPS(context.Background(), ctx, excelBPSAccount(true), []byte(`{"model":"client-model","tools":[{"type":"image_generation"}],"input":"hello"}`), time.Now())
	require.Error(t, err)
	require.Nil(t, result)
	require.Empty(t, stub.requests)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "Excel BPS does not support")
}

func TestExcelBPSCacheScopeIsAccountKeyAndThreadScoped(t *testing.T) {
	require.Empty(t, excelBPSCacheScope(1, 2, ""))
	require.Empty(t, excelBPSCacheScope(1, 2, "  "))
	scope := excelBPSCacheScope(1, 2, "thread-a")
	require.Equal(t, "account:1/key:2/thread:thread-a", scope)
	require.NotEqual(t, scope, excelBPSCacheScope(1, 3, "thread-a"))
	require.NotEqual(t, scope, excelBPSCacheScope(2, 2, "thread-a"))
	require.NotEqual(t, scope, excelBPSCacheScope(1, 2, "thread-b"))
}

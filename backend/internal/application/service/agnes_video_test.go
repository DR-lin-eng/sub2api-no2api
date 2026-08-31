package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type customModelVideoTypeResolverStub struct {
	videoAPIType string
	configured   bool
	err          error
}

type agnesVideoHTTPUpstreamStub struct {
	request  *http.Request
	response *http.Response
}

func (s *agnesVideoHTTPUpstreamStub) Do(
	req *http.Request,
	_ string,
	_ int64,
	_ int,
) (*http.Response, error) {
	s.request = req
	return s.response, nil
}

func (s *agnesVideoHTTPUpstreamStub) DoWithTLS(
	req *http.Request,
	proxyURL string,
	accountID int64,
	accountConcurrency int,
	_ *tlsfingerprint.Profile,
) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func (s customModelVideoTypeResolverStub) HasCapability(context.Context, string, string) (bool, error) {
	return false, nil
}

func (s customModelVideoTypeResolverStub) ResolveVideoAPIType(context.Context, string) (string, bool, error) {
	return s.videoAPIType, s.configured, s.err
}

func (s customModelVideoTypeResolverStub) ResolveRequestAdapter(context.Context, string) (map[string]any, bool, error) {
	return nil, false, nil
}

func TestResolveCustomModelVideoAPITypeUsesConfiguredProtocol(t *testing.T) {
	svc := &OpenAIGatewayService{customModelCapabilities: customModelVideoTypeResolverStub{
		videoAPIType: "agnes",
		configured:   true,
	}}

	videoAPIType, configured, err := svc.ResolveCustomModelVideoAPIType(context.Background(), "agnes-video-v2")
	require.NoError(t, err)
	require.True(t, configured)
	require.Equal(t, "agnes", videoAPIType)
}

func TestBuildAgnesVideoURLUsesAgnesProtocolPaths(t *testing.T) {
	account := &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{
		"base_url": "https://agnes.example/v1/",
	}}

	generationURL, err := buildAgnesVideoURL(account, AgnesVideoEndpointGenerations, "")
	require.NoError(t, err)
	require.Equal(t, "https://agnes.example/v1/video/generations", generationURL)

	statusURL, err := buildAgnesVideoURL(account, AgnesVideoEndpointStatus, "task/123")
	require.NoError(t, err)
	require.Equal(t, "https://agnes.example/v1/video/generations/task%2F123", statusURL)

	contentURL, err := buildAgnesVideoURL(account, AgnesVideoEndpointContent, "task/123")
	require.NoError(t, err)
	require.Equal(t, "https://agnes.example/v1/video/generations/task%2F123/content", contentURL)
}

func TestForwardAgnesVideoContentUsesBoundAccountEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/videos/task-1/content", nil)
	c.Request.Header.Set("Range", "bytes=0-3")
	upstream := &agnesVideoHTTPUpstreamStub{response: &http.Response{
		StatusCode: http.StatusPartialContent,
		Header: http.Header{
			"Content-Type":  []string{"video/mp4"},
			"Content-Range": []string{"bytes 0-3/8"},
		},
		Body: io.NopCloser(strings.NewReader("data")),
	}}
	svc := &OpenAIGatewayService{httpUpstream: upstream}
	account := &Account{ID: 9, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"base_url": "https://agnes.example/v1",
	}}

	result, err := svc.forwardAgnesVideoContent(
		context.Background(), c, account, "https://agnes.example/v1", "secret-token", "task-1", time.Now(),
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, upstream.request)
	require.Equal(t, "https://agnes.example/v1/video/generations/task-1/content", upstream.request.URL.String())
	require.Equal(t, "Bearer secret-token", upstream.request.Header.Get("Authorization"))
	require.Equal(t, "bytes=0-3", upstream.request.Header.Get("Range"))
	require.Equal(t, http.StatusPartialContent, recorder.Code)
	require.Equal(t, "private, no-store", recorder.Header().Get("Cache-Control"))
	require.Equal(t, "nosniff", recorder.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "data", recorder.Body.String())
}

func TestForwardAgnesVideoRejectsBaseURLOutsideAllowlist(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", nil)
	svc := &OpenAIGatewayService{cfg: &config.Config{Security: config.SecurityConfig{
		URLAllowlist: config.URLAllowlistConfig{
			Enabled:       true,
			UpstreamHosts: []string{"agnes.example"},
		},
	}}}
	account := &Account{Type: AccountTypeAPIKey, Credentials: map[string]any{
		"base_url": "https://attacker.example/v1",
	}}

	_, err := svc.ForwardAgnesVideo(
		context.Background(),
		c,
		account,
		AgnesVideoEndpointGenerations,
		"",
		[]byte(`{"model":"agnes-video-v2","prompt":"test"}`),
		"application/json",
	)
	require.ErrorContains(t, err, "invalid base_url")
}

func TestAgnesVideoErrorLogBodyIsBoundedAndRedacted(t *testing.T) {
	body := []byte(`{"access_token":"sensitive-token","message":"` + strings.Repeat("x", 4096) + `"}`)

	logged := agnesVideoErrorLogBody(body)

	require.NotContains(t, logged, "sensitive-token")
	require.LessOrEqual(t, len(logged), agnesVideoErrorLogMaxBytes)
}

func TestAgnesVideoTaskBindingIsScopedToUserAndAPIKey(t *testing.T) {
	groupID := int64(7)
	cache := &stubGatewayCache{}
	svc := &OpenAIGatewayService{cache: cache}
	ctx := context.Background()

	require.NoError(t, svc.BindAgnesVideoTaskAccount(ctx, &groupID, "task-123", 41, 51, 63))
	accountID, err := svc.ResolveAgnesVideoTaskAccount(ctx, &groupID, "task-123", 41, 51)
	require.NoError(t, err)
	require.Equal(t, int64(63), accountID)

	accountID, err = svc.ResolveAgnesVideoTaskAccount(ctx, &groupID, "task-123", 42, 51)
	require.Error(t, err)
	require.Zero(t, accountID)
	accountID, err = svc.ResolveAgnesVideoTaskAccount(ctx, &groupID, "task-123", 41, 52)
	require.Error(t, err)
	require.Zero(t, accountID)
}

func TestAgnesVideoRequestErrorsStopFailoverAndPreserveResponse(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", nil)
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 3}
	responseBody := `{"error":{"message":"invalid video request"}}`
	resp := &http.Response{
		StatusCode: http.StatusBadRequest,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(responseBody)),
	}

	_, err := svc.handleAgnesVideoErrorResponse(context.Background(), resp, c, account, "req-1", "agnes-video-v2")
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.False(t, failoverErr.ShouldRetryNextAccount())
	require.Equal(t, GatewayFailureScopeRequest, failoverErr.Scope)
	require.True(t, failoverErr.PreserveUpstreamResponse)
	require.JSONEq(t, responseBody, string(failoverErr.ResponseBody))
}

func TestAgnesVideoModelBlockedStopsFailoverAndPreservesForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/videos/generations", nil)
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 3}
	responseBody := `{"error":{"message":"litellm.PermissionDeniedError: Model is blocked","type":null,"param":null,"code":"403"}}`
	resp := &http.Response{
		StatusCode: http.StatusForbidden,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(responseBody)),
	}

	_, err := svc.handleAgnesVideoErrorResponse(context.Background(), resp, c, account, "req-2", "agnes-video-v2")
	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, err, &failoverErr)
	require.Equal(t, http.StatusForbidden, failoverErr.StatusCode)
	require.False(t, failoverErr.ShouldRetryNextAccount())
	require.Equal(t, GatewayFailureScopeRequest, failoverErr.Scope)
	require.True(t, failoverErr.PreserveUpstreamResponse)
	require.JSONEq(t, responseBody, string(failoverErr.ResponseBody))
}

func TestRewriteAgnesVideoContentURLsSupportsRootVideoURL(t *testing.T) {
	result := rewriteAgnesVideoContentURLs(
		[]byte(`{"task_id":"task-1","video_url":"https://upstream.example/video.mp4"}`),
		"task-1",
		"/v1/videos/task-1/content",
	)
	require.Equal(t, "/v1/videos/task-1/content", gjson.GetBytes(result, "video_url").String())
}

func TestAgnesVideoContentProxyURLEscapesTaskID(t *testing.T) {
	require.Equal(t, "/v1/videos/task%2F123/content", agnesVideoContentProxyURL(nil, "task/123"))
}

func TestResolveCustomModelVideoAPITypePropagatesResolverError(t *testing.T) {
	wantErr := errors.New("resolver failed")
	svc := &OpenAIGatewayService{customModelCapabilities: customModelVideoTypeResolverStub{err: wantErr}}

	_, _, err := svc.ResolveCustomModelVideoAPIType(context.Background(), "agnes-video-v2")
	require.ErrorIs(t, err, wantErr)
}

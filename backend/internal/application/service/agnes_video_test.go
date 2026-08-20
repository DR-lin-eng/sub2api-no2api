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
)

type customModelVideoTypeResolverStub struct {
	videoAPIType string
	configured   bool
	err          error
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

func TestResolveCustomModelVideoAPITypePropagatesResolverError(t *testing.T) {
	wantErr := errors.New("resolver failed")
	svc := &OpenAIGatewayService{customModelCapabilities: customModelVideoTypeResolverStub{err: wantErr}}

	_, _, err := svc.ResolveCustomModelVideoAPIType(context.Background(), "agnes-video-v2")
	require.ErrorIs(t, err, wantErr)
}

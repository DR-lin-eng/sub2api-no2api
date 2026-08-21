package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSanitizedUpstreamPathSuffix(t *testing.T) {
	for _, suffix := range []string{
		"/..", "/../..", "/./compact", "/compact/..", `/.\\x`,
		"/?a=b", "/compact?a=b", "/compact#frag", "/100%", "//double",
		"/compact//detail", "/compact/", "/ compact", "/compact\x00",
		"/compact\nX-Injected: 1", "/模型", "compact", "/a:b", "/a@b",
		"/...", "/compact/...",
	} {
		t.Run("reject_"+suffix, func(t *testing.T) {
			got, ok := sanitizedUpstreamPathSuffix(suffix)
			require.False(t, ok)
			require.Empty(t, got)
		})
	}

	for suffix, want := range map[string]string{
		"": "", "/compact": "/compact", "/compact/detail": "/compact/detail",
		"/resp_68f0a1b2c3d4/cancel": "/resp_68f0a1b2c3d4/cancel",
		"/gemini-2.5-pro_v1.2":      "/gemini-2.5-pro_v1.2",
	} {
		t.Run("accept_"+suffix, func(t *testing.T) {
			got, ok := sanitizedUpstreamPathSuffix(suffix)
			require.True(t, ok)
			require.Equal(t, want, got)
		})
	}
}

func TestSanitizedUpstreamPathSuffixEnforcesBounds(t *testing.T) {
	_, ok := sanitizedUpstreamPathSuffix("/" + strings.Repeat("a", maxUpstreamPathSegmentLen+1))
	require.False(t, ok)
	_, ok = sanitizedUpstreamPathSuffix(strings.Repeat("/a", maxUpstreamPathSegments+1))
	require.False(t, ok)
}

func TestOpenAIResponsesRequestPathSuffixRejectsNonConformingSubpaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{
		"/v1/responses/../../x/y", "/v1/responses/..%2f..%2fx/y",
		"/v1/responses/%2e%2e/%2e%2e/x", "/responses/%2e%2e%2fx",
		"/backend-api/codex/responses/../../../x", `/v1/responses/..\\..\\x`,
		"/v1/responses/%3fa=b", "/v1/responses/x%23frag", "/v1/responses//double",
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, path, nil)
		require.False(t, IsForwardableOpenAIResponsesRequestPath(c), "path=%s", path)
		require.Empty(t, openAIResponsesRequestPathSuffix(c), "path=%s", path)
		require.False(t, isOpenAIResponsesCompactPath(c), "path=%s", path)
	}

	for path, want := range map[string]string{
		"/v1/responses": "", "/v1/responses/input_tokens": "/input_tokens", "/v1/responses/compact": "/compact",
		"/responses/compact/": "/compact", "/backend-api/codex/responses/compact": "/compact",
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, path, nil)
		require.True(t, IsForwardableOpenAIResponsesRequestPath(c), "path=%s", path)
		require.Equal(t, want, openAIResponsesRequestPathSuffix(c), "path=%s", path)
	}
	inputTokens := newResponsesSuffixTestContext(t, "/v1/responses/input_tokens")
	require.True(t, IsOpenAIResponsesInputTokensRequestPath(inputTokens))
	require.False(t, IsOpenAIResponsesInputTokensRequestPath(newResponsesSuffixTestContext(t, "/v1/responses")))
}

func TestIsOpenAIResponsesCompactPathUsesLegacyEndpointShape(t *testing.T) {
	for _, path := range []string{
		"/v1/responses/compact",
		"/v1/responses/compact/detail",
		"/responses/compact/",
	} {
		t.Run("legacy_"+path, func(t *testing.T) {
			require.True(t, IsOpenAIResponsesCompactPath(newResponsesSuffixTestContext(t, path)))
		})
	}

	for _, path := range []string{
		"/v1/responses",
		"/openai/v1/responses",
		"/responses",
		"/backend-api/codex/responses",
		"/v1/responses/resp_123/cancel",
	} {
		t.Run("non_legacy_"+path, func(t *testing.T) {
			require.False(t, IsOpenAIResponsesCompactPath(newResponsesSuffixTestContext(t, path)))
		})
	}
}

func TestAppendOpenAIResponsesRequestPathSuffixRefusesUnsafeSuffix(t *testing.T) {
	require.Equal(t, chatgptCodexURL, appendOpenAIResponsesRequestPathSuffix(chatgptCodexURL, "/../../x"))
	require.Equal(t, chatgptCodexURL, appendOpenAIResponsesRequestPathSuffix(chatgptCodexURL, "/?a=b"))
	require.Equal(t, chatgptCodexURL+"/compact", appendOpenAIResponsesRequestPathSuffix(chatgptCodexURL, "/compact"))
}

func newResponsesSuffixTestContext(t *testing.T, path string) *gin.Context {
	t.Helper()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, path, nil)
	return c
}

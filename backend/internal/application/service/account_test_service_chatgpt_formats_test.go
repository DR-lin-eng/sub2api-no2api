//go:build unit

package service

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountTestService_ChatGPTAppGIFProbeUsesConversationFlow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	encoded := "data:image/gif;base64," + image26AnimatedGIF(t, 4)
	gateway, upstream := newChatGPTAppImageTestService(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/backend-api/f/conversation", r.URL.Path)
		chatGPTAppImageStream(w, encoded)
	})
	probe := &AccountTestService{
		oauthGatewayLimiter: gateway,
		settingService:      &SettingService{},
		cfg:                 &config.Config{},
		httpUpstream:        upstream,
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	frames := 4
	ctx := WithAccountImageTestOptions(t.Context(), AccountImageTestOptions{OutputFormat: "gif", FrameCount: &frames})
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil).WithContext(ctx)

	err := probe.testOpenAIImageOAuth(c, ctx, chatGPTAppImageTestAccount(), "gpt-image-2.6-flare", "draw a looping orange logo")
	require.NoError(t, err)
	require.Contains(t, rec.Body.String(), "data:image/gif;base64,")
	require.Contains(t, rec.Body.String(), `"success":true`)
	require.Contains(t, string(upstream.imageCalls()[0].body), "animated GIF")
}

func TestAccountTestService_ChatGPTAppSVGProbeFallsBackToCodexLegacyImages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svg := base64.StdEncoding.EncodeToString([]byte(`<svg xmlns="http://www.w3.org/2000/svg"><path d="M0 0L1 1"/></svg>`))
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"data":[{"b64_json":%q,"output_format":"svg"}]}`, svg))),
	}}
	settings := &SettingService{}
	probe := &AccountTestService{settingService: settings, cfg: &config.Config{}, httpUpstream: upstream}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	ctx := WithAccountImageTestOptions(t.Context(), AccountImageTestOptions{
		OutputFormat: "svg",
		SVG:          &OpenAIImagesSVGOptions{Model: "gpt-6-astra"},
	})
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil).WithContext(ctx)

	err := probe.testOpenAIImageOAuth(c, ctx, &Account{ID: 56, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "token-123"}}, "gpt-image-2.6-flare", "draw a vector logo")
	require.NoError(t, err)
	require.NotContains(t, rec.Body.String(), "falling back to the Codex legacy Images chain")
	require.NotContains(t, rec.Body.String(), "SVG is not exposed")
	require.Contains(t, rec.Body.String(), "data:image/svg+xml;base64,")
	require.Contains(t, rec.Body.String(), `"success":true`)
	require.NotNil(t, upstream.lastReq)
	require.Contains(t, upstream.lastReq.URL.Path, "/images/generations")
	requestBody, readErr := io.ReadAll(upstream.lastReq.Body)
	require.NoError(t, readErr)
	require.NotContains(t, string(requestBody), "falling back")
	require.NotContains(t, string(requestBody), "SVG is not exposed")
}

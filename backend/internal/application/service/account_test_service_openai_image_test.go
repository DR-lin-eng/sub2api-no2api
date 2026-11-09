package service

import (
	"context"
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

func TestAccountTestService_OpenAIImageOAuthHandlesOutputItemDoneFallback(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream"},
			},
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.output_item.done\",\"item\":{\"id\":\"ig_123\",\"type\":\"image_generation_call\",\"result\":\"aGVsbG8=\",\"revised_prompt\":\"draw a cat\",\"output_format\":\"png\"}}\n\n" +
					"data: {\"type\":\"response.completed\",\"response\":{\"created_at\":1710000006,\"tool_usage\":{\"image_gen\":{\"images\":1}},\"output\":[]}}\n\n" +
					"data: [DONE]\n\n",
			)),
		},
	}
	svc := &AccountTestService{httpUpstream: upstream}
	account := &Account{
		ID:       53,
		Name:     "openai-oauth",
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token": "token-123",
		},
	}

	err := svc.testOpenAIImageOAuth(c, context.Background(), account, "gpt-image-2", "draw a cat")
	require.NoError(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))
	require.Contains(t, rec.Body.String(), "Calling Codex /responses image tool")
	require.Contains(t, rec.Body.String(), "data:image/png;base64,aGVsbG8=")
	require.Contains(t, rec.Body.String(), "\"success\":true")
}

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
	require.Contains(t, rec.Body.String(), "falling back to the Codex legacy Images chain")
	require.Contains(t, rec.Body.String(), "data:image/svg+xml;base64,")
	require.Contains(t, rec.Body.String(), `"success":true`)
	require.NotNil(t, upstream.lastReq)
	require.Contains(t, upstream.lastReq.URL.Path, "/images/generations")
}

func TestAccountTestService_OpenAIImageAPIKeyUsesConfiguredV1BaseURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)

	upstream := &httpUpstreamRecorder{
		resp: &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"application/json"},
			},
			Body: io.NopCloser(strings.NewReader(`{"data":[{"b64_json":"aGVsbG8=","revised_prompt":"draw a cat"}]}`)),
		},
	}
	svc := &AccountTestService{
		httpUpstream: upstream,
		cfg:          &config.Config{},
	}
	account := &Account{
		ID:       54,
		Name:     "openai-apikey",
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Credentials: map[string]any{
			"api_key":  "test-api-key",
			"base_url": "https://image-upstream.example/v1",
		},
	}

	err := svc.testOpenAIImageAPIKey(c, context.Background(), account, "gpt-image-2", "draw a cat")
	require.NoError(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, HTTPUpstreamProfileOpenAI, HTTPUpstreamProfileFromContext(upstream.lastReq.Context()))
	require.Equal(t, "https://image-upstream.example/v1/images/generations", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer test-api-key", upstream.lastReq.Header.Get("Authorization"))
	require.Contains(t, rec.Body.String(), "data:image/png;base64,aGVsbG8=")
	require.Contains(t, rec.Body.String(), "\"success\":true")
}

func TestAccountTestService_OpenAIImageOAuthSurfacesSSEError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/1/test", nil)
	svc := &AccountTestService{httpUpstream: &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(
			"data: {\"type\":\"error\",\"error\":{\"type\":\"invalid_request_error\",\"message\":\"The selected image model is unavailable\"}}\n\n",
		)),
	}}}
	account := &Account{ID: 55, Name: "openai-oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "token-123"}}

	err := svc.testOpenAIImageOAuth(c, context.Background(), account, "gpt-image-2.5-flare", "draw a cup")
	require.ErrorContains(t, err, "The selected image model is unavailable")
	require.NotContains(t, rec.Body.String(), "No images returned")
}

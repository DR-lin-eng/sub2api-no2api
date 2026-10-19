package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// A cache/refresh call can block before the upstream HTTP round trip exists.
// Exercise a real downstream Go transport, as used by relay gateways.
type startupSlowTokenCache struct {
	OpenAITokenCache
	delay time.Duration
}

func (cache *startupSlowTokenCache) GetAccessToken(ctx context.Context, _ string) (string, error) {
	timer := time.NewTimer(cache.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-timer.C:
		return "fixture-startup-token", nil
	}
}

func (cache *startupSlowTokenCache) SetAccessToken(context.Context, string, string, time.Duration) error {
	return nil
}

func TestOpenAIResponsesPreparationDownstreamHeaderTimeout(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		t.Run(fmt.Sprintf("passthrough=%t", passthrough), func(t *testing.T) {
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": {"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_startup\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")),
			}}
			svc := &OpenAIGatewayService{
				cfg: &config.Config{Gateway: config.GatewayConfig{
					StreamKeepaliveInterval: 1,
					MaxLineSize:             defaultMaxLineSize,
				}},
				httpUpstream: upstream,
				openAITokenProvider: &OpenAITokenProvider{
					tokenCache: &startupSlowTokenCache{delay: 1600 * time.Millisecond},
				},
			}
			account := &Account{
				ID: 871, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Credentials: map[string]any{"access_token": "fixture-startup-token"},
				Extra:       map[string]any{"openai_passthrough": passthrough},
			}
			body := `{"model":"gpt-5.5","stream":true,"instructions":"test","input":"hello"}`
			router := gin.New()
			router.POST("/v1/responses", func(c *gin.Context) {
				_, err := svc.Forward(c.Request.Context(), c, account, []byte(body))
				if err != nil && c.Request.Context().Err() == nil {
					writeOpenAIResponsesErrorAfterKeepalive(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
				}
			})
			gateway := httptest.NewServer(router)
			defer gateway.Close()
			transport := &http.Transport{ResponseHeaderTimeout: 1300 * time.Millisecond}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
			resp, err := client.Post(gateway.URL+"/v1/responses", "application/json", strings.NewReader(body))
			if err != nil {
				t.Logf("CLIENT_ERROR passthrough=%t: %v", passthrough, err)
			}
			require.NoError(t, err, "the gateway must flush headers while preparing credentials")
			defer resp.Body.Close()
			wire, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, resp.StatusCode)
			require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
			require.Contains(t, string(wire), "response.completed")
			require.NotContains(t, string(wire), "Upstream request failed")
			t.Logf("CLIENT_OK passthrough=%t status=%d completed=true", passthrough, resp.StatusCode)
		})
	}
}

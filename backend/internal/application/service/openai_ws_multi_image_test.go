package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIWSMultiImageFirstTurnHTTPBridge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, mode := range []string{OpenAIWSIngressModeCtxPool, OpenAIWSIngressModePassthrough} {
		t.Run(mode, func(t *testing.T) {
			cfg := &config.Config{}
			cfg.Gateway.OpenAIWS = config.GatewayOpenAIWSConfig{
				Enabled: true, APIKeyEnabled: true, ResponsesWebsocketsV2: true,
				ModeRouterV2Enabled: true, IngressModeDefault: mode,
				HTTPBridgeEnabled: true, HTTPBridgeThresholdBytes: 15 * 1024 * 1024,
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body:       io.NopCloser(strings.NewReader("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_images\",\"status\":\"completed\",\"output\":[],\"usage\":{\"input_tokens\":12,\"output_tokens\":1}}}\n\n")),
			}}
			dialer := &openAIWSRejectDialer{statusCode: http.StatusBadGateway}
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(dialer)
			defer pool.Close()
			svc := &OpenAIGatewayService{cfg: cfg, httpUpstream: upstream, openaiWSPool: pool, openaiWSPassthroughDialer: dialer}
			account := &Account{ID: 22, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Concurrency: 1,
				Credentials: map[string]any{"api_key": "sk-fixture"}, Extra: map[string]any{"openai_apikey_responses_websockets_v2_enabled": true}}
			const imageURL = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII="
			payload := []byte(`{"type":"response.create","model":"gpt-5.6-sol","store":false,"input":[{"role":"user","content":[{"type":"input_text","text":"compare images"},{"type":"input_image","image_url":"` + imageURL + `"},{"type":"input_image","image_url":"` + imageURL + `"}]}],"tools":[{"type":"web_search"}]}`)
			var beforeTurns, afterTurns atomic.Int32
			resultCh := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := coderws.Accept(w, r, nil)
				if err != nil {
					resultCh <- err
					return
				}
				defer func() { _ = conn.CloseNow() }()
				_, first, err := conn.Read(r.Context())
				if err != nil {
					resultCh <- err
					return
				}
				c, _ := gin.CreateTestContext(httptest.NewRecorder())
				c.Request = r.Clone(r.Context())
				resultCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), c, conn, account, "sk-fixture", first, &OpenAIWSIngressHooks{
					BeforeTurn: func(int) error { beforeTurns.Add(1); return nil },
					AfterTurn:  func(int, *OpenAIForwardResult, error) { afterTurns.Add(1) },
				})
			}))
			defer server.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			conn, _, err := coderws.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, err)
			defer func() { _ = conn.CloseNow() }()
			require.NoError(t, conn.Write(ctx, coderws.MessageText, payload))
			_, event, err := conn.Read(ctx)
			require.NoError(t, err)
			require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
			require.NoError(t, conn.Close(coderws.StatusNormalClosure, "done"))
			require.NoError(t, <-resultCh)
			require.Zero(t, dialer.dialCount.Load(), "inline images must not open an upstream WS")
			require.Equal(t, int32(1), beforeTurns.Load())
			require.Equal(t, int32(1), afterTurns.Load())
			require.Equal(t, int64(3), gjson.GetBytes(upstream.lastBody, "input.0.content.#").Int())
			require.Equal(t, imageURL, gjson.GetBytes(upstream.lastBody, "input.0.content.1.image_url").String())
			require.Equal(t, imageURL, gjson.GetBytes(upstream.lastBody, "input.0.content.2.image_url").String())
			require.Equal(t, "web_search", gjson.GetBytes(upstream.lastBody, "tools.0.type").String())
		})
	}
}

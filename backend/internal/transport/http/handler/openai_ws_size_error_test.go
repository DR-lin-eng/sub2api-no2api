//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type openAIWSSizeErrorHTTPUpstream struct {
	service.HTTPUpstream
	client *http.Client
}

func (u *openAIWSSizeErrorHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return u.client.Do(req)
}

func TestOpenAIResponsesWSMessageTooBig(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name      string
		stream    bool
		keepalive bool
		forceHTTP bool
	}{
		{name: "json"},
		{name: "stream", stream: true},
		{name: "stream after heartbeat", stream: true, keepalive: true},
		{name: "HTTP JSON control", forceHTTP: true},
		{name: "HTTP stream control", stream: true, forceHTTP: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var wsCalls, httpCalls atomic.Int32
			upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if websocket.IsWebSocketUpgrade(r) {
					wsCalls.Add(1)
					conn, err := upgrader.Upgrade(w, r, nil)
					if err != nil {
						t.Errorf("upgrade: %v", err)
						return
					}
					defer func() { _ = conn.Close() }()
					var frame map[string]any
					if err := conn.ReadJSON(&frame); err != nil {
						t.Errorf("read prewarm: %v", err)
						return
					}
					if generate, ok := frame["generate"].(bool); !ok || generate {
						t.Error("expected generate=false prewarm before business request")
						return
					}
					if err := conn.WriteJSON(map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp_prewarm", "status": "completed", "output": []any{}}}); err != nil {
						t.Errorf("write prewarm: %v", err)
						return
					}
					if _, _, err := conn.ReadMessage(); err != nil {
						t.Errorf("read business request: %v", err)
						return
					}
					_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseMessageTooBig, ""), time.Now().Add(time.Second))
					return
				}
				httpCalls.Add(1)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"HTTP control succeeded\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_http\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"HTTP control succeeded\"}]}],\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")
			}))
			defer upstream.Close()
			h := newOpenAIResponsesFailoverTestHandler(t, &openAIWSSizeErrorHTTPUpstream{client: upstream.Client()}, func(accounts *[]service.Account, cfg *config.Config) {
				*accounts = (*accounts)[:1]
				cfg.Security.URLAllowlist.Enabled = false
				cfg.Security.URLAllowlist.AllowInsecureHTTP = true
				cfg.Gateway.OpenAIWS.Enabled = true
				cfg.Gateway.OpenAIWS.OAuthEnabled = true
				cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
				(*accounts)[0].Extra = map[string]any{
					service.CodexPrewarmContinuationExtraKey: true,
					"openai_ws_force_http":                   tc.forceHTTP,
					"custom_base_url_enabled":                true,
					"custom_base_url":                        upstream.URL,
				}
			})
			heartbeatHeadersSeen := make(chan struct{})
			router := gin.New()
			router.POST("/v1/responses", func(c *gin.Context) {
				fixture, _ := newOpenAIResponsesFailoverTestContext(t, c.Request.Context())
				for key, value := range fixture.Keys {
					c.Set(key, value)
				}
				if tc.keepalive {
					body, err := io.ReadAll(c.Request.Body)
					if err != nil {
						t.Errorf("read request body: %v", err)
						return
					}
					c.Request.Body = io.NopCloser(strings.NewReader(string(body)))
					stop := service.StartOpenAIResponsesSSEKeepalive(c, time.Millisecond)
					defer stop()
					select {
					case <-heartbeatHeadersSeen:
					case <-c.Request.Context().Done():
						return
					}
				}
				h.Responses(c)
			})
			gateway := httptest.NewServer(router)
			defer gateway.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			input := fmt.Sprintf(`{"model":"gpt-5.6-sol","stream":%v,"store":false,"input":"hello","instructions":"fixture"}`, tc.stream)
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, gateway.URL+"/v1/responses", strings.NewReader(input))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			resp, err := gateway.Client().Do(req)
			require.NoError(t, err)
			if tc.keepalive {
				close(heartbeatHeadersSeen)
			}
			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			require.NoError(t, err)
			if tc.forceHTTP {
				require.Equal(t, http.StatusOK, resp.StatusCode)
				require.Contains(t, string(body), "HTTP control succeeded")
				require.Zero(t, wsCalls.Load())
				require.Equal(t, int32(1), httpCalls.Load())
				return
			}
			require.Equal(t, int32(1), wsCalls.Load())
			require.Zero(t, httpCalls.Load(), "size rejection must not replay the request")
			require.Contains(t, string(body), "message size limit exceeded (1009)")
			require.NotContains(t, string(body), "Upstream request failed")
			require.NotContains(t, string(body), "token-1")
			if tc.keepalive {
				require.Equal(t, http.StatusOK, resp.StatusCode)
				require.Equal(t, 1, strings.Count(string(body), "event: response.failed"))
				for _, line := range strings.Split(string(body), "\n") {
					if strings.HasPrefix(line, "data: ") {
						var event map[string]any
						require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event))
						require.Equal(t, "response.failed", event["type"])
					}
				}
			} else {
				require.Equal(t, http.StatusBadGateway, resp.StatusCode)
				require.True(t, gjson.ValidBytes(body), "JSON must not have a second SSE error appended")
				require.Equal(t, "upstream_error", gjson.GetBytes(body, "error.type").String())
			}
		})
	}
}

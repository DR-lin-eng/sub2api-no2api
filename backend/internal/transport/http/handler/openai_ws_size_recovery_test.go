//go:build unit

package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIResponsesWSSizeHTTPRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		name, text  string
		stream      bool
		keepalive   bool
		prewarmFail bool
		pairedTools bool
		hostedTool  bool
		store       bool
		incremental bool
		afterOutput bool
		closeCode   int
		httpStatus  int
		wantWS      int
		wantHTTP    int
		wantStatus  int
	}{
		{name: "JSON recovery", wantWS: 1, wantHTTP: 1, wantStatus: 200},
		{name: "stream recovery", stream: true, wantWS: 1, wantHTTP: 1, wantStatus: 200},
		{name: "recovery after heartbeat", stream: true, keepalive: true, wantWS: 1, wantHTTP: 1, wantStatus: 200},
		{name: "prewarm itself rejected", prewarmFail: true, wantWS: 1, wantHTTP: 1, wantStatus: 200},
		{name: "unexpected close after bounded reconnects", closeCode: websocket.CloseAbnormalClosure, wantWS: 6, wantHTTP: 1, wantStatus: 200},
		{name: "internal close after bounded reconnects", closeCode: websocket.CloseInternalServerErr, stream: true, wantWS: 6, wantHTTP: 1, wantStatus: 200},
		{name: "2MiB context", text: strings.Repeat("context ", 262144), wantWS: 1, wantHTTP: 1, wantStatus: 200},
		{name: "large Unicode context", text: strings.Repeat("上下文", 262144), stream: true, wantWS: 1, wantHTTP: 1, wantStatus: 200},
		{name: "16MiB preflight", text: strings.Repeat("x", 16*1024*1024), wantWS: 0, wantHTTP: 1, wantStatus: 200},
		{name: "paired tool history", pairedTools: true, text: strings.Repeat("tool result ", 131072), wantWS: 1, wantHTTP: 1, wantStatus: 200},
		{name: "HTTP failure stays bounded", httpStatus: 502, wantWS: 1, wantHTTP: 1, wantStatus: 502},
		{name: "context window error stays 400", httpStatus: 400, wantWS: 1, wantHTTP: 1, wantStatus: 400},
		{name: "hosted tools are not replayed", hostedTool: true, wantWS: 1, wantHTTP: 0, wantStatus: 502},
		{name: "explicit store is not replayed", store: true, wantWS: 1, wantHTTP: 0, wantStatus: 502},
		{name: "incremental state is not replayed", incremental: true, wantWS: 0, wantHTTP: 0, wantStatus: 400},
		{name: "delivered output is not replayed", stream: true, afterOutput: true, wantWS: 1, wantHTTP: 0, wantStatus: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := tc.text
			if text == "" {
				text = "original user history"
			}
			input := []map[string]any{{"type": "message", "role": "user", "content": text}}
			if tc.pairedTools {
				input = append(input,
					map[string]any{"type": "function_call", "call_id": "call_read", "name": "read", "arguments": `{"offset":9007199254740993}`},
					map[string]any{"type": "function_call_output", "call_id": "call_read", "output": text},
				)
			}
			request := map[string]any{"model": "gpt-5.6-sol", "stream": tc.stream, "store": tc.store, "input": input, "instructions": "recovery fixture"}
			if tc.pairedTools {
				request["tools"] = []map[string]any{{"type": "function", "name": "read", "parameters": map[string]any{"type": "object"}}}
			}
			if tc.hostedTool {
				request["tools"] = []map[string]any{{"type": "mcp", "server_label": "fixture", "server_url": "https://example.invalid/mcp"}}
			}
			if tc.incremental {
				request["previous_response_id"] = "resp_client_owned"
			}
			payload, err := json.Marshal(request)
			require.NoError(t, err)
			var mu sync.Mutex
			wsCalls, httpCalls := 0, 0
			var httpBody []byte
			upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if websocket.IsWebSocketUpgrade(r) {
					mu.Lock()
					wsCalls++
					mu.Unlock()
					conn, upgradeErr := upgrader.Upgrade(w, r, nil)
					if upgradeErr != nil {
						t.Errorf("upgrade: %v", upgradeErr)
						return
					}
					defer func() { _ = conn.Close() }()
					_, frame, readErr := conn.ReadMessage()
					if readErr != nil {
						t.Errorf("read WS request: %v", readErr)
						return
					}
					if generate := gjson.GetBytes(frame, "generate"); generate.Exists() && !generate.Bool() && !tc.prewarmFail {
						_ = conn.WriteJSON(map[string]any{"type": "response.completed", "response": map[string]any{"id": "resp_connection_only_prewarm", "status": "completed", "output": []any{}}})
						if _, _, readErr = conn.ReadMessage(); readErr != nil {
							t.Errorf("read WS business request: %v", readErr)
							return
						}
					}
					if tc.afterOutput {
						_ = conn.WriteJSON(map[string]any{"type": "response.output_text.delta", "delta": "already delivered"})
					}
					code := tc.closeCode
					if code == 0 {
						code = websocket.CloseMessageTooBig
					}
					if code != websocket.CloseAbnormalClosure {
						_ = conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, ""), time.Now().Add(time.Second))
					}
					return
				}
				body, readErr := io.ReadAll(r.Body)
				if readErr == nil && r.Header.Get("Content-Encoding") == "zstd" {
					decoder, decodeErr := zstd.NewReader(nil)
					if decodeErr != nil {
						t.Errorf("zstd reader: %v", decodeErr)
						return
					}
					body, readErr = decoder.DecodeAll(body, nil)
					decoder.Close()
				}
				if readErr != nil {
					t.Errorf("read HTTP request: %v", readErr)
					return
				}
				mu.Lock()
				httpCalls++
				httpBody = body
				mu.Unlock()
				if r.Header.Get("Authorization") != "Bearer token-1" {
					t.Error("HTTP recovery must use the selected account")
				}
				if tc.httpStatus >= 400 {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(tc.httpStatus)
					if tc.httpStatus == 400 {
						_, _ = io.WriteString(w, `{"error":{"type":"invalid_request_error","code":"context_length_exceeded","message":"input exceeds the context window"}}`)
					} else {
						_, _ = io.WriteString(w, `{"error":{"type":"server_error","message":"HTTP upstream unavailable"}}`)
					}
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"recovered\"}\n\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_recovered\",\"created_at\":1700000000,\"status\":\"completed\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"recovered\"}]}],\"usage\":{\"input_tokens\":8,\"output_tokens\":2}}}\n\n")
			}))
			defer upstream.Close()
			h := newOpenAIResponsesFailoverTestHandler(t, &openAIWSSizeErrorHTTPUpstream{client: upstream.Client()}, func(accounts *[]service.Account, cfg *config.Config) {
				*accounts = (*accounts)[:1]
				cfg.Security.URLAllowlist.Enabled = false
				cfg.Security.URLAllowlist.AllowInsecureHTTP = true
				cfg.Gateway.OpenAIWS.Enabled = true
				cfg.Gateway.OpenAIWS.OAuthEnabled = true
				cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
				cfg.Gateway.OpenAIWS.EventFlushBatchSize = 1
				cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
				cfg.Gateway.OpenAIWS.HTTPBridgeThresholdBytes = 15 * 1024 * 1024
				(*accounts)[0].Extra = map[string]any{service.CodexPrewarmContinuationExtraKey: true, "custom_base_url_enabled": true, "custom_base_url": upstream.URL}
			})
			heartbeatHeadersSeen := make(chan struct{})
			router := gin.New()
			router.POST("/v1/responses", func(c *gin.Context) {
				fixture, _ := newOpenAIResponsesFailoverTestContext(t, c.Request.Context())
				for key, value := range fixture.Keys {
					c.Set(key, value)
				}
				if tc.keepalive {
					body, readErr := io.ReadAll(c.Request.Body)
					if readErr != nil {
						t.Errorf("read client body: %v", readErr)
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
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, gateway.URL+"/v1/responses", strings.NewReader(string(payload)))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			resp, err := gateway.Client().Do(req)
			require.NoError(t, err)
			if tc.keepalive {
				close(heartbeatHeadersSeen)
			}
			responseBody, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			require.NoError(t, err)
			mu.Lock()
			actualWS, actualHTTP := wsCalls, httpCalls
			sentBody := httpBody
			mu.Unlock()
			t.Logf("request_bytes=%d ws_attempts=%d http_attempts=%d status=%d response_bytes=%d", len(payload), actualWS, actualHTTP, resp.StatusCode, len(responseBody))
			require.Equal(t, tc.wantWS, actualWS)
			require.Equal(t, tc.wantHTTP, actualHTTP)
			require.Equal(t, tc.wantStatus, resp.StatusCode)
			if tc.wantHTTP > 0 {
				require.True(t, json.Valid(sentBody))
				require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(sentBody, "model").String())
				require.False(t, gjson.GetBytes(sentBody, "previous_response_id").Exists(), "prewarm ID must never cross to HTTP")
				require.False(t, gjson.GetBytes(sentBody, "type").Exists())
				require.Equal(t, "user", gjson.GetBytes(sentBody, "input.0.role").String(), "prewarm must not mutate the replayed user role")
				require.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(text))), fmt.Sprintf("%x", sha256.Sum256([]byte(gjson.GetBytes(sentBody, "input.0.content").String()))))
				if tc.pairedTools {
					require.NotEmpty(t, gjson.GetBytes(sentBody, "input.1.call_id").String())
					require.Equal(t, gjson.GetBytes(sentBody, "input.1.call_id").String(), gjson.GetBytes(sentBody, "input.2.call_id").String())
					require.Equal(t, fmt.Sprintf("%x", sha256.Sum256([]byte(text))), fmt.Sprintf("%x", sha256.Sum256([]byte(gjson.GetBytes(sentBody, "input.2.output").String()))))
					require.Contains(t, gjson.GetBytes(sentBody, "input.1.arguments").String(), "9007199254740993")
				}
			}
			if tc.wantStatus == 200 && tc.wantHTTP == 1 {
				require.Contains(t, string(responseBody), "recovered")
				require.NotContains(t, string(responseBody), "response.failed")
				require.NotContains(t, string(responseBody), "1009")
			} else if tc.httpStatus == 400 {
				require.Contains(t, string(responseBody), "context_length_exceeded")
			} else if tc.afterOutput {
				require.Contains(t, string(responseBody), "already delivered")
			} else if tc.wantHTTP == 0 && !tc.incremental {
				require.Contains(t, string(responseBody), "1009")
			}
		})
	}
}

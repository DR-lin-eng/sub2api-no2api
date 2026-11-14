package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestForwardCodexPrewarmNonStreamRecoversTransientWSThroughHTTP(t *testing.T) {
	for _, stage := range []string{"handshake", "prewarm", "business"} {
		t.Run(stage, func(t *testing.T) {
			events := []string{`{"type":"error","error":{"type":"server_error","code":"server_error","message":"temporarily unavailable"}}`}
			if stage == "business" {
				events = append([]string{`{"type":"response.completed","response":{"id":"resp_warm"}}`}, events...)
			}
			svc, account, c, rec, conn := newCodexPrewarmNonStreamFixture(t, events...)
			svc.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
			if stage == "handshake" {
				svc.openaiWSPool.setClientDialerForTest(&openAIWSRejectDialer{
					statusCode: http.StatusBadGateway, responseBody: []byte(`{"error":{"type":"server_error","message":"temporarily unavailable"}}`),
					err: errors.New("temporary WS upgrade failure"),
				})
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"req_http_recovered"}},
				Body:       io.NopCloser(strings.NewReader("data: " + `{"type":"response.completed","response":{"id":"resp_http_recovered","object":"response","model":"gpt-5.4","status":"completed","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"recovered"}]}],"usage":{"input_tokens":4,"output_tokens":2}}}` + "\n\n")),
			}}
			svc.httpUpstream = upstream
			body := []byte(`{"model":"gpt-5.4","instructions":"keep","stream":false,"input":[{"role":"user","content":"hello"}]}`)
			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.False(t, result.OpenAIWSMode)
			require.False(t, result.Stream)
			require.Equal(t, 4, result.Usage.InputTokens)
			require.Equal(t, 2, result.Usage.OutputTokens)
			require.NotNil(t, upstream.lastReq)
			require.True(t, gjson.GetBytes(upstream.lastBody, "stream").Bool())
			require.Equal(t, "user", gjson.GetBytes(upstream.lastBody, "input.0.role").String())
			require.False(t, gjson.GetBytes(upstream.lastBody, "previous_response_id").Exists(), "HTTP recovery must discard the connection-local prewarm anchor")
			require.Equal(t, http.StatusOK, rec.Code)
			require.True(t, gjson.ValidBytes(rec.Body.Bytes()))
			require.Equal(t, "resp_http_recovered", gjson.GetBytes(rec.Body.Bytes(), "id").String())
			require.NotContains(t, rec.Body.String(), "temporarily unavailable")
			if stage != "handshake" {
				require.True(t, conn.closed)
			}
		})
	}
}

func TestForwardCodexPrewarmNonStreamIntermittentFailureOverRealWS(t *testing.T) {
	var connections atomic.Int32
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Error(err)
			return
		}
		var warmup map[string]any
		if err := conn.ReadJSON(&warmup); err != nil {
			t.Error(err)
			return
		}
		if warmup["stream"] != true || warmup["generate"] != false {
			t.Error("prewarm must use stream=true and generate=false")
			return
		}
		if connections.Add(1) == 1 {
			_ = conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"error","error":{"type":"server_error","code":"server_error","message":"temporary prewarm failure"}}`))
			return
		}
		if err := conn.WriteMessage(websocket.TextMessage, []byte(`{"type":"response.completed","response":{"id":"resp_warm"}}`)); err != nil {
			t.Error(err)
			return
		}
		var business map[string]any
		if err := conn.ReadJSON(&business); err != nil {
			t.Error(err)
			return
		}
		if business["stream"] != true || business["previous_response_id"] != "resp_warm" {
			t.Error("business request must continue the prewarm on the same connection")
			return
		}
		for _, event := range []string{
			`{"type":"response.output_item.done","output_index":0,"item":{"id":"msg_ws","type":"message","role":"assistant","content":[{"type":"output_text","text":"WS healthy"}]}}`,
			`{"type":"response.completed","response":{"id":"resp_ws","object":"response","status":"completed","output":[],"usage":{"input_tokens":2,"output_tokens":1}}}`,
		} {
			if err := conn.WriteMessage(websocket.TextMessage, []byte(event)); err != nil {
				t.Error(err)
				return
			}
		}
	}))
	defer server.Close()

	svc, account, _, _, _ := newCodexPrewarmNonStreamFixture(t)
	svc.cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	svc.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = true
	svc.cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	pool := newOpenAIWSConnPool(svc.cfg)
	t.Cleanup(pool.Close)
	svc.openaiWSPool = pool
	account.Extra["custom_base_url_enabled"] = true
	account.Extra["custom_base_url"] = server.URL + "/backend-api/codex"
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
		Body: io.NopCloser(strings.NewReader(`{"id":"resp_http","object":"response","status":"completed","output":[],"usage":{"input_tokens":1,"output_tokens":1}}`)),
	}}
	svc.httpUpstream = upstream
	for attempt := 0; attempt < 2; attempt++ {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
		SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
		result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.4","stream":false,"input":"hello"}`))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, rec.Code)
		require.True(t, gjson.ValidBytes(rec.Body.Bytes()))
		require.Equal(t, attempt == 1, result.OpenAIWSMode)
		if attempt == 1 {
			require.Equal(t, "WS healthy", gjson.GetBytes(rec.Body.Bytes(), "output.0.content.0.text").String())
		}
	}
	require.Equal(t, int32(2), connections.Load(), "the next request should return to WS after a transient failure")
	require.Len(t, upstream.requests, 1, "the failed request gets one HTTP recovery")
}

func TestCodexPrewarmNonStreamTransientHTTPRecoveryGuards(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
		blocked    bool
	}{
		{name: "non-stream text", body: `{"stream":false,"input":"full history"}`, status: 502},
		{name: "omitted stream", body: `{"input":"full history"}`, status: 503},
		{name: "stream request", body: `{"stream":true,"input":"full history"}`, status: 502, blocked: true},
		{name: "quota", body: `{"input":"full history"}`, status: 429, blocked: true},
		{name: "authentication", body: `{"input":"full history"}`, status: 401, blocked: true},
		{name: "validation", body: `{"input":"full history"}`, status: 400, blocked: true},
		{name: "hosted tools", body: `{"input":"full history","tools":[{"type":"web_search"}]}`, status: 502, blocked: true},
		{name: "incremental", body: `{"input":"continue","previous_response_id":"resp_previous"}`, status: 502, blocked: true},
		{name: "stored response", body: `{"input":"full history","store":true}`, status: 502, blocked: true},
		{name: "output written", body: `{"input":"full history"}`, status: 502, blocked: true},
		{name: "cancelled", body: `{"input":"full history"}`, status: 502, blocked: true},
		{name: "bridge disabled", body: `{"input":"full history"}`, status: 502, blocked: true},
		{name: "prewarm disabled", body: `{"input":"full history"}`, status: 502, blocked: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, account, c, _, _ := newCodexPrewarmNonStreamFixture(t)
			svc.cfg.Gateway.OpenAIWS.HTTPBridgeEnabled = tc.name != "bridge disabled"
			if tc.name == "prewarm disabled" {
				account.Extra[CodexPrewarmContinuationExtraKey] = false
			}
			if tc.name == "output written" {
				_, err := c.Writer.WriteString("data: semantic output\n\n")
				require.NoError(t, err)
			}
			ctx := context.Background()
			if tc.name == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			err := newOpenAIWSPreOutputFailoverError(tc.status, nil, nil, "fixture failure")
			require.Equal(t, !tc.blocked, svc.prepareOpenAIWSHTTPFallback(ctx, c, account, err, []byte(tc.body), []byte(tc.body)))
		})
	}
}

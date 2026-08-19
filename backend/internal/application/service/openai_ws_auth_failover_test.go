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
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNewOpenAIWSDialAuthFailoverSanitizesTransportDetails(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 801, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	wsErr := wrapOpenAIWSFallback("auth_failed", &openAIWSDialError{
		StatusCode:      http.StatusUnauthorized,
		ResponseHeaders: http.Header{"X-Request-Id": []string{"rid-ws-401"}},
		ResponseBody:    []byte(`{"error":{"message":"token expired"}}`),
		Err:             errors.New("failed to WebSocket dial: expected handshake response status code 101 but got 401"),
	})

	failoverErr := svc.newOpenAIWSDialAuthFailover(context.Background(), account, "gpt-5.6-luna", wsErr)
	require.NotNil(t, failoverErr)
	require.Equal(t, http.StatusUnauthorized, failoverErr.StatusCode)
	require.Equal(t, GatewayFailureScopeAccount, failoverErr.Scope)
	require.True(t, failoverErr.ShouldRetryNextAccount())
	require.Equal(t, "rid-ws-401", failoverErr.ResponseHeaders.Get("X-Request-Id"))
	require.Contains(t, string(failoverErr.ResponseBody), openAIWSAuthenticationFailureClientMessage)
	require.NotContains(t, string(failoverErr.ResponseBody), "expected handshake response")
	require.NotContains(t, string(failoverErr.ResponseBody), "token expired")

	statusFromMessage := svc.newOpenAIWSDialAuthFailover(
		context.Background(),
		account,
		"gpt-5.6-luna",
		&openAIWSDialError{Err: errors.New("failed to WebSocket dial: expected handshake response status code 101 but got 401")},
	)
	require.NotNil(t, statusFromMessage)
	require.Equal(t, http.StatusUnauthorized, statusFromMessage.StatusCode)
}

func TestForwardWSDial401SilentlyFallsBackToHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.146.0")

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	rejectDialer := &openAIWSRejectDialer{
		statusCode:      http.StatusUnauthorized,
		responseHeaders: http.Header{"X-Request-Id": []string{"rid-forward-ws-401"}},
		responseBody:    []byte(`{"error":{"message":"expired bearer token"}}`),
		err:             errors.New("failed to WebSocket dial: expected handshake response status code 101 but got 401"),
	}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(rejectDialer)
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
		Body: io.NopCloser(strings.NewReader(strings.Join([]string{
			`data: {"type":"response.output_text.delta","response_id":"resp_http_401","delta":"ok"}`,
			"",
			`data: {"type":"response.completed","response":{"id":"resp_http_401","model":"gpt-5.6-luna","usage":{"input_tokens":1,"output_tokens":1}}}`,
			"",
		}, "\n"))),
	}}
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     upstream,
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		openaiWSPool:     pool,
		toolCorrector:    NewCodexToolCorrector(),
	}
	account := &Account{
		ID: 802, Name: "oauth-ws-401", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "expired", "chatgpt_account_id": "chatgpt-401"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
	}
	// A keepalive may commit HTTP 200 before the upstream WS handshake returns.
	// It is not semantic output and must not make the 401 visible to the client.
	keepaliveBytes, keepaliveErr := c.Writer.Write([]byte(":\n\n"))
	require.NoError(t, keepaliveErr)
	recordOpenAIStreamKeepaliveBytes(c, keepaliveBytes)
	require.True(t, c.Writer.Written())
	require.False(t, openAIStreamClientOutputStarted(c, false))

	result, err := svc.Forward(
		context.Background(), c, account,
		[]byte(`{"model":"gpt-5.6-luna","stream":true,"input":"hello"}`),
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.False(t, result.OpenAIWSMode)
	require.Equal(t, int32(1), rejectDialer.dialCount.Load())
	require.NotNil(t, upstream.lastReq)
	require.Contains(t, recorder.Body.String(), "response.completed")
	require.NotContains(t, recorder.Body.String(), "expected handshake response")
	require.NotContains(t, recorder.Body.String(), "upstream_error")
}

func TestIngressWSDial401SilentlyFallsBackToHTTPBridge(t *testing.T) {
	for _, mode := range []string{OpenAIWSIngressModeCtxPool, OpenAIWSIngressModePassthrough} {
		t.Run(mode, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			cfg := &config.Config{}
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Security.URLAllowlist.AllowInsecureHTTP = true
			cfg.Gateway.OpenAIWS.Enabled = true
			cfg.Gateway.OpenAIWS.OAuthEnabled = true
			cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
			cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
			cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
			cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
			cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3

			rejectDialer := &openAIWSRejectDialer{
				statusCode: http.StatusUnauthorized,
				err:        errors.New("failed to WebSocket dial: expected handshake response status code 101 but got 401"),
			}
			upstream := &httpUpstreamRecorder{resp: &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
				Body: io.NopCloser(strings.NewReader(
					"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_ingress_http_401\",\"model\":\"gpt-5.6-luna\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n",
				)),
			}}
			svc := &OpenAIGatewayService{
				cfg: cfg, httpUpstream: upstream, cache: &stubGatewayCache{},
				openaiWSResolver: NewOpenAIWSProtocolResolver(cfg), toolCorrector: NewCodexToolCorrector(),
			}
			if mode == OpenAIWSIngressModePassthrough {
				svc.openaiWSPassthroughDialer = rejectDialer
			} else {
				pool := newOpenAIWSConnPool(cfg)
				pool.setClientDialerForTest(rejectDialer)
				t.Cleanup(pool.Close)
				svc.openaiWSPool = pool
			}
			account := &Account{
				ID: 803, Name: "ingress-ws-401", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Status: StatusActive, Schedulable: true, Concurrency: 1,
				Credentials: map[string]any{"access_token": "oauth", "chatgpt_account_id": "chatgpt-ingress-401"},
				Extra: map[string]any{
					"responses_websockets_v2_enabled":           true,
					"openai_oauth_responses_websockets_v2_mode": mode,
				},
			}

			serverErr := make(chan error, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, acceptErr := coderws.Accept(w, r, nil)
				if acceptErr != nil {
					serverErr <- acceptErr
					return
				}
				defer func() { _ = conn.CloseNow() }()
				_, firstMessage, readErr := conn.Read(r.Context())
				if readErr != nil {
					serverErr <- readErr
					return
				}
				ginCtx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ginCtx.Request = r.Clone(r.Context())
				serverErr <- svc.ProxyResponsesWebSocketFromClient(
					r.Context(), ginCtx, conn, account, "oauth", firstMessage, nil,
				)
			}))
			defer server.Close()

			client, _, dialErr := coderws.Dial(context.Background(), "ws"+strings.TrimPrefix(server.URL, "http"), nil)
			require.NoError(t, dialErr)
			defer func() { _ = client.CloseNow() }()
			require.NoError(t, client.Write(
				context.Background(), coderws.MessageText,
				[]byte(`{"type":"response.create","model":"gpt-5.6-luna","stream":true,"input":"hello"}`),
			))
			readCtx, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
			_, event, readErr := client.Read(readCtx)
			cancelRead()
			require.NoError(t, readErr)
			require.Equal(t, "response.completed", gjson.GetBytes(event, "type").String())
			require.NotContains(t, string(event), "upstream_error")
			require.NoError(t, client.Close(coderws.StatusNormalClosure, "done"))

			select {
			case err := <-serverErr:
				require.NoError(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("silent 401 HTTP bridge fallback did not finish")
			}
			require.Equal(t, int32(1), rejectDialer.dialCount.Load())
			require.NotNil(t, upstream.lastReq)
		})
	}
}

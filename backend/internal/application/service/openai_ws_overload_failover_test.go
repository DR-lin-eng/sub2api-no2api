package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestForwardOpenAIWSV2OverloadFailsOverBeforeAnyDownstreamOutput(t *testing.T) {
	tests := []struct {
		name   string
		events [][]byte
	}{
		{
			name:   "typed_error",
			events: [][]byte{[]byte(`{"type":"error","error":{"type":"service_unavailable_error","code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`)},
		},
		{
			name:   "message_only_error",
			events: [][]byte{[]byte(`{"type":"error","error":{"type":"service_unavailable_error","message":"Our servers are currently overloaded. Please try again later."}}`)},
		},
		{
			name: "response_failed_after_preamble",
			events: [][]byte{
				[]byte(`{"type":"response.created","response":{"id":"resp-overload"}}`),
				[]byte(`{"type":"response.failed","response":{"id":"resp-overload","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}}`),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

			cfg := newOpenAIWSV2TestConfig()
			cfg.Security.URLAllowlist.Enabled = false
			cfg.Security.URLAllowlist.AllowInsecureHTTP = true
			cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
			cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
			cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
			cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
			conn := &openAIWSCaptureConn{events: tt.events}
			dialer := &openAIWSCaptureDialer{conn: conn, handshake: http.Header{"Retry-After": []string{"2"}}}
			pool := newOpenAIWSConnPool(cfg)
			pool.setClientDialerForTest(dialer)
			svc := &OpenAIGatewayService{
				cfg:              cfg,
				cache:            &stubGatewayCache{},
				openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
				toolCorrector:    NewCodexToolCorrector(),
				openaiWSPool:     pool,
			}
			account := &Account{
				ID: 701, Name: "ws-overload", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 1,
				Credentials: map[string]any{"api_key": "sk-test"},
			}

			result, err := svc.forwardOpenAIWSV2(
				context.Background(), c, account,
				map[string]any{"model": "gpt-5.5", "input": "hello", "stream": true},
				"sk-test",
				OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
				false, true, "gpt-5.5", "gpt-5.5", time.Now(), 1, "", nil, nil,
			)

			require.Nil(t, result)
			var failoverErr *UpstreamFailoverError
			require.True(t, errors.As(err, &failoverErr), "expected pre-output overload failover, got %v", err)
			require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
			require.Equal(t, tt.events[len(tt.events)-1], failoverErr.ResponseBody)
			require.Equal(t, "2", failoverErr.ResponseHeaders.Get("Retry-After"))
			require.Equal(t, GatewayFailureScopeRequest, failoverErr.Scope)
			require.Empty(t, recorder.Body.String())
		})
	}
}

func TestForwardOpenAIWSV2OverloadAfterSemanticOutputIsNotReplayed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	cfg := newOpenAIWSV2TestConfig()
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	conn := &openAIWSCaptureConn{events: [][]byte{
		[]byte(`{"type":"response.output_text.delta","response_id":"resp-started","delta":"started"}`),
		[]byte(`{"type":"error","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}`),
	}}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: conn})
	svc := &OpenAIGatewayService{
		cfg: cfg, cache: &stubGatewayCache{}, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool,
	}
	account := &Account{
		ID: 702, Name: "ws-overload-after-output", Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
	}

	result, err := svc.forwardOpenAIWSV2(
		context.Background(), c, account,
		map[string]any{"model": "gpt-5.5", "input": "hello", "stream": true},
		"sk-test", OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2},
		false, true, "gpt-5.5", "gpt-5.5", time.Now(), 1, "", nil, nil,
	)

	require.Nil(t, result)
	require.Error(t, err)
	var failoverErr *UpstreamFailoverError
	require.False(t, errors.As(err, &failoverErr), "semantic output must make the attempt non-replayable")
	require.Contains(t, recorder.Body.String(), "started")
	require.Contains(t, recorder.Body.String(), "server_is_overloaded")
}

func TestOpenAIWSIngressOverloadAfterCreatedStaysReplayable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := newOpenAIWSV2TestConfig()
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModeCtxPool
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	upstream := &openAIWSCaptureConn{events: [][]byte{
		[]byte(`{"type":"response.created","response":{"id":"resp-ingress-overload"}}`),
		[]byte(`{"type":"response.failed","response":{"id":"resp-ingress-overload","error":{"code":"server_is_overloaded","message":"Our servers are currently overloaded. Please try again later."}}}`),
	}}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: upstream})
	svc := &OpenAIGatewayService{
		cfg: cfg, cache: &stubGatewayCache{}, openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector: NewCodexToolCorrector(), openaiWSPool: pool,
	}
	account := &Account{
		ID: 703, Name: "ws-ingress-overload", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-ingress"},
	}

	serverErr := make(chan error, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			serverErr <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()
		msgType, firstMessage, readErr := ReadOpenAIWSClientMessage(
			r.Context(), conn, 3*time.Second, coderws.StatusPolicyViolation, "missing response.create",
		)
		if readErr != nil {
			serverErr <- readErr
			return
		}
		if msgType != coderws.MessageText {
			serverErr <- errors.New("first ingress frame was not text")
			return
		}
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = r.Clone(r.Context())
		serverErr <- svc.ProxyResponsesWebSocketFromClient(r.Context(), c, conn, account, "oauth-token", firstMessage, nil)
	}))
	defer server.Close()

	clientCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	client, _, err := coderws.Dial(clientCtx, "ws"+strings.TrimPrefix(server.URL, "http"), nil)
	cancel()
	require.NoError(t, err)
	defer func() { _ = client.CloseNow() }()
	writeCtx, cancelWrite := context.WithTimeout(context.Background(), time.Second)
	require.NoError(t, client.Write(writeCtx, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.5","stream":true,"input":"hello"}`)))
	cancelWrite()

	select {
	case err := <-serverErr:
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, http.StatusServiceUnavailable, failoverErr.StatusCode)
		require.Contains(t, string(failoverErr.ResponseBody), "server_is_overloaded")
	case <-time.After(3 * time.Second):
		t.Fatal("ingress did not return overload failover")
	}
}

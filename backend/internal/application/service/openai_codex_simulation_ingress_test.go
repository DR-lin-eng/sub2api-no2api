package service

import (
	"context"
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

func TestCodexFullSimulationIngressUsesOneAttemptPlanForHandshakeAndBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.ModeRouterV2Enabled = true
	cfg.Gateway.OpenAIWS.IngressModeDefault = OpenAIWSIngressModePassthrough
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 4
	cfg.Gateway.OpenAIWS.DialTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 3
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	cfg.Gateway.CodexSimulation = config.GatewayCodexSimulationConfig{
		FullSimulationEnabled: true,
		IdentitySecret:        codexSimulationTestSecret,
		ContinuationMode:      "enforce",
		StateTTLSeconds:       7 * 24 * 60 * 60,
	}

	captureConn := &openAIWSCaptureConn{events: [][]byte{
		[]byte(`{"type":"response.completed","response":{"id":"resp_full_ingress","model":"gpt-5.4","usage":{"input_tokens":1,"output_tokens":1}}}`),
		[]byte(`{"type":"response.completed","response":{"id":"resp_full_ingress_next","model":"gpt-5.4","usage":{"input_tokens":1,"output_tokens":1}}}`),
	}}
	captureDialer := &openAIWSCaptureDialer{conn: captureConn}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(captureDialer)
	svc := &OpenAIGatewayService{
		cfg:              cfg,
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}
	account := &Account{
		ID:          9101,
		Name:        "codex-full-ingress",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "oauth-token",
			"chatgpt_account_id": "upstream-principal",
		},
		Extra: map[string]any{
			codexFingerprintModeExtraKey:                "full",
			"responses_websockets_v2_enabled":           true,
			"openai_oauth_responses_websockets_v2_mode": OpenAIWSIngressModePassthrough,
		},
	}

	serverErrCh := make(chan error, 1)
	wsServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := coderws.Accept(w, r, nil)
		if err != nil {
			serverErrCh <- err
			return
		}
		defer func() { _ = conn.CloseNow() }()

		recorder := httptest.NewRecorder()
		ginContext, _ := gin.CreateTestContext(recorder)
		request := r.Clone(r.Context())
		request.Header = request.Header.Clone()
		request.Header.Set("User-Agent", "forged-agent/1.0")
		request.Header.Set("Originator", "forged-originator")
		request.Header.Set("Session-Id", "forged-session")
		request.Header.Set("Session_Id", "forged-session-alias")
		request.Header.Set("Thread-Id", "conversation-full-ingress")
		request.Header.Set("X-Client-Request-Id", "forged-request")
		request.Header.Set("X-Codex-Installation-Id", "forged-installation")
		request.Header.Set(CodexProjectIDHeader, "project-ingress")
		ginContext.Request = request

		readContext, cancelRead := context.WithTimeout(r.Context(), 3*time.Second)
		_, firstMessage, readErr := conn.Read(readContext)
		cancelRead()
		if readErr != nil {
			serverErrCh <- readErr
			return
		}
		serverErrCh <- svc.ProxyResponsesWebSocketFromClient(r.Context(), ginContext, conn, account, "oauth-token", firstMessage, nil)
	}))
	defer wsServer.Close()

	dialContext, cancelDial := context.WithTimeout(context.Background(), 3*time.Second)
	clientConn, _, err := coderws.Dial(dialContext, "ws"+strings.TrimPrefix(wsServer.URL, "http"), nil)
	cancelDial()
	require.NoError(t, err)
	defer func() { _ = clientConn.CloseNow() }()

	writeContext, cancelWrite := context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeContext, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.4","stream":false,"prompt_cache_key":"forged-cache","client_metadata":{"preserved":true}}`))
	cancelWrite()
	require.NoError(t, err)
	readContext, cancelRead := context.WithTimeout(context.Background(), 3*time.Second)
	_, event, err := clientConn.Read(readContext)
	cancelRead()
	require.NoError(t, err)
	require.Equal(t, "resp_full_ingress", gjson.GetBytes(event, "response.id").String())

	writeContext, cancelWrite = context.WithTimeout(context.Background(), 3*time.Second)
	err = clientConn.Write(writeContext, coderws.MessageText, []byte(`{"type":"response.create","model":"gpt-5.4","stream":false,"previous_response_id":"resp_full_ingress","input":"next"}`))
	cancelWrite()
	require.NoError(t, err)
	readContext, cancelRead = context.WithTimeout(context.Background(), 3*time.Second)
	_, event, err = clientConn.Read(readContext)
	cancelRead()
	require.NoError(t, err)
	require.Equal(t, "resp_full_ingress_next", gjson.GetBytes(event, "response.id").String())
	require.NoError(t, clientConn.Close(coderws.StatusNormalClosure, "done"))

	select {
	case serverErr := <-serverErrCh:
		require.NoError(t, serverErr)
	case <-time.After(5 * time.Second):
		t.Fatal("waiting for full simulation ingress to finish")
	}

	captureDialer.mu.Lock()
	headers := cloneHeader(captureDialer.lastHeaders)
	captureDialer.mu.Unlock()
	require.Empty(t, headers.Get("x-codex-installation-id"), "ordinary WS should use client_metadata projection")
	require.Equal(t, headers.Get("session-id"), headers.Get("thread-id"))
	require.Equal(t, headers.Get("thread-id"), headers.Get("x-client-request-id"))
	require.Empty(t, headers.Get("session_id"))
	require.Empty(t, headers.Get(CodexProjectIDHeader))
	require.Equal(t, "codex_cli_rs", headers.Get("originator"))
	require.NotEqual(t, "forged-agent/1.0", headers.Get("user-agent"))

	require.Equal(t, 1, captureDialer.DialCount(), "same-principal incremental must reuse the exact original connection")
	require.Len(t, captureConn.writes, 2)
	written := []byte(requestToJSONString(captureConn.writes[0]))
	require.Equal(t, headers.Get("session-id"), gjson.GetBytes(written, "prompt_cache_key").String())
	require.NotEmpty(t, gjson.GetBytes(written, "client_metadata.x-codex-installation-id").String())
	require.Equal(t, headers.Get("session-id"), gjson.GetBytes(written, "client_metadata.session_id").String())
	require.Equal(t, headers.Get("thread-id"), gjson.GetBytes(written, "client_metadata.thread_id").String())
	require.False(t, gjson.GetBytes(written, "client_metadata.preserved").Exists())
	bodyMetadata := decodeFingerprintMetadata(t, gjson.GetBytes(written, "client_metadata.x-codex-turn-metadata").String())
	require.Equal(t, "true", bodyMetadata["preserved"])

	headerMetadata := decodeFingerprintMetadata(t, headers.Get("x-codex-turn-metadata"))
	require.Equal(t, headerMetadata["turn_id"], bodyMetadata["turn_id"])
	require.Equal(t, headerMetadata["turn_started_at_unix_ms"], bodyMetadata["turn_started_at_unix_ms"])

	nextWritten := []byte(requestToJSONString(captureConn.writes[1]))
	require.Equal(t, "resp_full_ingress", gjson.GetBytes(nextWritten, "previous_response_id").String())
	require.Equal(t, headers.Get("session-id"), gjson.GetBytes(nextWritten, "prompt_cache_key").String())
	require.NotEqual(t, gjson.GetBytes(written, "client_metadata.turn_id").String(), gjson.GetBytes(nextWritten, "client_metadata.turn_id").String())
}

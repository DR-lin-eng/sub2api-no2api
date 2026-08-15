package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIGatewayServiceForwardWSV2FlushesSemanticOutputBeforeVisibleToken(t *testing.T) {
	for _, visibleOutputTTFT := range []bool{true, false} {
		name := "visible_ttft"
		if !visibleOutputTTFT {
			name = "legacy_ttft"
		}
		t.Run(name, func(t *testing.T) {
			testOpenAIGatewayServiceForwardWSV2FlushesSemanticOutputBeforeVisibleToken(t, visibleOutputTTFT)
		})
	}
}

func testOpenAIGatewayServiceForwardWSV2FlushesSemanticOutputBeforeVisibleToken(t *testing.T, visibleOutputTTFT bool) {
	gin.SetMode(gin.TestMode)

	recorder := newOpenAIResponseFlushRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/openai/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "unit-test-agent/1.0")

	created := []byte(`{"type":"response.created","response":{"id":"resp_flush","model":"gpt-5.1"}}`)
	semanticOutput := []byte(`{"type":"response.output_audio.done","response_id":"resp_flush","audio":"encoded"}`)
	visibleToken := []byte(`{"type":"response.output_text.delta","response_id":"resp_flush","delta":"visible"}`)
	terminal := []byte(`{"type":"response.completed","response":{"id":"resp_flush","model":"gpt-5.1","usage":{"input_tokens":2,"output_tokens":1}}}`)
	allowVisible := make(chan struct{})
	visibleWaiting := make(chan struct{})
	captureConn := &openAIWSCaptureConn{
		events:      [][]byte{created, semanticOutput, visibleToken, terminal},
		readGates:   []<-chan struct{}{nil, nil, allowVisible, nil},
		readWaiting: []chan struct{}{nil, nil, visibleWaiting, nil},
	}

	cfg := newOpenAIWSV2TestConfig()
	cfg.Security.URLAllowlist.Enabled = false
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	cfg.Gateway.OpenAIWS.MinIdlePerAccount = 0
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	cfg.Gateway.OpenAIWS.QueueLimitPerConn = 8
	cfg.Gateway.OpenAIWS.ReadTimeoutSeconds = 5
	cfg.Gateway.OpenAIWS.WriteTimeoutSeconds = 3
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: captureConn})

	svc := &OpenAIGatewayService{
		cfg:              cfg,
		httpUpstream:     &httpUpstreamRecorder{},
		cache:            &stubGatewayCache{},
		openaiWSResolver: NewOpenAIWSProtocolResolver(cfg),
		toolCorrector:    NewCodexToolCorrector(),
		openaiWSPool:     pool,
	}
	account := &Account{
		ID:          82,
		Name:        "openai-ws-flush",
		Platform:    PlatformOpenAI,
		Type:        AccountTypeAPIKey,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{"api_key": "sk-test"},
		Extra:       map[string]any{"responses_websockets_v2_enabled": true},
	}

	resultCh := make(chan *OpenAIForwardResult, 1)
	errCh := make(chan error, 1)
	go func() {
		result, err := svc.Forward(
			withOpenAIVisibleOutputTTFT(context.Background(), visibleOutputTTFT),
			c,
			account,
			[]byte(`{"model":"gpt-5.1","stream":true,"input":"hello"}`),
		)
		resultCh <- result
		errCh <- err
	}()

	waitOpenAIResponseFlushSignal(t, visibleWaiting)
	waitOpenAIResponseFlushCount(t, recorder, 1)
	_, flushes := recorder.snapshot()
	wantFirstFlush := "data: " + string(created) + "\n\ndata: " + string(semanticOutput) + "\n\n"
	require.Equal(t, []string{wantFirstFlush}, flushes, "semantic output must release the WS preamble before a later visible token")

	close(allowVisible)
	require.NoError(t, <-errCh)
	result := <-resultCh
	require.NotNil(t, result)
	require.NotNil(t, result.FirstTokenMs, "TTFT measurement must remain independent from delivery")
}

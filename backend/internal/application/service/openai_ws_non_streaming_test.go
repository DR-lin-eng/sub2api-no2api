package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/codexsimulation"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newCodexPrewarmNonStreamFixture(t *testing.T, events ...string) (*OpenAIGatewayService, *Account, *gin.Context, *httptest.ResponseRecorder, *openAIWSCaptureConn) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.OAuthEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	cfg.Gateway.OpenAIWS.MaxConnsPerAccount = 1
	cfg.Gateway.OpenAIWS.MaxIdlePerAccount = 1
	conn := &openAIWSCaptureConn{}
	for _, event := range events {
		conn.events = append(conn.events, []byte(event))
	}
	pool := newOpenAIWSConnPool(cfg)
	pool.setClientDialerForTest(&openAIWSCaptureDialer{conn: conn})
	t.Cleanup(pool.Close)
	svc := &OpenAIGatewayService{
		cfg: cfg, cache: &stubGatewayCache{}, httpUpstream: &httpUpstreamRecorder{},
		openaiWSPool: pool, toolCorrector: NewCodexToolCorrector(),
	}
	account := &Account{
		ID: 7591, Name: "codex-prewarm-non-stream", Platform: PlatformOpenAI,
		Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "fixture-token"},
		Extra:       map[string]any{CodexPrewarmContinuationExtraKey: true},
	}
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Request.Header.Set("User-Agent", "OpenAI/Python fixture")
	SetOpenAIClientTransport(c, OpenAIClientTransportHTTP)
	return svc, account, c, rec, conn
}

func TestForwardCodexPrewarmNonStreamReconstructsOutput(t *testing.T) {
	const output = `[{"id":"rs_final","type":"reasoning","encrypted_content":"opaque-fixture","summary":[],"vendor":{"value":9007199254740993}},{"id":"msg_final","type":"message","status":"completed","role":"assistant","content":[{"type":"output_text","text":"Hello","annotations":[]}]},{"id":"fc_final","type":"function_call","call_id":"call_final","name":"read_file","arguments":"{\"path\":\"a.txt\"}"}]`
	for _, tc := range []struct {
		name, streamField string
		global            bool
	}{
		{name: "explicit false", streamField: `,"stream":false`},
		{name: "omitted"},
		{name: "global force overrides account", streamField: `,"stream":false`, global: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, account, c, rec, conn := newCodexPrewarmNonStreamFixture(t,
				`{"type":"response.completed","response":{"id":"resp_warm","output":[],"usage":{"input_tokens":99,"output_tokens":0}}}`,
				`{"type":"response.created","response":{"id":"resp_final","object":"response","created_at":1234567890,"status":"in_progress","output":[]}}`,
				`{"type":"response.output_item.done","output_index":0,"item":`+gjson.Get(output, "0").Raw+`}`,
				`{"type":"response.output_text.delta","output_index":1,"item_id":"msg_final","delta":"Hello"}`,
				`{"type":"response.output_item.done","output_index":1,"item":`+gjson.Get(output, "1").Raw+`}`,
				`{"type":"response.output_item.done","output_index":2,"item":`+gjson.Get(output, "2").Raw+`}`,
				`{"type":"response.completed","response":{"id":"resp_final","object":"response","model":"gpt-5.1","status":"completed","instructions":"upstream instructions","output":[],"usage":{"input_tokens":12,"output_tokens":7,"input_tokens_details":{"cached_tokens":3},"output_tokens_details":{"reasoning_tokens":2}}}}`,
			)
			if tc.global {
				account.Extra[CodexPrewarmContinuationExtraKey] = false
				codexsimulation.SetPrewarmContinuationEnabled(true)
				t.Cleanup(func() { codexsimulation.SetPrewarmContinuationEnabled(false) })
			}
			body := []byte(`{"model":"gpt-5.1","instructions":""` + tc.streamField + `,"input":"hello"}`)
			originalBody := string(body)
			result, err := svc.Forward(context.Background(), c, account, body)
			require.NoError(t, err)
			require.NotNil(t, result)
			require.True(t, result.OpenAIWSMode)
			require.False(t, result.Stream)
			require.Equal(t, "resp_final", result.RequestID)
			require.Equal(t, 12, result.Usage.InputTokens)
			require.Equal(t, 7, result.Usage.OutputTokens)
			require.Equal(t, 3, result.Usage.CacheReadInputTokens)
			require.Equal(t, originalBody, string(body))
			require.Equal(t, http.StatusOK, rec.Code)
			require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
			require.True(t, gjson.ValidBytes(rec.Body.Bytes()), "non-stream output must be a single JSON document")
			require.JSONEq(t, output, gjson.GetBytes(rec.Body.Bytes(), "output").Raw)
			require.Equal(t, int64(1234567890), gjson.GetBytes(rec.Body.Bytes(), "created_at").Int())
			require.Equal(t, "", gjson.GetBytes(rec.Body.Bytes(), "instructions").String())
			require.Len(t, conn.writes, 2)
			for _, write := range conn.writes {
				require.Equal(t, true, write["stream"], "only the upstream request should stream")
			}
			require.Equal(t, "resp_warm", conn.writes[1]["previous_response_id"])
			require.NotContains(t, rec.Body.String(), "resp_warm")
		})
	}
}

func TestForwardCodexPrewarmNonStreamDoesNotReintroduceTokenLimits(t *testing.T) {
	for _, fields := range []string{`"max_tokens":64`, `"max_output_tokens":64`, `"max_completion_tokens":64`, `"max_tokens":64,"max_output_tokens":128`} {
		t.Run(fields, func(t *testing.T) {
			svc, account, c, _, conn := newCodexPrewarmNonStreamFixture(t,
				`{"type":"response.completed","response":{"id":"resp_warm"}}`,
				`{"type":"response.completed","response":{"id":"resp_final","output":[],"usage":{"input_tokens":1,"output_tokens":1}}}`,
			)
			_, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.1","stream":false,"input":"hello",`+fields+`}`))
			require.NoError(t, err)
			require.Len(t, conn.writes, 2)
			for _, write := range conn.writes {
				for _, field := range []string{"max_tokens", "max_output_tokens", "max_completion_tokens"} {
					_, exists := write[field]
					require.False(t, exists, "Codex rejects %s on both prewarm and business requests", field)
				}
			}
		})
	}
}

func TestOpenAIWSNonStreamRequiresTerminalResponse(t *testing.T) {
	svc, account, c, rec, _ := newCodexPrewarmNonStreamFixture(t,
		`{"type":"response.completed","response":{"id":"resp_warm"}}`,
		`{"type":"response.created","response":{"id":"resp_partial","status":"in_progress","output":[]}}`,
		`{"type":"response.completed"}`,
	)
	result, err := svc.forwardOpenAIWSV2(context.Background(), c, account, map[string]any{"model": "gpt-5.1", "stream": true}, "fixture-token",
		OpenAIWSProtocolDecision{Transport: OpenAIUpstreamTransportResponsesWebsocketV2}, false, false, "gpt-5.1", "gpt-5.1", time.Now(), 1, "", nil, nil, false)
	require.Error(t, err)
	require.Nil(t, result)
	require.Empty(t, rec.Body.String(), "a response.created snapshot cannot substitute for the missing terminal response")
}

func TestForwardCodexPrewarmNonStreamAggregatesDeltasForTerminalStatuses(t *testing.T) {
	for _, status := range []string{"completed", "done", "incomplete", "cancelled", "canceled"} {
		t.Run(status, func(t *testing.T) {
			svc, account, c, rec, _ := newCodexPrewarmNonStreamFixture(t,
				`{"type":"response.completed","response":{"id":"resp_warm"}}`,
				`{"type":"response.output_text.delta","delta":"Hel"}`,
				`{"type":"response.output_text.delta","delta":"lo"}`,
				`{"type":"response.`+status+`","response":{"id":"resp_final","object":"response","status":"`+status+`","output":[],"usage":{"input_tokens":3,"output_tokens":2}}}`,
			)
			result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.4","stream":false,"input":"hello"}`))
			require.NoError(t, err)
			require.False(t, result.Stream)
			require.Equal(t, "Hello", gjson.GetBytes(rec.Body.Bytes(), "output.0.content.0.text").String())
			require.Equal(t, status, gjson.GetBytes(rec.Body.Bytes(), "status").String())
			require.Equal(t, 3, result.Usage.InputTokens)
		})
	}
}

func TestForwardCodexPrewarmNonStreamLimitsAccumulation(t *testing.T) {
	svc, account, c, rec, conn := newCodexPrewarmNonStreamFixture(t,
		`{"type":"response.completed","response":{"id":"resp_warm"}}`,
		`{"type":"response.output_text.delta","delta":"`+strings.Repeat("x", 100)+`"}`,
	)
	svc.cfg.Gateway.UpstreamResponseReadMaxBytes = 100
	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.4","stream":false,"input":"hello"}`))
	require.ErrorIs(t, err, ErrUpstreamResponseBodyTooLarge)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadGateway, rec.Code)
	require.True(t, gjson.ValidBytes(rec.Body.Bytes()))
	require.True(t, IsResponseCommitted(c), "the handler must not append an SSE failure to the JSON error")
	require.Len(t, conn.writes, 2)
	require.True(t, conn.closed)
}

func TestForwardCodexPrewarmNonStreamCommitsJSONError(t *testing.T) {
	svc, account, c, rec, _ := newCodexPrewarmNonStreamFixture(t,
		`{"type":"response.completed","response":{"id":"resp_warm"}}`,
		`{"type":"error","error":{"type":"invalid_request_error","code":"invalid_request","message":"invalid fixture input"}}`,
	)
	result, err := svc.Forward(context.Background(), c, account, []byte(`{"model":"gpt-5.4","stream":false,"input":"hello"}`))
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.True(t, gjson.ValidBytes(rec.Body.Bytes()))
	require.True(t, IsResponseCommitted(c), "the handler must not append an SSE failure to the JSON error")
}

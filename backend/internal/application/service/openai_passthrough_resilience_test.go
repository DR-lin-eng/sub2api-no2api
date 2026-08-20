package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newOpenAIPassthroughResilienceAccount(id int64) *Account {
	return &Account{
		ID: id, Name: "resilience", Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{
			"access_token": "oauth-token", "chatgpt_account_id": "chatgpt-account",
		},
		Extra: map[string]any{"openai_passthrough": true},
	}
}

func newOpenAIPassthroughResilienceContext(body []byte) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader(body))
	c.Request.Header.Set("User-Agent", "codex_cli_rs/0.146.0")
	return c, recorder
}

func TestOpenAIPassthroughRetryBudgetSpansAccountForwards(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.6-luna","stream":false,"instructions":"test","input":"hello"}`)
	c, _ := newOpenAIPassthroughResilienceContext(body)
	// One permitted account switch plus the three same-account recovery attempts
	// yields five upstream attempts total, not four per selected account.
	ConfigureOpenAIPassthroughAttemptBudget(c, 1)
	upstream := &httpUpstreamRecorder{err: errors.New("connection reset")}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

	_, firstErr := svc.Forward(context.Background(), c, newOpenAIPassthroughResilienceAccount(901), body)
	require.Error(t, firstErr)
	_, secondErr := svc.Forward(context.Background(), c, newOpenAIPassthroughResilienceAccount(902), body)
	require.Error(t, secondErr)

	var failoverErr *UpstreamFailoverError
	require.ErrorAs(t, secondErr, &failoverErr)
	require.Equal(t, openAIPassthroughRetryBudgetReason, failoverErr.Reason)
	require.False(t, failoverErr.ShouldRetryNextAccount())
	require.Len(t, upstream.requests, 5)
	require.Equal(t, []int64{901, 901, 901, 901, 902}, upstream.accountIDs)
}

func TestOpenAIPassthroughDoesNotReplayAmbiguousSideEffects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range [][]byte{
		[]byte(`{"model":"gpt-5.6-luna","stream":false,"store":true,"instructions":"test","input":"hello"}`),
		[]byte(`{"model":"gpt-5.6-luna","stream":false,"instructions":"test","tools":[{"type":"image_generation"}],"input":"draw"}`),
	} {
		c, _ := newOpenAIPassthroughResilienceContext(body)
		upstream := &httpUpstreamRecorder{err: errors.New("connection reset")}
		svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}

		_, err := svc.Forward(context.Background(), c, newOpenAIPassthroughResilienceAccount(903), body)
		require.Error(t, err)
		var failoverErr *UpstreamFailoverError
		require.ErrorAs(t, err, &failoverErr)
		require.Equal(t, openAIPassthroughNonReplayableReason, failoverErr.Reason)
		require.False(t, failoverErr.ShouldRetryNextAccount())
		require.Len(t, upstream.requests, 1)
	}
}

func TestOpenAIPassthroughTimeoutRecoveryDoesNotLeaveRuntimeBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.6-luna","stream":false,"instructions":"test","input":"hello"}`)
	c, _ := newOpenAIPassthroughResilienceContext(body)
	upstream := &httpUpstreamRecorder{
		errs: []error{context.DeadlineExceeded, nil},
		responses: []*http.Response{{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: io.NopCloser(strings.NewReader(
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_recovered\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n",
			)),
		}},
	}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := newOpenAIPassthroughResilienceAccount(904)

	result, err := svc.Forward(context.Background(), c, account, body)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, upstream.requests, 2)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestOpenAIPassthroughTimeoutExhaustionBlocksAfterRetryBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.6-luna","stream":false,"instructions":"test","input":"hello"}`)
	c, _ := newOpenAIPassthroughResilienceContext(body)
	upstream := &httpUpstreamRecorder{err: context.DeadlineExceeded}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := newOpenAIPassthroughResilienceAccount(905)

	_, err := svc.Forward(context.Background(), c, account, body)
	require.Error(t, err)
	require.Len(t, upstream.requests, len(openAIPassthroughTransportRetryBackoffs)+1)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
}

func TestOpenAIPassthroughKeepaliveDeliversAndRestoresTurnState(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := []byte(`{"model":"gpt-5.6-luna","stream":true,"prompt_cache_key":"turn-state-key","instructions":"test","input":"hello"}`)
	c, recorder := newOpenAIPassthroughResilienceContext(body)
	account := newOpenAIPassthroughResilienceAccount(906)
	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{StreamKeepaliveInterval: 1}}}
	stageOpenAICompatTurnStateKey(c, account, body)

	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		time.Sleep(1100 * time.Millisecond)
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"ok\"}\n\n")
		_, _ = io.WriteString(writer, "data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_turn_state\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}}\n\n")
	}()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":       []string{"text/event-stream"},
			"X-Codex-Turn-State": []string{"opaque-turn-state"},
			"X-Request-Id":       []string{"rid-turn-state"},
		},
		Body: reader,
	}
	ctx := context.WithValue(context.Background(), openAIStreamRuntimeSettingsContextKey{}, OpenAIStreamRuntimeSettings{
		KeepaliveIntervalSeconds: 1,
	})
	_, err := svc.handleStreamingResponsePassthrough(ctx, resp, c, account, time.Now(), "gpt-5.6-luna", "gpt-5.6-luna")
	require.NoError(t, err)
	require.Contains(t, recorder.Body.String(), ":\n\n")
	require.Contains(t, recorder.Header().Get("Trailer"), "X-Codex-Turn-State")
	require.Equal(t, "opaque-turn-state", recorder.Header().Get("X-Codex-Turn-State"))
	require.Equal(t, "opaque-turn-state", svc.getOpenAICompatSessionTurnState(context.Background(), c, account, "turn-state-key"))

	next, _ := newOpenAIPassthroughResilienceContext(body)
	req, buildErr := svc.buildUpstreamRequestOpenAIPassthrough(context.Background(), next, account, body, "oauth-token")
	require.NoError(t, buildErr)
	require.Equal(t, "opaque-turn-state", req.Header.Get("X-Codex-Turn-State"))
}

func TestOpenAIHTTPTurnStateSharedAcrossServiceInstances(t *testing.T) {
	gin.SetMode(gin.TestMode)
	shared := &stubOpenAIWSSharedCache{}
	first := &OpenAIGatewayService{cache: shared}
	second := &OpenAIGatewayService{cache: shared}
	account := newOpenAIPassthroughResilienceAccount(907)
	body := []byte(`{"model":"gpt-5.6-luna","prompt_cache_key":"shared-turn","input":"hello"}`)
	c, _ := newOpenAIPassthroughResilienceContext(body)
	c.Request.Header.Set("session_id", "shared-session")

	first.bindOpenAIHTTPSharedTurnState(context.Background(), c, account, body, "shared-state")
	require.Eventually(t, func() bool {
		return second.getOpenAIHTTPSharedTurnState(context.Background(), c, account, body) == "shared-state"
	}, time.Second, 10*time.Millisecond)
}

type blockingStreamTimeoutSettingsRepo struct {
	*schedulerV2SettingRepo
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *blockingStreamTimeoutSettingsRepo) GetValue(ctx context.Context, key string) (string, error) {
	if key != SettingKeyStreamTimeoutSettings {
		return r.schedulerV2SettingRepo.GetValue(ctx, key)
	}
	r.once.Do(func() { close(r.started) })
	select {
	case <-r.release:
		return r.schedulerV2SettingRepo.GetValue(ctx, key)
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func TestOpenAIStreamRuntimeSettingsRefreshIsStaleWhileRevalidate(t *testing.T) {
	repo := &blockingStreamTimeoutSettingsRepo{
		schedulerV2SettingRepo: &schedulerV2SettingRepo{values: map[string]string{
			SettingKeyStreamTimeoutSettings: `{"response_header_timeout_seconds":37,"openai_first_output_timeout_seconds":60,"openai_high_effort_first_output_timeout_seconds":240,"stream_keepalive_interval_seconds":5,"enabled":false,"action":"none","temp_unsched_minutes":5,"threshold_count":3,"threshold_window_minutes":10}`,
		}},
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	svc := NewSettingService(repo, &config.Config{Gateway: config.GatewayConfig{
		OpenAIFirstOutputTimeoutSeconds:           90,
		OpenAIHighEffortFirstOutputTimeoutSeconds: 180,
		StreamKeepaliveInterval:                   10,
	}})

	start := time.Now()
	runtime := svc.OpenAIStreamRuntimeSettings(context.Background())
	require.Less(t, time.Since(start), 100*time.Millisecond)
	require.Equal(t, 90, runtime.FirstOutputTimeoutSeconds)
	select {
	case <-repo.started:
	case <-time.After(time.Second):
		t.Fatal("runtime settings refresh did not start")
	}
	close(repo.release)
	require.Eventually(t, func() bool {
		return svc.OpenAIStreamRuntimeSettings(context.Background()).FirstOutputTimeoutSeconds == 60
	}, time.Second, 10*time.Millisecond)
}

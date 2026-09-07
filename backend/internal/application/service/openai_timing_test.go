package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/openaitiming"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

const openAITimingSampleSSE = "data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp_test\",\"status\":\"in_progress\"}}\n\n" +
	"data: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\n" +
	"data: {\"type\":\"responsesapi.websocket_timing\",\"timing_metrics\":{\"response_id\":\"resp_test\",\"timing_scope\":\"logical_turn\",\"first_sampled_message_ttft_ms\":470,\"engine_service_ttft_total_ms\":690.87897,\"engine_queue_max_ms\":74,\"total_turn_time_s\":1.047620254,\"num_engine_calls\":1}}\n\n" +
	"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_test\",\"status\":\"completed\",\"model\":\"gpt-5.2\",\"output\":[{\"type\":\"message\",\"role\":\"assistant\",\"content\":[{\"type\":\"output_text\",\"text\":\"hi\"}]}],\"usage\":{\"input_tokens\":11,\"output_tokens\":7}}}\n\n"

func TestOpenAITimingForwardHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, passthrough := range []bool{false, true} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("passthrough=%v/stream=%v", passthrough, stream), func(t *testing.T) {
				rec := httptest.NewRecorder()
				c, _ := gin.CreateTestContext(rec)
				c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
				c.Request.Header.Set("User-Agent", "codex_cli_rs/0.1.0")
				upstream := &httpUpstreamRecorder{resp: &http.Response{
					StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
					Body: io.NopCloser(strings.NewReader(openAITimingSampleSSE)),
				}}
				svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
				account := &Account{ID: 123, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
					Credentials: map[string]any{"access_token": "test", "chatgpt_account_id": "test"},
					Extra:       map[string]any{"openai_passthrough": passthrough}}
				result, err := svc.Forward(context.Background(), c, account, []byte(fmt.Sprintf(`{"model":"gpt-5.2","stream":%v,"input":"hi"}`, stream)))
				require.NoError(t, err)
				require.NotNil(t, result.OpenAITiming)
				require.Equal(t, 691, *result.OpenAITiming.FirstTokenMs())
				require.Equal(t, 1048, *result.OpenAITiming.DurationMs())
				require.Equal(t, 11, result.Usage.InputTokens)
				require.Equal(t, 7, result.Usage.OutputTokens)
				require.Contains(t, rec.Body.String(), "hi")
			})
		}
	}
}

func TestOpenAITimingBufferedCompat(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	svc := &OpenAIGatewayService{}
	resp := &http.Response{Body: io.NopCloser(strings.NewReader(openAITimingSampleSSE))}
	final, usage, _, err := svc.readOpenAICompatBufferedTerminal(resp, "test", "test", c)
	require.NoError(t, err)
	require.Equal(t, "completed", final.Status)
	require.Equal(t, 11, usage.InputTokens)
	require.Equal(t, 691, *observedOpenAITiming(c).FirstTokenMs())
	beginOpenAITimingObservation(c)
	require.Nil(t, observedOpenAITiming(c))
}

func TestOpenAITimingRecordUsagePreservesLocalMetrics(t *testing.T) {
	usageRepo := &openAIRecordUsageLogRepoStub{inserted: true}
	billingRepo := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: true}}
	svc := newOpenAIRecordUsageServiceWithBillingRepoForTest(usageRepo, billingRepo, &openAIRecordUsageUserRepoStub{}, &openAIRecordUsageSubRepoStub{}, nil)
	first, engine, total := 1250, 690.87897, 1.047620254
	timing := &openaitiming.Metrics{EngineServiceTTFTTotalMs: &engine, TotalTurnTimeSeconds: &total}
	err := svc.RecordUsage(context.Background(), &OpenAIRecordUsageInput{
		Result: &OpenAIForwardResult{RequestID: "timing-record", Model: "gpt-5.1", Usage: OpenAIUsage{InputTokens: 11, OutputTokens: 7},
			FirstTokenMs: &first, Duration: 1800 * time.Millisecond, OpenAITiming: timing},
		APIKey: &APIKey{ID: 1000, Group: &Group{RateMultiplier: 1}},
		User:   &User{ID: 2000}, Account: &Account{ID: 3000, Type: AccountTypeAPIKey},
	})
	require.NoError(t, err)
	require.Equal(t, 1, usageRepo.calls)
	require.Equal(t, 1, billingRepo.calls)
	require.Equal(t, 1250, *usageRepo.lastLog.FirstTokenMs)
	require.Equal(t, 1800, *usageRepo.lastLog.DurationMs)
	require.Equal(t, timing, usageRepo.lastLog.OpenAITiming)
	require.Equal(t, 11, usageRepo.lastLog.InputTokens)
	require.Equal(t, 7, usageRepo.lastLog.OutputTokens)
	require.Positive(t, usageRepo.lastLog.ActualCost)
}

//go:build unit

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type syncStartupRemote struct {
	PricingRemoteClient
	body             []byte
	started, release chan struct{}
}

func (r *syncStartupRemote) FetchHashText(ctx context.Context, _ string) (string, error) {
	close(r.started)
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-r.release:
	}
	sum := sha256.Sum256(r.body)
	return hex.EncodeToString(sum[:]), nil
}
func (r *syncStartupRemote) FetchPricingJSON(context.Context, string) ([]byte, error) {
	return r.body, nil
}

func TestSyncPricingStartupUsesLocalCardAndJoinsRemoteWorker(t *testing.T) {
	remote := &syncStartupRemote{body: []byte(`{"fixture":{"input_cost_per_token":0.000001,"output_cost_per_token":0.000002}}`), started: make(chan struct{}), release: make(chan struct{})}
	cfg := &config.Config{Pricing: config.PricingConfig{DataDir: t.TempDir(), RemoteURL: "https://fixture.invalid/pricing.json", HashURL: "https://fixture.invalid/pricing.sha256"}}
	svc := NewPricingService(cfg, remote)
	require.NoError(t, os.WriteFile(svc.getPricingFilePath(), remote.body, 0600))
	initialized := make(chan error, 1)
	go func() { initialized <- svc.Initialize() }()
	released := false
	defer func() {
		if !released {
			close(remote.release)
		}
		svc.Stop()
	}()
	select {
	case err := <-initialized:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("local pricing startup waited for remote hash")
	}
	select {
	case <-remote.started:
	case <-time.After(time.Second):
		t.Fatal("startup hash was not scheduled")
	}
	require.NotNil(t, svc.GetModelPricing("fixture"))
	close(remote.release)
	released = true
}

type syncQuotaRepo struct {
	AccountRepository
	updates int
}

func (r *syncQuotaRepo) UpdateExtra(context.Context, int64, map[string]any) error {
	r.updates++
	return nil
}
func TestSyncKimiMonthlyAndMalformedSnapshot(t *testing.T) {
	tiers := parseKimiUsageTiers([]byte(`{"usages":{"limit_5h":{"used_ratio":0.2},"limit_month_total":{"used_ratio":0.8,"reset_time":"2026-12-10T00:00:00Z"},"limit_month_code":{"used_ratio":1,"reset_time":"2026-12-01T00:00:00Z"}}}`))
	require.Len(t, tiers, 2)
	require.Equal(t, "monthly", tiers[1].Window)
	require.Equal(t, 100.0, tiers[1].UsedPercent)
	require.Equal(t, "2026-12-10T00:00:00Z", tiers[1].ResetAt)
	updates := cnQuotaExtraUpdates(PlatformKimi, tiers, time.Now())
	clearAbsentCNQuotaWindows(PlatformKimi, tiers, updates)
	require.Contains(t, updates, "kimi_weekly_used_percent")
	require.Nil(t, updates["kimi_weekly_used_percent"])
	for _, raw := range []string{`{}`, `{"limits":[{"detail":{}}]}`, `{"usages":{"limit_month_total":{"used_ratio":"NaN"}}}`, `{"usages":{"limit_5h":{"used_ratio":65}}}`} {
		require.Empty(t, parseKimiUsageTiers([]byte(raw)))
		repo := &syncQuotaRepo{}
		upstream := &httpUpstreamRecorder{resp: &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(raw))}}
		svc := &CNProviderQuotaService{accountRepo: repo, httpUpstream: upstream, cfg: &config.Config{}}
		result, err := svc.queryUsageForAccount(context.Background(), &Account{ID: 1, Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture", "account_mode": AccountModeCoding}})
		require.NoError(t, err)
		require.False(t, result.Success)
		require.Zero(t, repo.updates, "unrecognized schema must preserve the old exhausted snapshot")
	}
}

func TestSyncAnthropicCompatibilityResponseModelObservation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"chat", "responses"} {
		for _, stream := range []bool{false, true} {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/"+endpoint, nil)
			svc := &GatewayService{cfg: &config.Config{}}
			var result *ForwardResult
			var err error
			response := nativeAnthropicStreamResponse()
			if endpoint == "chat" {
				if stream {
					result, err = svc.handleCCStreamingFromAnthropic(response, c, "public-model", "sent-model", nil, time.Now(), true)
				} else {
					result, err = svc.handleCCBufferedFromAnthropic(response, c, "public-model", "sent-model", nil, time.Now())
				}
			} else {
				if stream {
					result, err = svc.handleResponsesStreamingResponse(response, c, "public-model", "sent-model", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
				} else {
					result, err = svc.handleResponsesBufferedStreamingResponse(response, c, "public-model", "sent-model", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
				}
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "claude-sonnet-4-5", result.UpstreamResponseModel)
			require.Equal(t, "sent-model", result.UpstreamModel)
		}
	}
}

func TestSyncConservativeSSEFastPath(t *testing.T) {
	require.False(t, sseEventNeedsJSONParse("content_block_delta", []byte(`{"type":"content_block_delta","delta":{"type":"text_delta","text":"x"}}`), false, false))
	for _, raw := range []string{`{"type":"content_block_delta","us\u0061ge":{"output_tokens":3}}`, `{"type":"error","error":{"type":"overloaded_error"}}`, `{"type":"content_block_delta","usage":{"output_tokens":3}}`} {
		require.True(t, sseEventNeedsJSONParse("content_block_delta", []byte(raw), false, false), raw)
	}
}

func TestSyncPerTurnBetaKeepsControlMessage(t *testing.T) {
	body := []byte(`{"messages":[{"role":"system","content":[],"output_config":{"effort":"high"}},{"role":"user","content":"hello"}],"output_config":{"effort":"low"}}`)
	preserved, changed := stripAnthropicMessageOutputConfigUnlessBeta(body, "per-turn-control-2026-07-01")
	require.False(t, changed)
	require.Equal(t, body, preserved)
	_, changed = stripAnthropicMessageOutputConfigUnlessBeta(body, "")
	require.True(t, changed)
}

func TestSyncOpenAINativeCompatibilityResponseModelObservation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, endpoint := range []string{"chat", "responses"} {
		for _, stream := range []bool{false, true} {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/v1/"+endpoint, nil)
			svc := &OpenAIGatewayService{cfg: &config.Config{}}
			var result *OpenAIForwardResult
			var err error
			response := nativeAnthropicStreamResponse()
			if endpoint == "chat" {
				if stream {
					result, err = svc.handleCCStreamingFromNativeAnthropic(response, c, "public-model", "billing-model", "sent-model", nil, time.Now(), true)
				} else {
					result, err = svc.handleCCBufferedFromNativeAnthropic(response, c, "public-model", "billing-model", "sent-model", nil, time.Now())
				}
			} else {
				if stream {
					result, err = svc.handleResponsesStreamingFromNativeAnthropic(response, c, "public-model", "billing-model", "sent-model", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
				} else {
					result, err = svc.handleResponsesBufferedFromNativeAnthropic(response, c, "public-model", "billing-model", "sent-model", nil, time.Now(), apicompat.ResponsesClientToolMapping{})
				}
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, "claude-sonnet-4-5", result.UpstreamResponseModel)
			require.Equal(t, "sent-model", result.UpstreamModel)
			require.Equal(t, "billing-model", result.BillingModel)
		}
	}
}

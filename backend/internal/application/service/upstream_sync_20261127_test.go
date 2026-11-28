//go:build unit

package service

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"syscall"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/antigravity"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/apicompat"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/timezone"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/usagestats"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

type stickyCASStubCache struct {
	stubGatewayCache
	casCalled bool
	current   int64
}

func (c *stickyCASStubCache) CompareAndSetSessionAccountID(_ context.Context, _ int64, _ string, accountID int64, failedOver []int64, _ time.Duration) (bool, error) {
	c.casCalled = true
	if c.current == 0 || c.current == accountID {
		c.current = accountID
		return true, nil
	}
	for _, f := range failedOver {
		if f == c.current {
			c.current = accountID
			return true, nil
		}
	}
	return false, nil
}

func TestGatewayService_BindStickySessionUnlessRebound(t *testing.T) {
	groupID := int64(5)
	cache := &stickyCASStubCache{current: 300}
	svc := &GatewayService{cache: cache}

	// Rebound to 300 by another request -> does not overwrite with 100.
	require.NoError(t, svc.BindStickySessionUnlessRebound(context.Background(), &groupID, "sess", 100, []int64{99}))
	require.True(t, cache.casCalled)
	require.Equal(t, int64(300), cache.current)

	// Failed over from 300 -> updates to 400.
	require.NoError(t, svc.BindStickySessionUnlessRebound(context.Background(), &groupID, "sess", 400, []int64{300}))
	require.Equal(t, int64(400), cache.current)
}

func TestOpenAITransportErrorClassificationAndCooldowns(t *testing.T) {
	require.Equal(t, 30*time.Second, classifyOpenAITransportError(syscall.ECONNREFUSED).CooldownDuration())
	require.Equal(t, 30*time.Second, classifyOpenAITransportError(&net.DNSError{IsNotFound: true, Err: "no such host"}).CooldownDuration())
	require.Equal(t, 30*time.Second, classifyOpenAITransportError(errors.New("dial tcp: network is unreachable")).CooldownDuration())
	require.Equal(t, 10*time.Minute, classifyOpenAITransportError(errors.New("socks connect: username/password authentication failed")).CooldownDuration())
	require.Equal(t, 10*time.Minute, classifyOpenAITransportError(errors.New("proxyconnect tcp: Proxy Authentication Required")).CooldownDuration())
	require.Equal(t, time.Duration(0), classifyOpenAITransportError(errors.New("unexpected EOF")).CooldownDuration())
	require.False(t, classifyOpenAITransportError(errors.New("unexpected EOF")).Persistent())
	require.True(t, classifyOpenAITransportError(syscall.ECONNREFUSED).Persistent())
}

func TestExtractGeminiUsage_IgnoresNullAndEmptyMetadata(t *testing.T) {
	require.Nil(t, extractGeminiUsage([]byte(`{"candidates":[]}`)))
	require.Nil(t, extractGeminiUsage([]byte(`{"usageMetadata":null}`)))
	require.Nil(t, extractGeminiUsage([]byte(`{"usageMetadata":{}}`)))
	require.Nil(t, extractGeminiUsage([]byte(`{"usageMetadata":{"promptTokenCount":null,"candidatesTokenCount":null}}`)))

	explicitZero := extractGeminiUsage([]byte(`{"usageMetadata":{"promptTokenCount":0,"candidatesTokenCount":0}}`))
	require.NotNil(t, explicitZero)
	require.Zero(t, explicitZero.InputTokens)
	require.Zero(t, explicitZero.OutputTokens)

	valid := extractGeminiUsage([]byte(`{"usageMetadata":{"promptTokenCount":120,"cachedContentTokenCount":20,"candidatesTokenCount":35,"thoughtsTokenCount":10}}`))
	require.NotNil(t, valid)
	require.Equal(t, 100, valid.InputTokens)
	require.Equal(t, 45, valid.OutputTokens)
	require.Equal(t, 20, valid.CacheReadInputTokens)
}

type accountUpstreamErrorStubRepo struct {
	UsageLogRepository
	todayStats  *usagestats.AccountStats
	batchStats  map[int64]*usagestats.AccountStats
	usageStats  *usagestats.AccountUsageStatsResponse
	batchErrors map[int64]int64
	dailyErrors map[string]int64
}

func (r *accountUpstreamErrorStubRepo) GetAccountTodayStats(context.Context, int64) (*usagestats.AccountStats, error) {
	return r.todayStats, nil
}

func (r *accountUpstreamErrorStubRepo) GetAccountWindowStatsBatch(context.Context, []int64, time.Time) (map[int64]*usagestats.AccountStats, error) {
	return r.batchStats, nil
}

func (r *accountUpstreamErrorStubRepo) GetAccountUsageStats(context.Context, int64, time.Time, time.Time) (*usagestats.AccountUsageStatsResponse, error) {
	return r.usageStats, nil
}

func (r *accountUpstreamErrorStubRepo) GetAccountUpstreamErrorCountsBatch(context.Context, []int64, time.Time) (map[int64]int64, error) {
	return r.batchErrors, nil
}

func (r *accountUpstreamErrorStubRepo) GetAccountDailyUpstreamErrorCounts(context.Context, int64, time.Time, time.Time) (map[string]int64, error) {
	return r.dailyErrors, nil
}

func TestAccountUsageService_AttachesUpstreamErrors(t *testing.T) {
	todayStr := timezone.Now().Format("2006-01-02")
	repo := &accountUpstreamErrorStubRepo{
		todayStats: &usagestats.AccountStats{Requests: 10, Tokens: 500, Cost: 1.25},
		batchStats: map[int64]*usagestats.AccountStats{
			1: {Requests: 10, Tokens: 500, Cost: 1.25},
			2: {Requests: 4, Tokens: 100, Cost: 0.2},
		},
		usageStats: &usagestats.AccountUsageStatsResponse{
			History: []usagestats.AccountUsageHistory{
				{Date: "2026-11-20", Label: "11/20", Requests: 5},
			},
			Summary: usagestats.AccountUsageSummary{Days: 30, TotalRequests: 5},
		},
		batchErrors: map[int64]int64{1: 3},
		dailyErrors: map[string]int64{
			"2026-11-19": 2,
			"2026-11-20": 4,
			todayStr:     1,
		},
	}

	svc := &AccountUsageService{usageLogRepo: repo}
	single, err := svc.GetTodayStats(context.Background(), 1)
	require.NoError(t, err)
	require.Equal(t, int64(3), single.UpstreamErrors)

	batch, err := svc.GetTodayStatsBatch(context.Background(), []int64{1, 2})
	require.NoError(t, err)
	require.Equal(t, int64(3), batch[1].UpstreamErrors)
	require.Zero(t, batch[2].UpstreamErrors)

	detail, err := svc.GetAccountUsageStats(context.Background(), 1, time.Now().AddDate(0, 0, -29), time.Now())
	require.NoError(t, err)
	require.Equal(t, int64(7), detail.Summary.TotalUpstreamErrors)
	require.NotNil(t, detail.Summary.Today)
	require.Equal(t, int64(1), detail.Summary.Today.UpstreamErrors)
	require.Equal(t, "2026-11-19", detail.History[0].Date)
	require.Equal(t, int64(2), detail.History[0].UpstreamErrors)
	require.Equal(t, int64(4), detail.History[1].UpstreamErrors)
}

func TestAntigravityGeminiCompat_OmitIncludeThoughtsConfig(t *testing.T) {
	claudeBody := []byte(`{"model":"gemini-2.5-pro","stream":true,"thinking":{"type":"enabled","budget_tokens":2048},"messages":[{"role":"user","content":"hi"}]}`)
	claudeReq := &antigravity.ClaudeRequest{Model: "gemini-2.5-pro", Stream: true}

	defaultSvc := &AntigravityGatewayService{
		settingService: &SettingService{cfg: &config.Config{}},
	}
	withThoughts, err := defaultSvc.buildAntigravityCompatGeminiBody(context.Background(), claudeBody, claudeReq, "proj-1", "gemini-2.5-pro")
	require.NoError(t, err)
	require.True(t, gjson.GetBytes(withThoughts, "request.generationConfig.thinkingConfig.includeThoughts").Bool())

	omitSvc := &AntigravityGatewayService{
		settingService: &SettingService{cfg: &config.Config{Gateway: config.GatewayConfig{AntigravityGeminiOmitIncludeThoughts: true}}},
	}
	omitted, err := omitSvc.buildAntigravityCompatGeminiBody(context.Background(), claudeBody, claudeReq, "proj-1", "gemini-2.5-pro")
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(omitted, "request.generationConfig.thinkingConfig.includeThoughts").Exists())
}

type proxyLatencyCacheStub struct {
	items map[int64]*ProxyLatencyInfo
}

func (s *proxyLatencyCacheStub) GetProxyLatencies(_ context.Context, ids []int64) (map[int64]*ProxyLatencyInfo, error) {
	out := make(map[int64]*ProxyLatencyInfo, len(ids))
	for _, id := range ids {
		if info, ok := s.items[id]; ok {
			out[id] = info
		}
	}
	return out, nil
}

func (s *proxyLatencyCacheStub) SetProxyLatency(context.Context, int64, *ProxyLatencyInfo) error {
	return nil
}

func TestAdminProxy_AttachProxyLatencySetsCheckedAt(t *testing.T) {
	checkedAt := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC)
	latency := int64(85)
	svc := &adminServiceImpl{
		proxyLatencyCache: &proxyLatencyCacheStub{
			items: map[int64]*ProxyLatencyInfo{
				9: {Success: true, LatencyMs: &latency, IPAddress: "203.0.113.9", Region: "Tokyo", UpdatedAt: checkedAt},
			},
		},
	}
	proxies := []ProxyWithAccountCount{{Proxy: Proxy{ID: 9}}}
	svc.attachProxyLatency(context.Background(), proxies)
	require.NotNil(t, proxies[0].LatencyCheckedAt)
	require.True(t, proxies[0].LatencyCheckedAt.Equal(checkedAt))
	require.Equal(t, "203.0.113.9", proxies[0].IPAddress)
	require.Equal(t, "Tokyo", proxies[0].Region)
}

func TestEnsureOpenAIChatReasoningContentOptIn(t *testing.T) {
	body := []byte(`{"model":"kimi-for-coding","messages":[{"role":"user","content":"hi"},{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"ls","arguments":"{}"}}]},{"role":"assistant","content":"done"}]}`)

	disabledAccount := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	require.Equal(t, body, ensureOpenAIChatReasoningContentOptIn(disabledAccount, body))

	optInAccount := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{OpenAIChatReasoningContentExtraKey: true},
	}
	patched := ensureOpenAIChatReasoningContentOptIn(optInAccount, body)
	rc := gjson.GetBytes(patched, "messages.1.reasoning_content")
	require.True(t, rc.Exists())
	require.Equal(t, "", rc.String())
	require.False(t, gjson.GetBytes(patched, "messages.2.reasoning_content").Exists(), "non-tool assistant turn must stay untouched")

	withExisting := []byte(`{"model":"kimi-for-coding","messages":[{"role":"assistant","reasoning_content":"step 1","tool_calls":[{"id":"c1","type":"function","function":{"name":"ls","arguments":"{}"}}]}]}`)
	require.Equal(t, "step 1", gjson.GetBytes(ensureOpenAIChatReasoningContentOptIn(optInAccount, withExisting), "messages.0.reasoning_content").String())
}

func TestCodexModelsMetadata_ImageInputModelsExtra(t *testing.T) {
	manifest := []byte(`{"models":[{"slug":"qwen-vl-local","input_modalities":["text"]},{"slug":"text-only","input_modalities":["text"]}]}`)
	account := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeAPIKey,
		Extra: map[string]any{
			ImageInputModelsExtraKey: []any{"Qwen-VL-Local"},
		},
	}
	require.True(t, CodexManifestProjectionEnabled(account))
	require.True(t, AccountDeclaresImageInputModel(account, "qwen-vl-local"))
	require.False(t, AccountDeclaresImageInputModel(account, "text-only"))

	enriched, err := enrichCodexModelsManifest(manifest, account)
	require.NoError(t, err)
	var parsed struct {
		Models []struct {
			Slug            string   `json:"slug"`
			InputModalities []string `json:"input_modalities"`
		} `json:"models"`
	}
	require.NoError(t, json.Unmarshal(enriched, &parsed))
	require.Equal(t, []string{"text", "image"}, parsed.Models[0].InputModalities)
	require.Equal(t, []string{"text"}, parsed.Models[1].InputModalities)
}

func TestOpenAICompatAnthropicReasoningEffort_DeepSeekDefaultHigh(t *testing.T) {
	require.Equal(t, "high", openAICompatAnthropicReasoningEffort(nil, "deepseek-reasoner", "medium"))
	require.Equal(t, "high", openAICompatAnthropicReasoningEffort(&apicompat.AnthropicRequest{}, "deepseek-ai/DeepSeek-V3.2", "medium"))
	require.Equal(t, "medium", openAICompatAnthropicReasoningEffort(&apicompat.AnthropicRequest{
		OutputConfig: &apicompat.AnthropicOutputConfig{Effort: "medium"},
	}, "deepseek-reasoner", "medium"))
	require.Equal(t, "medium", openAICompatAnthropicReasoningEffort(nil, "gpt-5.4", "medium"))
}

func TestChannel_IsResponseModelExposed(t *testing.T) {
	var nilCh *Channel
	require.False(t, nilCh.IsResponseModelExposed())
	require.False(t, (&Channel{}).IsResponseModelExposed())
	require.False(t, (&Channel{FeaturesConfig: map[string]any{"expose_response_model": false}}).IsResponseModelExposed())
	require.True(t, (&Channel{FeaturesConfig: map[string]any{"expose_response_model": true}}).IsResponseModelExposed())

	svc := &ChannelService{}
	cache := newEmptyChannelCache()
	cache.loadedAt = time.Now()
	cache.byID[1] = &Channel{ID: 1, Status: StatusActive, FeaturesConfig: map[string]any{"expose_response_model": true}}
	cache.byID[2] = &Channel{ID: 2, Status: "inactive", FeaturesConfig: map[string]any{"expose_response_model": true}}
	cache.byID[3] = &Channel{ID: 3, Status: StatusActive, FeaturesConfig: map[string]any{"expose_response_model": false}}
	svc.cache.Store(cache)

	exposed := svc.ResponseModelExposedChannels(context.Background(), []int64{1, 2, 3, 999})
	require.Equal(t, map[int64]bool{1: true}, exposed)

	// Sanity check gin unused import guard in case future refactors move helpers
	_ = httptest.NewRecorder
	_ = http.StatusOK
	_ = gin.TestMode
}

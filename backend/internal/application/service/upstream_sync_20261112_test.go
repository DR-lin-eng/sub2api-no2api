package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestClaude52PriceCardAliasesAndCacheTTLs(t *testing.T) {
	for _, model := range []string{"claude-fable-5-2", "claude-mythos-5-2", "anthropic/claude-fable-5-2-high", "us.anthropic.claude-mythos-5-2-v1:0"} {
		billing := NewBillingService(&config.Config{}, nil)
		cost, err := billing.CalculateCost(model, UsageTokens{InputTokens: 100, OutputTokens: 50, CacheCreationTokens: 60, CacheCreation5mTokens: 20, CacheCreation1hTokens: 40, CacheReadTokens: 80}, 1)
		require.NoError(t, err, model)
		require.InDelta(t, 100*8e-6+50*40e-6+20*10e-6+40*16e-6+80*.2e-6, cost.TotalCost, 1e-12, model)
	}
	dynamic := &PricingService{pricingData: map[string]*LiteLLMModelPricing{"claude-fable-5-2": {InputCostPerToken: 0.123}}}
	p, err := NewBillingService(&config.Config{}, dynamic).GetModelPricing("claude-fable-5-2")
	require.NoError(t, err)
	require.Equal(t, 0.123, p.InputPricePerToken)
	require.Nil(t, (&PricingService{}).GetModelPricing("claude-fable-5-20"), "do not match another minor version")
}

func TestPaymentMultiplierPreservesSixDecimalsAndCurrencySettlement(t *testing.T) {
	repo := &paymentConfigSettingRepoStub{}
	svc := &PaymentConfigService{settingRepo: repo}
	multiplier := 0.140845
	require.NoError(t, svc.UpdatePaymentConfig(context.Background(), UpdatePaymentConfigRequest{BalanceRechargeMultiplier: &multiplier}))
	require.Equal(t, "0.140845", repo.values[SettingBalanceRechargeMult])
	cfg := svc.parsePaymentConfig(repo.values)
	require.Equal(t, multiplier, cfg.BalanceRechargeMultiplier)
	require.Equal(t, 14.08, quoteRechargeBonus(cfg, 100, "CNY").Credited)
	for _, bad := range []float64{0, -1, 0.1408451, 1e-13, math.SmallestNonzeroFloat64, math.NaN(), math.Inf(1)} {
		require.Error(t, svc.UpdatePaymentConfig(context.Background(), UpdatePaymentConfigRequest{BalanceRechargeMultiplier: &bad}))
	}
	minimum := 0.000001
	require.NoError(t, svc.UpdatePaymentConfig(context.Background(), UpdatePaymentConfigRequest{BalanceRechargeMultiplier: &minimum}))
	require.Equal(t, "0.000001", repo.values[SettingBalanceRechargeMult])
	large := math.MaxFloat64
	require.NoError(t, svc.UpdatePaymentConfig(context.Background(), UpdatePaymentConfigRequest{BalanceRechargeMultiplier: &large}))
	require.Equal(t, large, svc.parsePaymentConfig(repo.values).BalanceRechargeMultiplier)
}

type mismatchedCatalogRemote struct{ solCatalogRemote }

func (*mismatchedCatalogRemote) FetchHashText(context.Context, string) (string, error) {
	return "out-of-sync", nil
}

func TestPricingHashMismatchKeepsDownloadedBodyAsSyncAnchor(t *testing.T) {
	remote := &mismatchedCatalogRemote{solCatalogRemote{body: []byte(`{"fixture":{"input_cost_per_token":0.000001,"output_cost_per_token":0.000002}}`)}}
	svc := NewPricingService(&config.Config{Pricing: config.PricingConfig{RemoteURL: "https://fixture.test/prices", HashURL: "https://fixture.test/hash", DataDir: t.TempDir()}}, remote)
	require.NoError(t, svc.downloadPricingData())
	sum := sha256.Sum256(remote.body)
	require.Equal(t, hex.EncodeToString(sum[:]), svc.localHash)
}

func TestResponsesSessionSeedIgnoresRemindersAfterToolLoop(t *testing.T) {
	base := []byte(`{"model":"gpt-5.4","input":[{"role":"developer","content":"stable"},{"role":"user","content":"initial"}]}`)
	later := []byte(`{"model":"gpt-5.4","input":[{"role":"developer","content":"stable"},{"role":"user","content":"initial"},{"type":"function_call","call_id":"call_1","name":"read","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"done"},{"role":"developer","content":"dynamic reminder"}]}`)
	require.Equal(t, deriveOpenAIContentSessionSeed(base), deriveOpenAIContentSessionSeed(later))
}

func TestAntigravityProArgonCooldownDoesNotBlockFlash(t *testing.T) {
	for _, plan := range []string{"Pro", "g1-pro-tier", "Ultra", "unknown"} {
		account := &Account{Platform: PlatformAntigravity, Credentials: map[string]any{"plan_type": plan}}
		keys := antigravityModelRateLimitKeys(account, "gemini-4-argon-high")
		if plan == "Pro" || plan == "g1-pro-tier" {
			require.Contains(t, keys, antigravityArgonModelRateLimitKey)
			require.NotContains(t, keys, antigravityGeminiModelRateLimitKey)
		} else {
			require.Contains(t, keys, antigravityGeminiModelRateLimitKey)
		}
		require.Contains(t, antigravityModelRateLimitKeys(account, "gemini-3.8-flash"), antigravityGeminiModelRateLimitKey)
	}
	cache := &AccountUsageService{cache: NewUsageCache()}
	window := &UsageProgress{RemainingSeconds: 7}
	cache.cache.antigravityCache.Store(int64(3), &antigravityUsageCache{usageInfo: &UsageInfo{FiveHour: window}, timestamp: time.Now()})
	read, ok := cache.cachedAntigravityUsage(3, false)
	require.True(t, ok)
	read.FiveHour.RemainingSeconds = 999
	require.Equal(t, 7, window.RemainingSeconds)
	_, ok = cache.cachedAntigravityUsage(3, true)
	require.False(t, ok)
}

func TestGeminiNonstreamThoughtsRemainUsageWithoutVisibleText(t *testing.T) {
	raw := []byte(`{"candidates":[{"content":{"parts":[{"thought":true,"text":"private summary"},{"text":"answer"}]}}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":3,"thoughtsTokenCount":7}}`)
	var response map[string]any
	require.NoError(t, json.Unmarshal(raw, &response))
	for _, images := range []bool{true, false} {
		message, usage := convertGeminiToClaudeMessage(response, "gemini-test", raw, images)
		wire, err := json.Marshal(message)
		require.NoError(t, err)
		require.NotContains(t, string(wire), "private summary")
		require.Contains(t, string(wire), "answer")
		require.Equal(t, 10, usage.OutputTokens)
	}
}

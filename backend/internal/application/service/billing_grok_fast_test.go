//go:build unit

package service

import (
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func pr8170BundledPricing(t *testing.T) *PricingService {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)
	svc := &PricingService{}
	prices, err := svc.parsePricingData(data)
	require.NoError(t, err)
	svc.pricingData = prices
	svc.identifiedPricingIndex = buildIdentifiedModelPricingIndex(prices)
	return svc
}

func TestPR8170FastPriceAndBoundary(t *testing.T) {
	sources := map[string]*BillingService{"fallback": NewBillingService(&config.Config{}, nil), "bundled": NewBillingService(&config.Config{}, pr8170BundledPricing(t))}
	for source, svc := range sources {
		t.Run(source, func(t *testing.T) {
			for _, model := range []string{"grok-4.7-fast", "grok-4.7-build-fast", "GROK-4.7-FAST", "xai/grok-4.7-fast", "x-ai/grok-4.7-build-fast", "grok/grok-4.7-fast"} {
				t.Run(model, func(t *testing.T) {
					pricing, err := svc.GetModelPricing(model)
					require.NoError(t, err)
					require.InDelta(t, 4e-6, pricing.InputPricePerToken, 1e-15)
					require.InDelta(t, 1e-6, pricing.CacheReadPricePerToken, 1e-15)
					require.InDelta(t, 12e-6, pricing.OutputPricePerToken, 1e-15)
					require.Equal(t, 200000, pricing.LongContextInputThreshold)
					require.True(t, pricing.LongContextThresholdInclusive)
					require.True(t, svc.HasIdentifiedTokenPricing(model))
					for _, input := range []int{100000, 149999, 150000, 150001} {
						multiplier := 1.0
						if input+50000 >= 200000 {
							multiplier = 1.5
						}
						cost, err := svc.CalculateCost(model, UsageTokens{InputTokens: input, CacheReadTokens: 50000, OutputTokens: 10000}, 2)
						require.NoError(t, err)
						require.InDelta(t, float64(input)*4e-6*multiplier, cost.InputCost, 1e-12)
						require.InDelta(t, .05*multiplier, cost.CacheReadCost, 1e-12)
						require.InDelta(t, .12*multiplier, cost.OutputCost, 1e-12)
						require.Equal(t, multiplier > 1, cost.LongContextBillingApplied)
						require.InDelta(t, cost.TotalCost*2, cost.ActualCost, 1e-12)
					}
				})
			}
		})
	}
}

func TestPR8170CustomPricesStillWin(t *testing.T) {
	ps := &PricingService{cfg: &config.Config{}}
	prices, err := ps.parsePricingData([]byte(`{"grok-4.7-fast":{"input_cost_per_token":0.000009,"cache_read_input_token_cost":0.000003,"output_cost_per_token":0.000017,"litellm_provider":"xai"},"grok-4.7-build-fast":{"input_cost_per_token":0.000008,"cache_read_input_token_cost":0.000002,"output_cost_per_token":0.000016,"litellm_provider":"xai"}}`))
	require.NoError(t, err)
	ps.cfg.Pricing.FallbackFile = filepath.Join("..", "..", "..", "resources", "model-pricing", "model_prices_and_context_window.json")
	ps.pricingData = ps.mergeFallbackPricingData(prices)
	svc := NewBillingService(&config.Config{}, ps)
	for _, tc := range []struct {
		model                string
		input, cache, output float64
	}{{"grok-4.7-fast", 9e-6, 3e-6, 17e-6}, {"grok-4.7-build-fast", 8e-6, 2e-6, 16e-6}} {
		price, err := svc.GetModelPricing(tc.model)
		require.NoError(t, err)
		require.InDelta(t, tc.input, price.InputPricePerToken, 1e-15)
		require.InDelta(t, tc.cache, price.CacheReadPricePerToken, 1e-15)
		require.InDelta(t, tc.output, price.OutputPricePerToken, 1e-15)
		require.Zero(t, price.LongContextInputThreshold, "Do not impose default long-context policy on explicit custom cards")
	}
	input, output, cache := 7e-6, 11e-6, 2e-6
	price, err := svc.GetModelPricingWithChannel("grok-4.7-fast", &ChannelModelPricing{InputPrice: &input, OutputPrice: &output, CacheReadPrice: &cache})
	require.NoError(t, err)
	require.Equal(t, input, price.InputPricePerToken)
	require.Equal(t, output, price.OutputPricePerToken)
	require.Equal(t, cache, price.CacheReadPricePerToken)
}

func TestPR8170OnlyDeclaredFastIDsChange(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	regular, err := svc.GetModelPricing("grok-4.6")
	require.NoError(t, err)
	require.InDelta(t, 2e-6, regular.InputPricePerToken, 1e-15)
	require.InDelta(t, .5e-6, regular.CacheReadPricePerToken, 1e-15)
	require.InDelta(t, 6e-6, regular.OutputPricePerToken, 1e-15)
	for _, name := range []string{"grok-4.7", "grok-4.7-fast-beta", "vendor/grok-4.7-fast", "grok-imagine-image"} {
		require.Empty(t, grok47FastPricingID(name), name)
	}
	require.Empty(t, grok47FastPricingID("grok-4.7-build"))
	require.True(t, svc.getFallbackPricing("grok-4.7-fast-beta") == svc.fallbackPrices["grok-4.5"])
}

package service

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestGeminiNeonFallbackBillingByServiceTier(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000}

	tests := []struct {
		name        string
		serviceTier string
		input       float64
		output      float64
		cacheRead   float64
	}{
		{name: "standard", input: 3, output: 15, cacheRead: 0.15},
		{name: "priority", serviceTier: "priority", input: 5.4, output: 27, cacheRead: 0.27},
		{name: "flex", serviceTier: "flex", input: 1.5, output: 7.5, cacheRead: 0.075},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost, err := svc.CalculateCostWithServiceTier("gemini-4-neon", tokens, 1, tt.serviceTier)
			require.NoError(t, err)
			require.InDelta(t, tt.input, cost.InputCost, 1e-12)
			require.InDelta(t, tt.output, cost.OutputCost, 1e-12)
			require.InDelta(t, tt.cacheRead, cost.CacheReadCost, 1e-12)
			require.InDelta(t, tt.input+tt.output+tt.cacheRead, cost.TotalCost, 1e-12)
		})
	}
}

func TestDefaultPricingIncludesGeminiNeonOfficialRates(t *testing.T) {
	data, err := os.ReadFile("../../../resources/model-pricing/model_prices_and_context_window.json")
	require.NoError(t, err)

	pricingSvc := &PricingService{}
	cards, err := pricingSvc.parsePricingData(data)
	require.NoError(t, err)
	pricingSvc.pricingData = cards

	pricing := pricingSvc.GetModelPricing("gemini-4-neon")
	require.NotNil(t, pricing)
	require.Equal(t, "gemini", pricing.LiteLLMProvider)
	require.InDelta(t, 3e-6, pricing.InputCostPerToken, 1e-15)
	require.InDelta(t, 15e-6, pricing.OutputCostPerToken, 1e-15)
	require.InDelta(t, 0.15e-6, pricing.CacheReadInputTokenCost, 1e-15)
	require.InDelta(t, 5.4e-6, pricing.InputCostPerTokenPriority, 1e-15)
	require.InDelta(t, 27e-6, pricing.OutputCostPerTokenPriority, 1e-15)
	require.InDelta(t, 0.27e-6, pricing.CacheReadInputTokenCostPriority, 1e-15)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	var card map[string]any
	require.NoError(t, json.Unmarshal(raw["gemini-4-neon"], &card))
	require.Equal(t, float64(1_048_576), card["max_input_tokens"])
	require.Equal(t, float64(65_536), card["max_output_tokens"])
	require.Equal(t, float64(65_536), card["max_tokens"])
}

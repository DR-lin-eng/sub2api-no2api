package service

import (
	"os"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestGeminiNonPromotionalFallbackBilling(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	tokens := UsageTokens{InputTokens: 1_000_000, OutputTokens: 1_000_000, CacheReadTokens: 1_000_000}

	for _, model := range []string{
		"gemini-3.7-flash", "gemini-3.7-flash-high",
		"gemini-3.8-flash", "gemini-3.8-flash-tiered",
	} {
		t.Run(model, func(t *testing.T) {
			cost, err := svc.CalculateCost(model, tokens, 1)
			require.NoError(t, err)
			require.InDelta(t, 1.5, cost.InputCost, 1e-12)
			require.InDelta(t, 7.5, cost.OutputCost, 1e-12)
			require.InDelta(t, 0.15, cost.CacheReadCost, 1e-12)
			require.InDelta(t, 9.15, cost.TotalCost, 1e-12)
		})
	}

	for _, model := range []string{"gemini-4-argon", "gemini-4-argon-high"} {
		t.Run(model, func(t *testing.T) {
			cost, err := svc.CalculateCost(model, tokens, 1)
			require.NoError(t, err)
			require.InDelta(t, 4, cost.InputCost, 1e-12)
			require.InDelta(t, 20, cost.OutputCost, 1e-12)
			require.InDelta(t, 0.2, cost.CacheReadCost, 1e-12)
			require.InDelta(t, 24.2, cost.TotalCost, 1e-12)

			priority, err := svc.CalculateCostWithServiceTier(model, tokens, 1, "priority")
			require.NoError(t, err)
			require.InDelta(t, 7.2, priority.InputCost, 1e-12)
			require.InDelta(t, 36, priority.OutputCost, 1e-12)
			require.InDelta(t, 0.36, priority.CacheReadCost, 1e-12)
			require.InDelta(t, 43.56, priority.TotalCost, 1e-12)
		})
	}
}

func TestDefaultCatalogGeminiFlashCardsUseStandardRates(t *testing.T) {
	data, err := os.ReadFile("../../../resources/model-pricing/model_prices_and_context_window.json")
	require.NoError(t, err)

	pricingSvc := &PricingService{}
	cards, err := pricingSvc.parsePricingData(data)
	require.NoError(t, err)
	pricingSvc.pricingData = cards

	for _, model := range []string{"gemini-3.7-flash", "gemini-3.8-flash"} {
		pricing := pricingSvc.GetIdentifiedModelPricing(model)
		require.NotNil(t, pricing)
		require.InDelta(t, 1.5e-6, pricing.InputCostPerToken, 1e-15)
		require.InDelta(t, 7.5e-6, pricing.OutputCostPerToken, 1e-15)
		require.InDelta(t, 0.15e-6, pricing.CacheReadInputTokenCost, 1e-15)
	}
}

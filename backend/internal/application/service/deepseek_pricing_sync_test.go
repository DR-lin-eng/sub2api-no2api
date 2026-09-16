//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDeepseekPricingSyncProRoutesToFlashAtCutoff(t *testing.T) {
	bs := newTestBillingService()

	before, err := bs.getModelPricingAt("deepseek-v4-pro", deepseekProRoutesToFlashAt.Add(-time.Second))
	require.NoError(t, err)
	require.InDelta(t, deepseekProOffPeakInputPrice, before.InputPricePerToken, 1e-15)
	require.InDelta(t, deepseekProOffPeakOutputPrice, before.OutputPricePerToken, 1e-15)
	require.InDelta(t, deepseekProOffPeakCacheRead, before.CacheReadPricePerToken, 1e-15)

	atCutoff, err := bs.getModelPricingAt("deepseek-v4-pro", deepseekProRoutesToFlashAt)
	require.NoError(t, err)
	require.InDelta(t, deepseekFlashOffPeakInputPrice, atCutoff.InputPricePerToken, 1e-15)
	require.InDelta(t, deepseekFlashOffPeakOutputPrice, atCutoff.OutputPricePerToken, 1e-15)
	require.InDelta(t, deepseekFlashOffPeakCacheRead, atCutoff.CacheReadPricePerToken, 1e-15)

	versioned, err := bs.getModelPricingAt("deepseek-v4-pro-0813", deepseekProRoutesToFlashAt)
	require.NoError(t, err)
	require.InDelta(t, deepseekFlashOffPeakInputPrice, versioned.InputPricePerToken, 1e-15)
}

func TestDeepseekPricingSyncPeakMultiplierAndCustomPriceBoundary(t *testing.T) {
	bs := newTestBillingService()
	resolver := NewModelPricingResolver(nil, bs)
	tokens := UsageTokens{InputTokens: 1000, OutputTokens: 500, CacheReadTokens: 1000}
	offPeak := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	peak := time.Date(2026, 9, 14, 6, 0, 0, 0, time.UTC)

	offPeakCost, err := bs.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "deepseek-flash", Tokens: tokens,
		RateMultiplier: 1, Resolver: resolver, PricingAt: offPeak,
	})
	require.NoError(t, err)
	peakCost, err := bs.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "deepseek-flash", Tokens: tokens,
		RateMultiplier: 1, Resolver: resolver, PricingAt: peak,
	})
	require.NoError(t, err)
	require.InDelta(t, offPeakCost.TotalCost*2, peakCost.TotalCost, 1e-12)

	custom := &ResolvedPricing{
		Mode:   BillingModeToken,
		Source: PricingSourceGroup,
		BasePricing: &ModelPricing{
			InputPricePerToken: 1e-6, OutputPricePerToken: 2e-6, CacheReadPricePerToken: 4e-7,
		},
	}
	customCost, err := bs.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: "deepseek-flash", Tokens: tokens,
		RateMultiplier: 1, Resolver: resolver, Resolved: custom, PricingAt: peak,
	})
	require.NoError(t, err)
	require.InDelta(t, 1000*1e-6+500*2e-6+1000*4e-7, customCost.TotalCost, 1e-12)
}

func TestDeepseekPricingSyncResourceCatalog(t *testing.T) {
	for _, model := range []string{
		"deepseek-flash",
		"deepseek-v4-flash",
		"deepseek-v4-flash-vision-exp",
		"deepseek-v4-pro",
	} {
		pricing, err := newTestBillingService().GetModelPricing(model)
		require.NoError(t, err)
		require.NotNil(t, pricing)
	}
}

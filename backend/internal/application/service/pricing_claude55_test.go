package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestClaude55PricingFallbackBoundariesAndCustomPrecedence(t *testing.T) {
	service := NewBillingService(&config.Config{}, nil)
	for _, item := range []struct {
		model                      string
		input, output, write, read float64
	}{{"opus", 4e-6, 20e-6, 5e-6, .2e-6}, {"sonnet", 2e-6, 10e-6, 2.5e-6, .2e-6}, {"haiku", .1e-6, .5e-6, .125e-6, .01e-6}} {
		for _, suffix := range []string{"", "-high"} {
			p, err := service.GetModelPricing("anthropic/claude-" + item.model + "-5-5" + suffix)
			require.NoError(t, err)
			require.InDelta(t, item.input, p.InputPricePerToken, 1e-15)
			require.InDelta(t, item.output, p.OutputPricePerToken, 1e-15)
			require.InDelta(t, item.write, p.CacheCreationPricePerToken, 1e-15)
			require.InDelta(t, item.read, p.CacheReadPricePerToken, 1e-15)
		}
	}
	for _, total := range []int{99999, 100000, 100001} {
		tokens := UsageTokens{InputTokens: total - 50000, CacheCreationTokens: 20000, CacheReadTokens: 30000, OutputTokens: 100}
		cost, err := service.CalculateCost("claude-haiku-5-5", tokens, 1)
		require.NoError(t, err)
		multiplier := 1.
		if total > 100000 {
			multiplier = 5
		}
		require.InDelta(t, float64(tokens.InputTokens)*.1e-6*multiplier, cost.InputCost, 1e-12)
		require.InDelta(t, 20000*.125e-6*multiplier, cost.CacheCreationCost, 1e-12)
		require.InDelta(t, 30000*.01e-6*multiplier, cost.CacheReadCost, 1e-12)
		require.InDelta(t, 100*.5e-6*multiplier, cost.OutputCost, 1e-12)
	}
	dynamic := &PricingService{pricingData: map[string]*LiteLLMModelPricing{"claude-opus-5": {InputCostPerToken: 99}, "claude-haiku-5-5": {InputCostPerToken: .2e-6, OutputCostPerToken: 1e-6}}}
	p, err := NewBillingService(&config.Config{}, dynamic).GetModelPricing("claude-opus-5-5")
	require.NoError(t, err)
	require.InDelta(t, 4e-6, p.InputPricePerToken, 1e-12)
	p, err = NewBillingService(&config.Config{}, dynamic).GetModelPricing("claude-haiku-5-5")
	require.NoError(t, err)
	require.InDelta(t, .2e-6, p.InputPricePerToken, 1e-12)
	require.Equal(t, 100000, p.LongContextInputThreshold)
	custom := &ModelPricing{InputPricePerToken: 7e-6}
	require.Same(t, custom, applyHaiku55DefaultPolicy("claude-haiku-5-5", custom, false))
	require.Zero(t, custom.LongContextInputThreshold)
}

func TestClaude55CustomResolvedPriceDoesNotGainProviderMultiplier(t *testing.T) {
	service := NewBillingService(&config.Config{}, nil)
	custom := &ModelPricing{InputPricePerToken: 7e-6, OutputPricePerToken: 9e-6, CacheCreationPricePerToken: 2e-6, CacheReadPricePerToken: 1e-6}
	for _, source := range []string{PricingSourceChannel, PricingSourceGroup} {
		resolved := &ResolvedPricing{BasePricing: custom, Source: source}
		cost, err := service.calculateTokenCost(resolved, CostInput{Model: "claude-haiku-5-5", Tokens: UsageTokens{InputTokens: 100001, OutputTokens: 100}, RateMultiplier: 1, Resolver: &ModelPricingResolver{}})
		require.NoError(t, err)
		require.InDelta(t, 100001*7e-6, cost.InputCost, 1e-12)
		require.InDelta(t, 100*9e-6, cost.OutputCost, 1e-12)
	}
}

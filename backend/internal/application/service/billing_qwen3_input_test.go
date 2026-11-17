//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQwen3InputFallbackBilling(t *testing.T) {
	bs := newTestBillingService()
	for _, tt := range []struct {
		model string
		cost  float64 // 1000 input tokens; output must remain free.
	}{
		{"Qwen/Qwen3-Embedding-8B", 0.00004},
		{"qwen3-embedding-8b", 0.00004},
		{"siliconflow/Qwen/Qwen3-Embedding-8B", 0.00004},
		{"Qwen/Qwen3-Embedding-4B", 0.00002},
		{"Qwen/Qwen3-Embedding-0.6B", 0.00001},
		{"qwen3-embedding-0-6b", 0.00001},
		{"Qwen/Qwen3-Reranker-8B", 0.00004},
		{"qwen3-reranker-8b", 0.00004},
		{"Qwen/Qwen3-Reranker-0.6B", 0.00001},
		{"qwen3-reranker-0-6b", 0.00001},
	} {
		t.Run(tt.model, func(t *testing.T) {
			cost, err := bs.CalculateCost(tt.model, UsageTokens{InputTokens: 1000, OutputTokens: 500}, 2)
			require.NoError(t, err)
			require.InDelta(t, tt.cost, cost.TotalCost, 1e-12)
			require.InDelta(t, 2*tt.cost, cost.ActualCost, 1e-12)
			require.Zero(t, cost.OutputCost)
		})
	}
	for _, model := range []string{
		"Qwen/Qwen3-Embedding-2B", "Qwen/Qwen3-Reranker-2B", "Qwen/Qwen3-Reranker-4B",
		"Qwen/Qwen3-VL-Reranker-8B", "bge-reranker-v2-m3", "qwen3-embedding-8billion",
		"qwen3-embedding-8b-preview", "my-qwen3-reranker-8b", "unknown/Qwen3-Embedding-8B",
	} {
		t.Run(model, func(t *testing.T) {
			_, err := bs.GetModelPricing(model)
			require.ErrorIs(t, err, ErrModelPricingUnavailable)
		})
	}
}

func TestQwen3InputExplicitPricingPrecedence(t *testing.T) {
	const model = "Qwen/Qwen3-Embedding-8B"
	bs := newTestBillingService()
	bs.pricingService = &PricingService{pricingData: map[string]*LiteLLMModelPricing{
		"qwen/qwen3-embedding-8b": {InputCostPerToken: 2e-6},
	}}
	tokens := UsageTokens{InputTokens: 1000}
	dynamic, err := bs.CalculateCost(model, tokens, 1)
	require.NoError(t, err)
	require.InDelta(t, 0.002, dynamic.TotalCost, 1e-12)

	channel := &ChannelModelPricing{InputPrice: testPtrFloat64(3e-6), OutputPrice: testPtrFloat64(0)}
	card, err := bs.GetModelPricingWithChannel(model, channel)
	require.NoError(t, err)
	require.InDelta(t, 3e-6, card.InputPricePerToken, 1e-15)

	group := &Group{ID: 7, ModelPricing: []ChannelModelPricing{testGroupPricing(4e-6, 0, model)}}
	group.compileModelPricingIndex()
	cost, err := bs.CalculateCostUnified(CostInput{
		Ctx: context.Background(), Model: model, Group: group, Tokens: tokens,
		RateMultiplier: 1, Resolver: NewModelPricingResolver(nil, bs),
	})
	require.NoError(t, err)
	require.InDelta(t, 0.004, cost.TotalCost, 1e-12)
	unchanged, err := bs.GetModelPricing(model)
	require.NoError(t, err)
	require.InDelta(t, 2e-6, unchanged.InputPricePerToken, 1e-15)
}

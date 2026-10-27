package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/modules/modelcatalog"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/stretchr/testify/require"
)

func TestGPT61TerraIdentityAndCapabilities(t *testing.T) {
	for _, model := range []string{"gpt-6.1-terra", "openai/gpt6.1_terra", "gpt-6.1-terra-max", "gpt-6.1-terra-openai-compact"} {
		require.Equal(t, "gpt-6.1-terra", normalizeKnownOpenAICodexModel(model), model)
		require.True(t, isOpenAIGPT6Model(model), model)
		require.True(t, codexManifestKnownImageInputModel(model), model)
		require.True(t, codexManifestKnownPriorityTierModel(model), model)
	}
	require.Empty(t, normalizeKnownOpenAICodexModel("gpt-6.1-terra-unknown"))
	require.False(t, isOpenAIGPT6Model("gpt-6.1-terra-unknown"))
	require.Contains(t, openai.DefaultModelIDs(), "gpt-6.1-terra")
	require.True(t, isOpenAIOAuthServableModel("gpt-6.1-terra"))
	require.True(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-6.1-terra"))
	for _, effort := range []string{"none", "low", "medium", "high", "xhigh", "max"} {
		require.Equal(t, effort, normalizeOpenAIReasoningEffortForModel(effort, "gpt-6.1-terra"))
	}
	require.Equal(t, "none", normalizeOpenAIReasoningEffortForModel("minimal", "gpt-6.1-terra"))
	d := modelcatalog.KnownFallback("gpt-6.1-terra")
	require.JSONEq(t, `1050000`, string(d["context_window"]))
	require.JSONEq(t, `128000`, string(d["max_output_tokens"]))
	require.JSONEq(t, `["none","low","medium","high","xhigh","max"]`, string(d["supported_reasoning_levels"]))
}

func TestGPT61TerraOfficialPricesAndBoundaries(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)
	ps := &PricingService{}
	catalog, err := ps.parsePricingData(data)
	require.NoError(t, err)
	for _, source := range []string{"static", "dynamic-fallback", "bundled"} {
		t.Run(source, func(t *testing.T) {
			var priceService *PricingService
			if source != "static" {
				priceService = &PricingService{pricingData: map[string]*LiteLLMModelPricing{}}
				if source == "bundled" {
					priceService.pricingData = catalog
				}
			}
			svc := NewBillingService(&config.Config{}, priceService)
			price, err := svc.GetModelPricing("openai/gpt-6.1-terra")
			require.NoError(t, err)
			require.InDelta(t, 1e-6, price.InputPricePerToken, 1e-15)
			require.InDelta(t, 5e-6, price.OutputPricePerToken, 1e-15)
			require.InDelta(t, 0.05e-6, price.CacheReadPricePerToken, 1e-15)
			require.InDelta(t, 1.25e-6, price.CacheCreationPricePerToken, 1e-15)
			for _, tc := range []struct {
				tier   string
				factor float64
			}{{"default", 1}, {"fast", 2}, {"priority", 2}, {"flex", 0.5}} {
				for _, contextSize := range []int{272000, 272001} {
					tokens := UsageTokens{InputTokens: contextSize - 300, CacheReadTokens: 100, CacheCreationTokens: 200, OutputTokens: 10}
					cost, err := svc.CalculateCostWithServiceTier("gpt-6.1-terra", tokens, 1, tc.tier)
					require.NoError(t, err)
					inputFactor, outputFactor := 1.0, 1.0
					if contextSize > 272000 {
						inputFactor, outputFactor = 2, 1.5
					}
					expected := ((float64(tokens.InputTokens)*1e-6+100*0.05e-6+200*1.25e-6)*inputFactor + 10*5e-6*outputFactor) * tc.factor
					require.InDelta(t, expected, cost.TotalCost, 1e-12, "%s %d", tc.tier, contextSize)
					require.Equal(t, contextSize > 272000, cost.LongContextBillingApplied)
				}
			}
		})
	}
	var raw map[string]map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	card := raw["gpt-6.1-terra"]
	require.InDelta(t, 0.5e-6, card["input_cost_per_token_batches"], 1e-15)
	require.InDelta(t, 2.5e-6, card["output_cost_per_token_flex"], 1e-15)
}

func TestOpenAIUltrafastOfficialPricing(t *testing.T) {
	for _, model := range []string{"gpt-6-astra", "gpt-6.1-sol", "openai/gpt6.1_sol-max"} {
		for _, dynamic := range []bool{false, true} {
			t.Run(model+map[bool]string{true: "/dynamic", false: "/static"}[dynamic], func(t *testing.T) {
				var ps *PricingService
				if dynamic {
					// Custom catalog prices retain precedence, and receive the tier multiplier.
					ps = &PricingService{pricingData: map[string]*LiteLLMModelPricing{model: {
						InputCostPerToken: 3e-6, OutputCostPerToken: 7e-6,
						CacheCreationInputTokenCost: 4e-6, CacheReadInputTokenCost: 0.3e-6,
					}}}
				}
				svc := NewBillingService(&config.Config{}, ps)
				for _, input := range []int{1000, 272001} {
					tokens := UsageTokens{InputTokens: input, CacheReadTokens: 100, CacheCreationTokens: 200, OutputTokens: 10}
					base, err := svc.CalculateCostWithServiceTier(model, tokens, 0.5, "default")
					require.NoError(t, err)
					ultra, err := svc.CalculateCostWithServiceTier(model, tokens, 0.5, " ULTRAFAST ")
					require.NoError(t, err)
					require.InDelta(t, base.TotalCost*6, ultra.TotalCost, 1e-12)
					require.InDelta(t, base.ActualCost*6, ultra.ActualCost, 1e-12)
					require.InDelta(t, base.CacheReadCost*6, ultra.CacheReadCost, 1e-12)
					require.InDelta(t, base.CacheCreationCost*6, ultra.CacheCreationCost, 1e-12)
				}
			})
		}
	}
	// Models without an official Ultrafast card retain their existing tier behavior.
	svc := NewBillingService(&config.Config{}, nil)
	for _, model := range []string{"gpt-6.1-terra", "gpt-6-sol", "gpt-6-luna", "gpt-5.6-sol"} {
		tokens := UsageTokens{InputTokens: 1000, OutputTokens: 10}
		base, err := svc.CalculateCost(model, tokens, 1)
		require.NoError(t, err)
		ultra, err := svc.CalculateCostWithServiceTier(model, tokens, 1, "ultrafast")
		require.NoError(t, err)
		require.InDelta(t, base.TotalCost, ultra.TotalCost, 1e-12)
	}
}

func TestOpenAIUltrafastUnifiedChannelPricing(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	resolver := NewModelPricingResolver(nil, svc)
	resolved := &ResolvedPricing{Mode: BillingModeToken, Source: PricingSourceChannel,
		BasePricing: &ModelPricing{InputPricePerToken: 3e-6, OutputPricePerToken: 7e-6,
			CacheCreationPricePerToken: 4e-6, CacheReadPricePerToken: 0.3e-6,
			ApplyServiceTierMultiplier: true}}
	input := CostInput{Ctx: context.Background(), Model: "gpt-6.1-sol", RateMultiplier: 1,
		Tokens:   UsageTokens{InputTokens: 1000, OutputTokens: 10, CacheCreationTokens: 200, CacheReadTokens: 100},
		Resolver: resolver, Resolved: resolved}
	base, err := svc.CalculateCostUnified(input)
	require.NoError(t, err)
	input.ServiceTier = "ultrafast"
	ultra, err := svc.CalculateCostUnified(input)
	require.NoError(t, err)
	require.InDelta(t, base.TotalCost*6, ultra.TotalCost, 1e-12)
	require.Zero(t, resolved.BasePricing.LongContextInputThreshold, "shared channel pricing must stay immutable")
}

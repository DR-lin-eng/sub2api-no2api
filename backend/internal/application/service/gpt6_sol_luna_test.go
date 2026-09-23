package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestGPT6SolAndLunaModelIdentityAndCapabilities(t *testing.T) {
	for _, model := range []string{"gpt-6-sol", "gpt-6-luna"} {
		t.Run(model, func(t *testing.T) {
			require.Equal(t, model, normalizeKnownOpenAICodexModel(model))
			require.Equal(t, model, normalizeKnownOpenAICodexModel("openai/"+model+"-2026-09-22"))
			require.Equal(t, model, normalizeKnownOpenAICodexModel("gpt6_"+model[len("gpt-6-"):]+"-max"))
			require.True(t, isOpenAIOAuthServableModel(model))
			require.True(t, isOpenAIGPT6Model(model))
			require.True(t, codexManifestKnownImageInputModel(model))
			require.True(t, codexManifestKnownPriorityTierModel(model))
			require.True(t, shouldAutoInjectPromptCacheKeyForCompat(model))
			require.Equal(t, "max", normalizeOpenAIReasoningEffortForModel("max", model))
		})
	}

	require.Empty(t, normalizeKnownOpenAICodexModel("gpt-6-orion"))
	require.False(t, isOpenAIGPT6Model("gpt-6-orion"))
	require.False(t, shouldAutoInjectPromptCacheKeyForCompat("gpt-6-orion"))
}

func TestGPT6SolAndLunaOfficialFallbackPricing(t *testing.T) {
	tests := []struct {
		model                             string
		input, cached, cacheWrite, output float64
		inputFast, cachedFast, writeFast  float64
		outputFast                        float64
	}{
		{model: "gpt-6-sol", input: 2e-6, cached: 0.2e-6, cacheWrite: 2.5e-6, output: 10e-6, inputFast: 4e-6, cachedFast: 0.4e-6, writeFast: 5e-6, outputFast: 20e-6},
		{model: "gpt-6-luna", input: 0.1e-6, cached: 0.01e-6, cacheWrite: 0.125e-6, output: 0.5e-6, inputFast: 0.2e-6, cachedFast: 0.02e-6, writeFast: 0.25e-6, outputFast: 1e-6},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			svc := NewBillingService(&config.Config{}, nil)
			pricing, err := svc.GetModelPricing(tt.model)
			require.NoError(t, err)
			require.InDelta(t, tt.input, pricing.InputPricePerToken, 1e-15)
			require.InDelta(t, tt.cached, pricing.CacheReadPricePerToken, 1e-15)
			require.InDelta(t, tt.cacheWrite, pricing.CacheCreationPricePerToken, 1e-15)
			require.InDelta(t, tt.output, pricing.OutputPricePerToken, 1e-15)
			require.Equal(t, 272000, pricing.LongContextInputThreshold)

			tokens := UsageTokens{InputTokens: 1000, CacheReadTokens: 100, CacheCreationTokens: 200, OutputTokens: 10}
			standard, err := svc.CalculateCostWithServiceTier(tt.model, tokens, 1, "")
			require.NoError(t, err)
			require.InDelta(t, 1000*tt.input+100*tt.cached+200*tt.cacheWrite+10*tt.output, standard.TotalCost, 1e-12)

			fast, err := svc.CalculateCostWithServiceTier(tt.model, tokens, 1, "fast")
			require.NoError(t, err)
			require.InDelta(t, 1000*tt.inputFast+100*tt.cachedFast+200*tt.writeFast+10*tt.outputFast, fast.TotalCost, 1e-12)

			long, err := svc.CalculateCostWithServiceTier(tt.model, UsageTokens{InputTokens: 272001, OutputTokens: 1}, 1, "")
			require.NoError(t, err)
			require.True(t, long.LongContextBillingApplied)
			require.InDelta(t, 272001*tt.input*2, long.InputCost, 1e-9)
			require.InDelta(t, tt.output*1.5, long.OutputCost, 1e-15)
		})
	}
}

func TestGPT6SolAndLunaBundledPricingCatalog(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "resources", "model-pricing", "model_prices_and_context_window.json"))
	require.NoError(t, err)

	pricingSvc := &PricingService{}
	catalog, err := pricingSvc.parsePricingData(data)
	require.NoError(t, err)
	for model, input := range map[string]float64{"gpt-6-sol": 2e-6, "gpt-6-luna": 0.1e-6} {
		pricing := catalog[model]
		require.NotNil(t, pricing, model)
		require.InDelta(t, input, pricing.InputCostPerToken, 1e-15, model)
		require.Equal(t, 272000, pricing.LongContextInputTokenThreshold, model)
		require.InDelta(t, 2.0, pricing.LongContextInputCostMultiplier, 1e-12, model)
		require.InDelta(t, 1.5, pricing.LongContextOutputCostMultiplier, 1e-12, model)
		require.True(t, pricing.SupportsServiceTier, model)
	}

	var raw map[string]struct {
		MaxInputTokens              int  `json:"max_input_tokens"`
		MaxOutputTokens             int  `json:"max_output_tokens"`
		SupportsNoneReasoningEffort bool `json:"supports_none_reasoning_effort"`
		SupportsMaxReasoningEffort  bool `json:"supports_max_reasoning_effort"`
	}
	require.NoError(t, json.Unmarshal(data, &raw))
	for _, model := range []string{"gpt-6-sol", "gpt-6-luna"} {
		entry, ok := raw[model]
		require.True(t, ok, model)
		require.Equal(t, 922000, entry.MaxInputTokens, model)
		require.Equal(t, 128000, entry.MaxOutputTokens, model)
		require.True(t, entry.SupportsNoneReasoningEffort, model)
		require.True(t, entry.SupportsMaxReasoningEffort, model)
	}
}

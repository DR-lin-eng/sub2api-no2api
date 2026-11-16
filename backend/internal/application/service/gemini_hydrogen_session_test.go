package service

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/gemini"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/geminicli"
	"github.com/stretchr/testify/require"
)

func TestGeminiHydrogenPricingAndDiscovery(t *testing.T) {
	svc := NewBillingService(&config.Config{}, nil)
	for _, model := range []string{"gemini-4-hydrogen", "models/gemini-4-hydrogen", "projects/p/locations/global/publishers/google/models/gemini-4-hydrogen"} {
		for _, tier := range []string{"", "priority"} {
			cost, err := svc.CalculateCostWithServiceTier(model, UsageTokens{InputTokens: 1000000, OutputTokens: 1000000, CacheReadTokens: 1000000}, 1, tier)
			require.NoError(t, err)
			mult := 1.0
			if tier == "priority" {
				mult = 1.8
			}
			require.InDelta(t, 1.5*mult, cost.InputCost, 1e-12)
			require.InDelta(t, 7.5*mult, cost.OutputCost, 1e-12)
			require.InDelta(t, 0.075*mult, cost.CacheReadCost, 1e-12)
		}
	}
	data, err := os.ReadFile("../../../resources/model-pricing/model_prices_and_context_window.json")
	require.NoError(t, err)
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Contains(t, raw, "gemini-4-hydrogen")
	pricing := NewPricingService(&config.Config{}, nil)
	cards, err := pricing.parsePricingData(data)
	require.NoError(t, err)
	require.Contains(t, cards, "gemini-4-hydrogen")
	found := false
	for _, model := range geminicli.DefaultModels {
		if model.ID == "gemini-4-hydrogen" {
			found = true
		}
	}
	require.True(t, found)
	require.Equal(t, "models/gemini-4-hydrogen", gemini.FallbackModel("gemini-4-hydrogen").Name)
}

func TestGeminiContentSessionStablePrefixAndTenantIsolation(t *testing.T) {
	first := []byte(`{"systemInstruction":{"parts":[{"text":"system"}]},"tools":[{"functionDeclarations":[{"name":"fixture"}]}],"contents":[{"role":"user","parts":[{"text":"first"}]},{"role":"model","parts":[{"text":"old tail"}]}]}`)
	next := []byte(`{ "contents":[{"parts":[{"text":"first"}],"role":"user"},{"role":"model","parts":[{"text":"new tail"}]},{"role":"user","parts":[{"text":"next turn"}]}],"tools":[{"functionDeclarations":[{"name":"fixture"}]}],"systemInstruction":{"parts":[{"text":"system"}]}}`)
	seed := GenerateGeminiContentSessionHash(1, "gemini-4-hydrogen", first)
	require.NotEmpty(t, seed)
	require.Equal(t, seed, GenerateGeminiContentSessionHash(1, "gemini-4-hydrogen", next))
	require.NotEqual(t, seed, GenerateGeminiContentSessionHash(2, "gemini-4-hydrogen", first))
	require.NotEqual(t, seed, GenerateGeminiContentSessionHash(1, "gemini-4-argon", first))
	require.Empty(t, GenerateGeminiContentSessionHash(1, "model", []byte(`{"contents":[{"role":"model","parts":[{"text":"answer"}]}]}`)))
	require.NotEqual(t, GenerateGeminiContentSessionHash(1, "model", []byte(`{"contents":[{"parts":[{"inlineData":{"data":"x","number":9007199254740992}}]}]}`)), GenerateGeminiContentSessionHash(1, "model", []byte(`{"contents":[{"parts":[{"inlineData":{"data":"x","number":9007199254740993}}]}]}`)))
}

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

// The default catalog still used this complete pre-promotion price card on
// 2026-09-10. Keep it separate from the current shipped catalog for regression.
const solLegacyCatalog = `{
	"gpt-5.6-sol": {
		"input_cost_per_token": 0.000005,
		"output_cost_per_token": 0.000030,
		"cache_read_input_token_cost": 0.0000005,
		"cache_creation_input_token_cost": 0.00000625,
		"input_cost_per_token_priority": 0.000010,
		"output_cost_per_token_priority": 0.000060,
		"cache_read_input_token_cost_priority": 0.000001,
		"cache_creation_input_token_cost_priority": 0.0000125,
		"litellm_provider": "openai",
		"mode": "chat",
		"supports_prompt_caching": true
	},
	"gpt-5.5": {"input_cost_per_token": 0.000005, "output_cost_per_token": 0.000030}
}`

type solCatalogRemote struct {
	body []byte
}

func (r *solCatalogRemote) FetchPricingJSON(context.Context, string) ([]byte, error) {
	return r.body, nil
}

func (r *solCatalogRemote) FetchHashText(context.Context, string) (string, error) {
	hash := sha256.Sum256(r.body)
	return hex.EncodeToString(hash[:]), nil
}

func TestPricingService_SolPromotionSurvivesCatalogRefreshAndRestart(t *testing.T) {
	for _, remoteURL := range []string{
		"https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/main/model_prices_and_context_window.json",
		"https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/refs/heads/main//model_prices_and_context_window.json",
	} {
		t.Run(remoteURL, func(t *testing.T) {
			cfg := &config.Config{Pricing: config.PricingConfig{
				RemoteURL: remoteURL,
				DataDir:   t.TempDir(),
			}}
			remote := &solCatalogRemote{body: []byte(solLegacyCatalog)}
			svc := NewPricingService(cfg, remote)
			for _, phase := range []string{"download", "refresh", "restart"} {
				t.Run(phase, func(t *testing.T) {
					if phase == "restart" {
						svc = NewPricingService(cfg, remote)
						require.NoError(t, svc.loadPricingData(svc.getPricingFilePath()))
					} else {
						require.NoError(t, svc.downloadPricingData())
					}
					cached, err := os.ReadFile(svc.getPricingFilePath())
					require.NoError(t, err)
					require.Equal(t, remote.body, cached, "raw cache and synchronization hash stay paired")
					hash := sha256.Sum256(cached)
					require.Equal(t, hex.EncodeToString(hash[:]), svc.localHash)
					pricing := svc.GetIdentifiedModelPricing("gpt-5.6-sol")
					require.NotNil(t, pricing)
					require.InDelta(t, 4e-6, pricing.InputCostPerToken, 1e-12)
					require.InDelta(t, 5e-6, svc.GetModelPricing("gpt-5.5").InputCostPerToken, 1e-12)
					billing := NewBillingService(cfg, svc)
					for _, model := range []string{"gpt-5.6-sol", "gpt-5.6", "openai/gpt-5.6-sol"} {
						p, err := billing.GetModelPricing(model)
						require.NoError(t, err)
						assertGPT56FallbackPricing(t, p, 4e-6, 0.4e-6, 5e-6, 20e-6)
						for _, tier := range []struct {
							name  string
							scale float64
						}{{"", 1}, {"fast", 2}, {"priority", 2}, {"flex", 0.5}} {
							cost, err := billing.CalculateCostWithServiceTier(model, UsageTokens{
								InputTokens: 700, CacheCreationTokens: 200, CacheReadTokens: 100, OutputTokens: 50,
							}, 1, tier.name)
							require.NoError(t, err)
							require.InDelta(t, 0.00484*tier.scale, cost.TotalCost, 1e-12, "model=%s tier=%s", model, tier.name)
						}
					}
				})
			}
		})
	}
}

func TestPricingService_SolPromotionPreservesCustomAndUpdatedCatalogs(t *testing.T) {
	for _, tc := range []struct {
		name, remoteURL string
		input           float64
	}{
		{"custom source", "https://pricing.example.test/models.json", 5e-6},
		{"local-only source", "", 5e-6},
		{"pinned source", "https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/1234567/model_prices_and_context_window.json", 5e-6},
		{"changed upstream rate", "https://raw.githubusercontent.com/Wei-Shaw/model-price-repo/main/model_prices_and_context_window.json", 3e-6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var catalog map[string]map[string]any
			require.NoError(t, json.Unmarshal([]byte(solLegacyCatalog), &catalog))
			catalog["gpt-5.6-sol"]["input_cost_per_token"] = tc.input
			body, err := json.Marshal(catalog)
			require.NoError(t, err)
			svc := NewPricingService(&config.Config{Pricing: config.PricingConfig{
				RemoteURL: tc.remoteURL, DataDir: t.TempDir(),
			}}, nil)
			require.NoError(t, os.WriteFile(svc.getPricingFilePath(), body, 0600))
			require.NoError(t, svc.loadPricingData(svc.getPricingFilePath()))
			p := svc.GetModelPricing("gpt-5.6-sol")
			require.NotNil(t, p)
			require.InDelta(t, tc.input, p.InputCostPerToken, 1e-12)
			require.InDelta(t, 30e-6, p.OutputCostPerToken, 1e-12)
			require.InDelta(t, 12.5e-6, p.CacheCreationInputTokenCostPriority, 1e-12)
		})
	}
}

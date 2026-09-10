package service

import (
	"net/url"
	"path"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
)

// correctDefaultCatalogPricing handles the default feed's known stale Sol
// card, verified against https://developers.openai.com/api/docs/pricing on
// 2026-09-10. Run only when loading a catalog, before publishing its index.
// Custom feeds and pinned revisions keep their explicit prices. A different
// future upstream card must also win over this narrowly matched correction.
func (s *PricingService) correctDefaultCatalogPricing(data map[string]*LiteLLMModelPricing) {
	if s == nil || s.cfg == nil {
		return
	}
	u, err := url.Parse(strings.TrimSpace(s.cfg.Pricing.RemoteURL))
	if err != nil || u.Scheme != "https" || u.Host != "raw.githubusercontent.com" || u.RawQuery != "" {
		return
	}
	switch path.Clean(u.Path) {
	case "/Wei-Shaw/model-price-repo/main/model_prices_and_context_window.json",
		"/Wei-Shaw/model-price-repo/refs/heads/main/model_prices_and_context_window.json":
	default:
		return
	}

	p := data["gpt-5.6-sol"]
	if p == nil || p.LiteLLMProvider != "openai" ||
		p.InputCostPerToken != 5e-6 || p.OutputCostPerToken != 30e-6 ||
		p.CacheReadInputTokenCost != 0.5e-6 || p.CacheCreationInputTokenCost != 6.25e-6 ||
		p.InputCostPerTokenPriority != 10e-6 || p.OutputCostPerTokenPriority != 60e-6 ||
		p.CacheReadInputTokenCostPriority != 1e-6 || p.CacheCreationInputTokenCostPriority != 12.5e-6 {
		return
	}
	current := openAIGPT56SolFallbackPricing
	p.InputCostPerToken = current.InputCostPerToken
	p.OutputCostPerToken = current.OutputCostPerToken
	p.CacheReadInputTokenCost = current.CacheReadInputTokenCost
	p.CacheCreationInputTokenCost = current.CacheCreationInputTokenCost
	p.InputCostPerTokenPriority = current.InputCostPerTokenPriority
	p.OutputCostPerTokenPriority = current.OutputCostPerTokenPriority
	p.CacheReadInputTokenCostPriority = current.CacheReadInputTokenCostPriority
	p.CacheCreationInputTokenCostPriority = current.CacheCreationInputTokenCostPriority
	logger.LegacyPrintf("service.pricing", "%s", "[Pricing] Corrected default catalog GPT-5.6 Sol legacy rates to official promotional rates")
}

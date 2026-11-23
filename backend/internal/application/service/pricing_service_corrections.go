package service

import (
	"net/url"
	"path"
	"strings"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/logger"
)

// correctDefaultCatalogPricing applies narrow price corrections to the default
// feed only. Custom feeds and pinned revisions keep their explicit prices.
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

	correctGeminiNonPromotionalPricing(data)

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

func correctGeminiNonPromotionalPricing(data map[string]*LiteLLMModelPricing) {
	corrected := false
	for _, name := range []string{"gemini-3.7-flash", "gemini-3.8-flash"} {
		p := data[name]
		if p == nil || p.LiteLLMProvider != "gemini" ||
			p.InputCostPerToken != 0.75e-6 || p.OutputCostPerToken != 3.75e-6 ||
			p.CacheReadInputTokenCost != 0.075e-6 {
			continue
		}
		p.InputCostPerToken = 1.5e-6
		p.OutputCostPerToken = 7.5e-6
		p.CacheReadInputTokenCost = 0.15e-6
		corrected = true
	}

	for _, name := range []string{"gemini-4-argon", "gemini-4-carbon"} {
		p := data[name]
		if p == nil || p.LiteLLMProvider != "gemini" ||
			p.InputCostPerToken != 2e-6 || p.OutputCostPerToken != 10e-6 ||
			p.CacheReadInputTokenCost != 0.1e-6 {
			continue
		}
		p.InputCostPerToken = 4e-6
		p.OutputCostPerToken = 20e-6
		p.CacheReadInputTokenCost = 0.2e-6
		if p.InputCostPerTokenPriority == 3.6e-6 && p.OutputCostPerTokenPriority == 18e-6 &&
			p.CacheReadInputTokenCostPriority == 0.18e-6 {
			p.InputCostPerTokenPriority = 7.2e-6
			p.OutputCostPerTokenPriority = 36e-6
			p.CacheReadInputTokenCostPriority = 0.36e-6
		}
		corrected = true
	}
	if corrected {
		logger.LegacyPrintf("service.pricing", "%s", "[Pricing] Corrected default Gemini promo cards to standard rates")
	}
}

package service

import "github.com/Wei-Shaw/sub2api/internal/shared/claude"

func claude55PricingID(model string) string {
	switch {
	case claude.IsOpus55(model):
		return "claude-opus-5-5"
	case claude.IsSonnet55(model):
		return "claude-sonnet-5-5"
	case claude.IsHaiku55(model):
		return "claude-haiku-5-5"
	}
	return ""
}

func (s *BillingService) initClaude55FallbackPricing() {
	s.fallbackPrices["claude-opus-5-5"] = &ModelPricing{
		InputPricePerToken: 4e-6, OutputPricePerToken: 20e-6,
		CacheCreationPricePerToken: 5e-6, CacheReadPricePerToken: .2e-6,
		InputPricePerTokenPriority: 8e-6, OutputPricePerTokenPriority: 40e-6,
		CacheCreationPricePerTokenPriority: 10e-6, CacheReadPricePerTokenPriority: .4e-6,
		CacheCreation5mPrice: 5e-6, CacheCreation1hPrice: 8e-6, SupportsCacheBreakdown: true,
	}
	s.fallbackPrices["claude-sonnet-5-5"] = &ModelPricing{
		InputPricePerToken: 2e-6, OutputPricePerToken: 10e-6,
		CacheCreationPricePerToken: 2.5e-6, CacheReadPricePerToken: .2e-6,
		CacheCreation5mPrice: 2.5e-6, CacheCreation1hPrice: 4e-6, SupportsCacheBreakdown: true,
	}
	s.fallbackPrices["claude-haiku-5-5"] = &ModelPricing{
		InputPricePerToken: .1e-6, OutputPricePerToken: .5e-6,
		CacheCreationPricePerToken: .125e-6, CacheReadPricePerToken: .01e-6,
		CacheCreation5mPrice: .125e-6, CacheCreation1hPrice: .2e-6, SupportsCacheBreakdown: true,
		LongContextInputThreshold: 100000, LongContextInputMultiplier: 5, LongContextOutputMultiplier: 5,
	}
}

// Only default provider rates gain the missing policy. Channel/interval prices
// retain their configured precedence and can disable model context pricing.
func applyHaiku55DefaultPolicy(model string, pricing *ModelPricing, defaultRates bool) *ModelPricing {
	if !defaultRates || !claude.IsHaiku55(model) {
		return pricing
	}
	cloned := *pricing
	if cloned.LongContextInputThreshold <= 0 {
		cloned.LongContextInputThreshold = 100000
	}
	if cloned.LongContextInputMultiplier <= 0 {
		cloned.LongContextInputMultiplier = 5
	}
	if cloned.LongContextOutputMultiplier <= 0 {
		cloned.LongContextOutputMultiplier = 5
	}
	return &cloned
}

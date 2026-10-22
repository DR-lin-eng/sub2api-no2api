package service

import (
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/shared/xai"
)

// Only the two declared Fast IDs use this card; generic and future Grok models
// retain their existing fallback. Provider prefixes do not create new aliases.
func grok47FastPricingID(model string) string {
	switch strings.ToLower(xai.StripGrokProviderPrefix(model)) {
	case "grok-4.7-fast", "grok-4.7-build-fast":
		return "grok-4.7-fast"
	default:
		return ""
	}
}

func (s *BillingService) initGrok47FastFallbackPricing() {
	// https://docs.x.ai/developers/pricing: USD per token, Fast only.
	// Keep the project's xAI >= 200k boundary for the whole request.
	card := &ModelPricing{
		InputPricePerToken: 4e-6, CacheReadPricePerToken: 1e-6, OutputPricePerToken: 12e-6,
		LongContextInputThreshold: 200000, LongContextThresholdInclusive: true,
		LongContextInputMultiplier: 1.5, LongContextOutputMultiplier: 1.5,
	}
	s.fallbackPrices["grok-4.7-fast"] = card
	s.fallbackPrices["grok-4.7-build-fast"] = card
}

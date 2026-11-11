package service

import "github.com/Wei-Shaw/sub2api/internal/shared/claude"

var claude52FallbackPricing = &LiteLLMModelPricing{
	InputCostPerToken:                   8e-6,
	OutputCostPerToken:                  40e-6,
	CacheCreationInputTokenCost:         10e-6,
	CacheCreationInputTokenCostAbove1hr: 16e-6,
	CacheReadInputTokenCost:             0.2e-6,
	LiteLLMProvider:                     "anthropic",
	Mode:                                "chat",
	SupportsPromptCaching:               true,
}

func claude52PricingID(model string) string {
	switch {
	case claude.IsFable52(model):
		return "claude-fable-5-2"
	case claude.IsMythos52(model):
		return "claude-mythos-5-2"
	default:
		return ""
	}
}

func (s *BillingService) initClaude52FallbackPricing() {
	for _, model := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		s.fallbackPrices[model] = &ModelPricing{
			InputPricePerToken:         claude52FallbackPricing.InputCostPerToken,
			OutputPricePerToken:        claude52FallbackPricing.OutputCostPerToken,
			CacheCreationPricePerToken: claude52FallbackPricing.CacheCreationInputTokenCost,
			CacheCreation5mPrice:       claude52FallbackPricing.CacheCreationInputTokenCost,
			CacheCreation1hPrice:       claude52FallbackPricing.CacheCreationInputTokenCostAbove1hr,
			CacheReadPricePerToken:     claude52FallbackPricing.CacheReadInputTokenCost,
			SupportsCacheBreakdown:     true,
		}
	}
}

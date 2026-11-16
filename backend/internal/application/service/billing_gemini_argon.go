package service

// Argon promotional rates run through 2026-11-30; Cyber has a separate rate.
func (s *BillingService) initGeminiArgonFallbackPricing() {
	s.fallbackPrices["gemini-4-argon"] = &ModelPricing{InputPricePerToken: 2e-6, OutputPricePerToken: 10e-6, CacheReadPricePerToken: 0.1e-6}
	s.fallbackPrices["gemini-4-argon-cyber-permissive"] = &ModelPricing{InputPricePerToken: 4e-6, OutputPricePerToken: 20e-6, CacheReadPricePerToken: 0.2e-6}
	s.fallbackPrices["gemini-4-hydrogen"] = &ModelPricing{
		InputPricePerToken: 1.5e-6, OutputPricePerToken: 7.5e-6, CacheReadPricePerToken: 0.075e-6,
		InputPricePerTokenPriority: 2.7e-6, OutputPricePerTokenPriority: 13.5e-6, CacheReadPricePerTokenPriority: 0.135e-6,
	}
}

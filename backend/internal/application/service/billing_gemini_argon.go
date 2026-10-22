package service

// Argon promotional rates run through 2026-11-30; Cyber has a separate rate.
func (s *BillingService) initGeminiArgonFallbackPricing() {
	s.fallbackPrices["gemini-4-argon"] = &ModelPricing{InputPricePerToken: 2e-6, OutputPricePerToken: 10e-6, CacheReadPricePerToken: 0.1e-6}
	s.fallbackPrices["gemini-4-argon-cyber-permissive"] = &ModelPricing{InputPricePerToken: 4e-6, OutputPricePerToken: 20e-6, CacheReadPricePerToken: 0.2e-6}
}

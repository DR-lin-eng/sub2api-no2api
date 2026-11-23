package service

// Argon and Carbon use non-promotional standard rates; Cyber has a separate rate.
func (s *BillingService) initGeminiArgonFallbackPricing() {
	// Standard rate card per million tokens, with 1.8x priority prices.
	s.fallbackPrices["gemini-4-carbon"] = &ModelPricing{
		InputPricePerToken: 4e-6, OutputPricePerToken: 20e-6, CacheReadPricePerToken: 0.2e-6,
		InputPricePerTokenPriority: 7.2e-6, OutputPricePerTokenPriority: 36e-6, CacheReadPricePerTokenPriority: 0.36e-6,
	}
	s.fallbackPrices["gemini-4-argon"] = &ModelPricing{
		InputPricePerToken: 4e-6, OutputPricePerToken: 20e-6, CacheReadPricePerToken: 0.2e-6,
		InputPricePerTokenPriority: 7.2e-6, OutputPricePerTokenPriority: 36e-6, CacheReadPricePerTokenPriority: 0.36e-6,
	}
	s.fallbackPrices["gemini-4-argon-cyber-permissive"] = &ModelPricing{InputPricePerToken: 4e-6, OutputPricePerToken: 20e-6, CacheReadPricePerToken: 0.2e-6}
	s.fallbackPrices["gemini-4-hydrogen"] = &ModelPricing{
		InputPricePerToken: 1.5e-6, OutputPricePerToken: 7.5e-6, CacheReadPricePerToken: 0.075e-6,
		InputPricePerTokenPriority: 2.7e-6, OutputPricePerTokenPriority: 13.5e-6, CacheReadPricePerTokenPriority: 0.135e-6,
	}
	s.fallbackPrices["gemini-4-neon"] = &ModelPricing{
		InputPricePerToken: 3e-6, OutputPricePerToken: 15e-6, CacheReadPricePerToken: 0.15e-6,
		InputPricePerTokenPriority: 5.4e-6, OutputPricePerTokenPriority: 27e-6, CacheReadPricePerTokenPriority: 0.27e-6,
	}
}

package service

// Reviewed built-in cards shared by token and image billing fallbacks.
var (
	openAIGPT62AstraFallbackPricing = &LiteLLMModelPricing{
		InputCostPerToken:                   8e-06,
		InputCostPerTokenPriority:           16e-06,
		OutputCostPerToken:                  40e-06,
		OutputCostPerTokenPriority:          80e-06,
		CacheCreationInputTokenCost:         10e-06,
		CacheCreationInputTokenCostPriority: 20e-06,
		CacheReadInputTokenCost:             0.4e-06,
		CacheReadInputTokenCostPriority:     0.8e-06,
		LongContextInputTokenThreshold:      openAIGPT54LongContextInputThreshold,
		LongContextInputCostMultiplier:      openAIGPT54LongContextInputMultiplier,
		LongContextOutputCostMultiplier:     openAIGPT54LongContextOutputMultiplier,
		SupportsServiceTier:                 true,
		LiteLLMProvider:                     "openai",
		Mode:                                "chat",
		SupportsPromptCaching:               true,
	}
	// GPT-6.2 Sol and GPT-6.1 Sol have the same Standard and Fast cards.
	openAIGPT62SolFallbackPricing   = openAIGPT61SolFallbackPricing
	openAIGPT61TerraFallbackPricing = &LiteLLMModelPricing{
		InputCostPerToken:                   1e-06,
		InputCostPerTokenPriority:           2e-06,
		OutputCostPerToken:                  5e-06,
		OutputCostPerTokenPriority:          10e-06,
		CacheCreationInputTokenCost:         1.25e-06,
		CacheCreationInputTokenCostPriority: 2.5e-06,
		CacheReadInputTokenCost:             0.05e-06,
		CacheReadInputTokenCostPriority:     0.1e-06,
		LongContextInputTokenThreshold:      openAIGPT54LongContextInputThreshold,
		LongContextInputCostMultiplier:      openAIGPT54LongContextInputMultiplier,
		LongContextOutputCostMultiplier:     openAIGPT54LongContextOutputMultiplier,
		SupportsServiceTier:                 true,
		LiteLLMProvider:                     "openai",
		Mode:                                "chat",
		SupportsPromptCaching:               true,
	}
	openAIGPTImage25FallbackPricing = &LiteLLMModelPricing{
		InputCostPerToken:            5e-06,
		CacheReadInputTokenCost:      1.25e-06,
		InputCostPerImageToken:       8e-06,
		CacheReadInputImageTokenCost: 2e-06,
		OutputCostPerImageToken:      3e-05,
		LiteLLMProvider:              "openai",
		Mode:                         "image_generation",
		SupportsPromptCaching:        true,
	}
)

package service

// The official Ultrafast card is 6x Standard for Astra and GPT-6.1 Sol.
// Attach the policy to a copy so dynamic/channel price snapshots stay immutable.
// Other models keep their existing behavior until an official card is available.
func applyOpenAIUltrafastPricingPolicy(model string, pricing *ModelPricing) *ModelPricing {
	if pricing == nil || (!isOpenAIGPT6AstraModel(model) && !isOpenAIGPT61SolModel(model)) {
		return pricing
	}
	cloned := *pricing
	cloned.UltrafastCostMultiplier = 6
	return &cloned
}

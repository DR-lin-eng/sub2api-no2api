package service

import "time"

// March 1, 2027 00:00 Pacific is 08:00 UTC (before the daylight-saving switch).
var gpt62UltrafastPromotionEnd = time.Date(2027, time.March, 1, 8, 0, 0, 0, time.UTC)

// Attach the policy to a copy so dynamic/channel price snapshots stay immutable.
// Other models keep their existing behavior until an official card is available.
func applyOpenAIUltrafastPricingPolicy(model string, pricing *ModelPricing, pricingAt time.Time) *ModelPricing {
	if pricing == nil || (!isOpenAIGPT6AstraModel(model) && !isOpenAIGPT61SolModel(model) && !isOpenAIGPT62Model(model)) {
		return pricing
	}
	cloned := *pricing
	cloned.UltrafastCostMultiplier = 6
	if isOpenAIGPT62Model(model) && pricingAt.Before(gpt62UltrafastPromotionEnd) {
		cloned.UltrafastCostMultiplier = 3
	}
	return &cloned
}

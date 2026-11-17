package service

import "strings"

// Match only published SiliconFlow model IDs and the documented 0-6B alias.
// The caller has already lowercased the ID. Unknown sizes, versions and VL
// variants require an explicit card instead of borrowing another model's price.
func qwen3InputPricingID(model string) string {
	model = strings.TrimPrefix(model, "siliconflow/")
	model = strings.TrimPrefix(model, "qwen/")
	switch model {
	case "qwen3-embedding-8b", "qwen3-embedding-4b", "qwen3-embedding-0.6b",
		"qwen3-reranker-8b", "qwen3-reranker-0.6b":
		return model
	case "qwen3-embedding-0-6b":
		return "qwen3-embedding-0.6b"
	case "qwen3-reranker-0-6b":
		return "qwen3-reranker-0.6b"
	default:
		return ""
	}
}

func (s *BillingService) initQwen3InputFallbackPricing() {
	// Official international USD / MTok cards, checked 2026-11-17:
	// https://www.siliconflow.com/models/qwen3-embedding-8b (and 4b / 0-6b)
	// https://www.siliconflow.com/models/qwen3-reranker-8b (and 0-6b).
	// Reranker-4B has no verified USD card; do not infer an exchange rate.
	for _, card := range []struct {
		model string
		input float64
	}{
		{"qwen3-embedding-8b", 0.04e-6},
		{"qwen3-embedding-4b", 0.02e-6},
		{"qwen3-embedding-0.6b", 0.01e-6},
		{"qwen3-reranker-8b", 0.04e-6},
		{"qwen3-reranker-0.6b", 0.01e-6},
	} {
		s.fallbackPrices[card.model] = &ModelPricing{InputPricePerToken: card.input}
	}
}

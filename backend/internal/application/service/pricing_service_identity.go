package service

import "strings"

type identifiedModelPricingIndex struct {
	exact  map[string]*LiteLLMModelPricing
	byBase map[string]*LiteLLMModelPricing
}

// GetIdentifiedModelPricing resolves only deterministic catalog identities. It
// accepts exact names, known spelling variants, and an unambiguous date-suffix
// match. It deliberately excludes family/OpenAI fallback guesses because this
// method is used to admit an upstream-provided model name into billing.
func (s *PricingService) GetIdentifiedModelPricing(modelName string) *LiteLLMModelPricing {
	if s == nil {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	modelLower := strings.ToLower(strings.TrimSpace(modelName))
	if modelLower == "" {
		return nil
	}
	lookupCandidates := s.buildModelLookupCandidates(modelLower)
	index := s.identifiedPricingIndex
	if index == nil {
		// Focused tests in this package sometimes construct PricingService directly.
		// Production snapshots build the index before publication.
		index = buildIdentifiedModelPricingIndex(s.pricingData)
	}
	return index.lookup(lookupCandidates)
}

func buildIdentifiedModelPricingIndex(pricingData map[string]*LiteLLMModelPricing) *identifiedModelPricingIndex {
	index := &identifiedModelPricingIndex{
		exact:  make(map[string]*LiteLLMModelPricing, len(pricingData)),
		byBase: make(map[string]*LiteLLMModelPricing, len(pricingData)),
	}
	for model, pricing := range pricingData {
		if pricing == nil {
			continue
		}
		model = strings.ToLower(strings.TrimSpace(model))
		if model == "" {
			continue
		}
		addIdentifiedPricing(index.exact, model, pricing)
		base, _ := identifiedPricingBaseName(model)
		addIdentifiedPricing(index.byBase, base, pricing)
	}
	return index
}

func addIdentifiedPricing(index map[string]*LiteLLMModelPricing, key string, pricing *LiteLLMModelPricing) {
	existing, exists := index[key]
	if !exists {
		index[key] = pricing
		return
	}
	if existing != nil && *existing != *pricing {
		// nil is an ambiguity sentinel. It makes lookup deterministic even when
		// multiple dated cards share a base name but not a price card.
		index[key] = nil
	}
}

func (i *identifiedModelPricingIndex) lookup(candidates []string) *LiteLLMModelPricing {
	if i == nil || len(candidates) == 0 {
		return nil
	}
	for _, candidate := range candidates {
		if pricing, exists := i.exact[candidate]; exists {
			return pricing
		}
	}
	for _, candidate := range candidates {
		normalized := strings.ReplaceAll(candidate, "-4-5-", "-4.5-")
		if pricing, exists := i.exact[normalized]; exists {
			return pricing
		}
	}
	base, versioned := identifiedPricingBaseName(candidates[0])
	if !versioned {
		return nil
	}
	return i.byBase[base]
}

// identifiedPricingBaseName accepts only suffix-shaped version identifiers.
// The broader legacy pricing lookup may remove date-like segments anywhere,
// but an upstream-controlled billing declaration must not create an identity by
// inserting eight digits into the middle of an unrelated model name.
func identifiedPricingBaseName(model string) (string, bool) {
	model = strings.TrimSpace(model)
	versioned := false
	if idx := strings.LastIndexByte(model, '-'); idx > 0 && isPricingVersionSuffix(model[idx+1:]) {
		model = model[:idx]
		versioned = true
	}
	if idx := strings.LastIndexByte(model, '-'); idx > 0 {
		suffix := model[idx+1:]
		if len(suffix) == 8 && isNumeric(suffix) {
			model = model[:idx]
			versioned = true
		}
	}
	return model, versioned
}

func isPricingVersionSuffix(value string) bool {
	if len(value) < 4 || value[0] != 'v' {
		return false
	}
	major, minor, ok := strings.Cut(value[1:], ":")
	return ok && major != "" && minor != "" && isNumeric(major) && isNumeric(minor)
}

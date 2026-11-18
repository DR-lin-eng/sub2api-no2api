package apicompat

import "encoding/json"

// AnthropicPromptTokensDetails holds OpenAI-compatible prompt token details
// occasionally included by Anthropic-compatible providers.
type AnthropicPromptTokensDetails struct {
	CachedTokens        int  `json:"cached_tokens,omitempty"`
	CachedTokensPresent bool `json:"-"`
}

func (details *AnthropicPromptTokensDetails) UnmarshalJSON(data []byte) error {
	var decoded struct {
		CachedTokens *int `json:"cached_tokens"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*details = AnthropicPromptTokensDetails{}
	if decoded.CachedTokens != nil {
		details.CachedTokens = *decoded.CachedTokens
		details.CachedTokensPresent = true
	}
	return nil
}

func (details AnthropicPromptTokensDetails) MarshalJSON() ([]byte, error) {
	var cachedTokens *int
	if details.CachedTokensPresent || details.CachedTokens != 0 {
		cachedTokens = &details.CachedTokens
	}
	return json.Marshal(struct {
		CachedTokens *int `json:"cached_tokens,omitempty"`
	}{CachedTokens: cachedTokens})
}

// AnthropicCacheCreation holds Anthropic's cache creation TTL breakdown.
// Pointers preserve the distinction between an omitted field and an explicit
// zero in streaming usage events.
type AnthropicCacheCreation struct {
	Ephemeral5mInputTokens *int `json:"ephemeral_5m_input_tokens,omitempty"`
	Ephemeral1hInputTokens *int `json:"ephemeral_1h_input_tokens,omitempty"`
}

// AnthropicUsage holds token counts in Anthropic format.
type AnthropicUsage struct {
	InputTokens              int                     `json:"input_tokens"`
	OutputTokens             int                     `json:"output_tokens"`
	CacheCreationInputTokens int                     `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int                     `json:"cache_read_input_tokens"`
	CacheCreation            *AnthropicCacheCreation `json:"cache_creation,omitempty"`
	// Anthropic-compatible providers can also expose their native OpenAI-style
	// total/cache fields. Preserve them so callers can normalize provider totals
	// into Anthropic's mutually-exclusive billing buckets.
	PromptTokens          int                           `json:"prompt_tokens,omitempty"`
	CachedTokens          int                           `json:"cached_tokens,omitempty"`
	PromptTokensDetails   *AnthropicPromptTokensDetails `json:"prompt_tokens_details,omitempty"`
	PromptCacheHitTokens  *int                          `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens *int                          `json:"prompt_cache_miss_tokens,omitempty"`

	// Presence distinguishes explicit zero from missing/null; TTL details remain raw.
	CacheCreationInputTokensPresent bool `json:"-"`
	CacheReadInputTokensPresent     bool `json:"-"`
	CachedTokensPresent             bool `json:"-"`
}

func (usage *AnthropicUsage) UnmarshalJSON(data []byte) error {
	type usageAlias AnthropicUsage
	var decoded struct {
		usageAlias
		CacheCreationInputTokens *int `json:"cache_creation_input_tokens"`
		CacheReadInputTokens     *int `json:"cache_read_input_tokens"`
		CachedTokens             *int `json:"cached_tokens"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*usage = AnthropicUsage(decoded.usageAlias)
	if decoded.CacheCreationInputTokens != nil {
		usage.CacheCreationInputTokens = *decoded.CacheCreationInputTokens
		usage.CacheCreationInputTokensPresent = true
	}
	if decoded.CacheReadInputTokens != nil {
		usage.CacheReadInputTokens = *decoded.CacheReadInputTokens
		usage.CacheReadInputTokensPresent = true
	}
	if decoded.CachedTokens != nil {
		usage.CachedTokens = *decoded.CachedTokens
		usage.CachedTokensPresent = true
	}
	return nil
}

func (usage AnthropicUsage) HasCacheCreationInputTokens() bool {
	return usage.CacheCreationInputTokensPresent || usage.CacheCreationInputTokens != 0
}

// MarshalJSON preserves aggregate presence and raw TTL details without deriving totals.
func (usage AnthropicUsage) MarshalJSON() ([]byte, error) {
	type usageAlias AnthropicUsage
	var aggregate *int
	if usage.HasCacheCreationInputTokens() {
		aggregate = &usage.CacheCreationInputTokens
	}
	var cacheReadTokens, cachedTokens *int
	if usage.CacheReadInputTokensPresent || usage.CacheReadInputTokens != 0 {
		cacheReadTokens = &usage.CacheReadInputTokens
	}
	if usage.CachedTokensPresent || usage.CachedTokens != 0 {
		cachedTokens = &usage.CachedTokens
	}
	return json.Marshal(struct {
		usageAlias
		CacheCreationInputTokens *int `json:"cache_creation_input_tokens,omitempty"`
		CacheReadInputTokens     *int `json:"cache_read_input_tokens,omitempty"`
		CachedTokens             *int `json:"cached_tokens,omitempty"`
	}{usageAlias: usageAlias(usage), CacheCreationInputTokens: aggregate, CacheReadInputTokens: cacheReadTokens, CachedTokens: cachedTokens})
}

func anthropicCacheCreationInputTokens(usage AnthropicUsage) int {
	if usage.CacheCreationInputTokens > 0 {
		return max(usage.CacheCreationInputTokens, 0)
	}
	if usage.CacheCreation != nil && (usage.CacheCreation.Ephemeral5mInputTokens != nil || usage.CacheCreation.Ephemeral1hInputTokens != nil) {
		cacheCreationTokens := 0
		if usage.CacheCreation.Ephemeral5mInputTokens != nil {
			cacheCreationTokens += max(*usage.CacheCreation.Ephemeral5mInputTokens, 0)
		}
		if usage.CacheCreation.Ephemeral1hInputTokens != nil {
			cacheCreationTokens += max(*usage.CacheCreation.Ephemeral1hInputTokens, 0)
		}
		return cacheCreationTokens
	}
	return max(usage.CacheCreationInputTokens, 0)
}

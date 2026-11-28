package service

import (
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/apicompat"
	"github.com/tidwall/gjson"
)

func mergeAnthropicUsage(dst *ClaudeUsage, src apicompat.AnthropicUsage) {
	if dst == nil {
		return
	}

	cacheReadTokens := max(src.CacheReadInputTokens, 0)
	hasCacheReadTokens := src.CacheReadInputTokensPresent || src.CacheReadInputTokens != 0
	if cacheReadTokens == 0 && (src.CachedTokensPresent || src.CachedTokens != 0) {
		cacheReadTokens = max(src.CachedTokens, 0)
		hasCacheReadTokens = true
	}
	if cacheReadTokens == 0 && src.PromptTokensDetails != nil && (src.PromptTokensDetails.CachedTokensPresent || src.PromptTokensDetails.CachedTokens != 0) {
		cacheReadTokens = max(src.PromptTokensDetails.CachedTokens, 0)
		hasCacheReadTokens = true
	}
	if cacheReadTokens == 0 && src.PromptCacheHitTokens != nil {
		cacheReadTokens = max(*src.PromptCacheHitTokens, 0)
		hasCacheReadTokens = true
	}
	mergeAnthropicCacheCreationUsage(dst, src)

	// Some Anthropic-compatible providers retain OpenAI-style prompt/cache
	// fields. Prefer those authoritative totals or hit/miss buckets over the
	// overloaded input_tokens field. This covers Kimi's changing stream
	// semantics as well as GLM/DeepSeek cache aliases.
	if src.PromptTokens <= 0 && src.PromptCacheHitTokens == nil && src.PromptCacheMissTokens == nil && !dst.promptTotalPresent {
		// Without an authoritative prompt total or miss bucket, input_tokens is
		// provider-specific: it may already be the uncached bucket, or it may be
		// a total from an earlier event. Do not infer a subtraction merely because
		// a later event contains cache buckets; that would corrupt providers whose
		// stream uses independent input and cache fields.
		if src.InputTokens > 0 {
			dst.InputTokens = src.InputTokens
		}
	}
	if hasCacheReadTokens && (cacheReadTokens > 0 || dst.CacheReadInputTokens <= 0) {
		dst.CacheReadInputTokens = cacheReadTokens
	}
	normalizeAnthropicPromptTotal(dst, src.PromptTokens, src.PromptCacheMissTokens)
	if src.OutputTokens > 0 {
		dst.OutputTokens = src.OutputTokens
	}
	if details := src.OutputTokensDetails; details != nil && details.ThinkingTokens != nil {
		dst.ReasoningTokens = details.ThinkingTokens
	}
}

func normalizeAnthropicPromptTotal(usage *ClaudeUsage, promptTokens int, promptCacheMissTokens *int) {
	if promptCacheMissTokens != nil {
		usage.promptTotalPresent = false
		usage.InputTokens = max(*promptCacheMissTokens, 0)
		return
	}
	if promptTokens > 0 {
		usage.promptTotalTokens = promptTokens
		usage.promptTotalPresent = true
	}
	if usage.promptTotalPresent {
		usage.InputTokens = max(usage.promptTotalTokens-usage.CacheReadInputTokens-usage.CacheCreationInputTokens, 0)
	}
}

// Values and presence travel together without allocating pointer fields on raw SSE.
type anthropicCacheCreationSnapshot struct {
	aggregate, fiveMinutes, oneHour          int
	hasAggregate, hasFiveMinutes, hasOneHour bool
}

func mergeAnthropicCacheCreationSnapshot(dst *ClaudeUsage, snapshot anthropicCacheCreationSnapshot) {
	if snapshot.hasAggregate && (snapshot.aggregate > 0 || dst.CacheCreationInputTokens <= 0) {
		dst.CacheCreationInputTokens = max(snapshot.aggregate, 0)
		dst.cacheCreationInputTokensPresent = true
		dst.cacheCreationInputTokensFromDetails = false
	}
	// Preserve this fork's established delta-zero placeholder policy (#6132).
	if snapshot.hasFiveMinutes && (snapshot.fiveMinutes > 0 || dst.CacheCreation5mTokens <= 0) {
		dst.CacheCreation5mTokens = max(snapshot.fiveMinutes, 0)
	}
	if snapshot.hasOneHour && (snapshot.oneHour > 0 || dst.CacheCreation1hTokens <= 0) {
		dst.CacheCreation1hTokens = max(snapshot.oneHour, 0)
	}

	if (!dst.cacheCreationInputTokensPresent || dst.cacheCreationInputTokensFromDetails || dst.CacheCreationInputTokens <= 0) &&
		(snapshot.hasFiveMinutes || snapshot.hasOneHour) {
		dst.CacheCreationInputTokens = max(dst.CacheCreation5mTokens, 0) + max(dst.CacheCreation1hTokens, 0)
		dst.cacheCreationInputTokensFromDetails = true
	}
}

func mergeAnthropicCacheCreationUsage(dst *ClaudeUsage, src apicompat.AnthropicUsage) {
	snapshot := anthropicCacheCreationSnapshot{aggregate: src.CacheCreationInputTokens, hasAggregate: src.HasCacheCreationInputTokens()}
	if details := src.CacheCreation; details != nil {
		if details.Ephemeral5mInputTokens != nil {
			snapshot.fiveMinutes, snapshot.hasFiveMinutes = *details.Ephemeral5mInputTokens, true
		}
		if details.Ephemeral1hInputTokens != nil {
			snapshot.oneHour, snapshot.hasOneHour = *details.Ephemeral1hInputTokens, true
		}
	}
	mergeAnthropicCacheCreationSnapshot(dst, snapshot)
}

func rawAnthropicCacheCreationDetails(node gjson.Result, snapshot *anthropicCacheCreationSnapshot) {
	node.ForEach(func(key, value gjson.Result) bool {
		if value.Type == gjson.Null {
			return true
		}
		switch key.Str {
		case "ephemeral_5m_input_tokens":
			snapshot.fiveMinutes, snapshot.hasFiveMinutes = int(value.Int()), true
		case "ephemeral_1h_input_tokens":
			snapshot.oneHour, snapshot.hasOneHour = int(value.Int()), true
		}
		return true
	})
}

func mergeRawAnthropicCacheCreationUsage(node gjson.Result, dst *ClaudeUsage) {
	aggregate := node.Get("cache_creation_input_tokens")
	snapshot := anthropicCacheCreationSnapshot{aggregate: int(aggregate.Int()), hasAggregate: aggregate.Exists() && aggregate.Type != gjson.Null}
	rawAnthropicCacheCreationDetails(node.Get("cache_creation"), &snapshot)
	mergeAnthropicCacheCreationSnapshot(dst, snapshot)
}

// Parse the usage object once. Content-only deltas never enter this function.
func mergeRawAnthropicUsage(node gjson.Result, usage *ClaudeUsage, start bool) {
	var snapshot anthropicCacheCreationSnapshot
	var reads [4]int
	var prompt, miss int
	hasMiss := false
	node.ForEach(func(key, value gjson.Result) bool {
		if value.Type == gjson.Null {
			return true
		}
		switch key.Str {
		case "input_tokens":
			if start || value.Int() > 0 {
				usage.InputTokens = int(value.Int())
			}
		case "output_tokens":
			if value.Int() > 0 {
				usage.OutputTokens = int(value.Int())
			}
		case "cache_creation_input_tokens":
			snapshot.aggregate, snapshot.hasAggregate = int(value.Int()), true
		case "cache_creation":
			rawAnthropicCacheCreationDetails(value, &snapshot)
		case "cache_read_input_tokens":
			reads[0] = int(value.Int())
		case "cached_tokens":
			reads[1] = int(value.Int())
		case "prompt_tokens_details":
			reads[2] = int(value.Get("cached_tokens").Int())
		case "prompt_cache_hit_tokens":
			reads[3] = int(value.Int())
		case "prompt_tokens":
			prompt = int(value.Int())
		case "prompt_cache_miss_tokens":
			miss, hasMiss = int(value.Int()), true
		}
		return true
	})
	mergeAnthropicCacheCreationSnapshot(usage, snapshot)
	for _, tokens := range reads {
		if tokens > 0 {
			usage.CacheReadInputTokens = tokens
			break
		}
	}
	var missValue *int
	if hasMiss {
		missValue = &miss
	}
	normalizeAnthropicPromptTotal(usage, prompt, missValue)
}

func normalizeAnthropicEventUsageForResponses(event *apicompat.AnthropicStreamEvent, usage ClaudeUsage) {
	normalize := func(dst *apicompat.AnthropicUsage) {
		if dst == nil {
			return
		}
		dst.InputTokens = usage.InputTokens
		dst.OutputTokens = usage.OutputTokens
		dst.CacheReadInputTokens = usage.CacheReadInputTokens
		dst.CacheCreationInputTokens = usage.CacheCreationInputTokens
		dst.CacheCreationInputTokensPresent = true
		dst.CacheCreation = nil
		if dst.OutputTokensDetails == nil && usage.ReasoningTokens != nil {
			dst.OutputTokensDetails = &apicompat.AnthropicOutputTokensDetails{ThinkingTokens: usage.ReasoningTokens}
		}
	}
	normalize(event.Usage)
	if event.Message != nil {
		normalize(&event.Message.Usage)
	}
}

func parseSSEUsagePassthrough(data string, usage *ClaudeUsage) {
	if usage == nil || data == "" || data == "[DONE]" {
		return
	}
	parsed := gjson.Parse(data)
	start := parsed.Get("type").String() == "message_start"
	node := parsed.Get("usage")
	if start {
		node = parsed.Get("message.usage")
	}
	if !node.IsObject() {
		return
	}
	mergeRawAnthropicUsage(node, usage, start)
}

func normalizeAnthropicCompatiblePromptUsage(usageNode gjson.Result, usage *ClaudeUsage) bool {
	if usage == nil || !usageNode.Exists() {
		return false
	}
	promptTokens := usageNode.Get("prompt_tokens")
	promptCacheHitTokens := usageNode.Get("prompt_cache_hit_tokens")
	promptCacheMissTokens := usageNode.Get("prompt_cache_miss_tokens")
	if (!promptTokens.Exists() || promptTokens.Int() <= 0) &&
		!promptCacheHitTokens.Exists() && !promptCacheMissTokens.Exists() && !usage.promptTotalPresent {
		return false
	}

	cacheReadTokens := usage.CacheReadInputTokens
	for _, path := range []string{"cache_read_input_tokens", "cached_tokens", "prompt_tokens_details.cached_tokens", "prompt_cache_hit_tokens"} {
		if value := usageNode.Get(path); value.Exists() && value.Type != gjson.Null {
			cacheReadTokens = max(int(value.Int()), 0)
			if cacheReadTokens > 0 {
				break
			}
		}
	}

	if cacheReadTokens > 0 || usage.CacheReadInputTokens <= 0 {
		usage.CacheReadInputTokens = cacheReadTokens
	}
	var cacheMissTokens *int
	if promptCacheMissTokens.Exists() && promptCacheMissTokens.Type != gjson.Null {
		value := int(promptCacheMissTokens.Int())
		cacheMissTokens = &value
	}
	normalizeAnthropicPromptTotal(usage, int(promptTokens.Int()), cacheMissTokens)
	return true
}

// Keep converter snapshots in sync even when a later usage event explicitly
// reduces a cumulative bucket to zero.
func syncAnthropicResponsesUsage(state *apicompat.AnthropicEventToResponsesState, usage ClaudeUsage) {
	state.InputTokens = usage.InputTokens
	state.OutputTokens = usage.OutputTokens
	state.CacheReadInputTokens = usage.CacheReadInputTokens
	state.CacheCreationInputTokens = usage.CacheCreationInputTokens
}

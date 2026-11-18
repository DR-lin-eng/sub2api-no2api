package service

func (usage ClaudeUsage) billingUsageTokens() UsageTokens {
	creationTokens := usage.CacheCreationInputTokens
	if creationTokens <= 0 {
		creationTokens = max(usage.CacheCreation5mTokens, 0) + max(usage.CacheCreation1hTokens, 0)
	}
	tokens := UsageTokens{
		InputTokens:           usage.InputTokens,
		OutputTokens:          usage.OutputTokens,
		CacheCreationTokens:   creationTokens,
		CacheReadTokens:       usage.CacheReadInputTokens,
		CacheCreation5mTokens: usage.CacheCreation5mTokens,
		CacheCreation1hTokens: usage.CacheCreation1hTokens,
		ImageOutputTokens:     usage.ImageOutputTokens,
	}
	return tokens
}

func (usage OpenAIUsage) billingUsageTokens() UsageTokens {
	creationTokens := usage.CacheCreationInputTokens
	if creationTokens <= 0 {
		creationTokens = max(usage.CacheCreation5mTokens, 0) + max(usage.CacheCreation1hTokens, 0)
	}
	tokens := UsageTokens{
		InputTokens:           max(usage.InputTokens-usage.CacheReadInputTokens-creationTokens, 0),
		ImageInputTokens:      max(usage.ImageInputTokens-usage.ImageCacheReadTokens, 0),
		ImageCacheReadTokens:  usage.ImageCacheReadTokens,
		OutputTokens:          usage.OutputTokens,
		CacheCreationTokens:   creationTokens,
		CacheCreation5mTokens: usage.CacheCreation5mTokens,
		CacheCreation1hTokens: usage.CacheCreation1hTokens,
		CacheReadTokens:       usage.CacheReadInputTokens,
		ImageOutputTokens:     usage.ImageOutputTokens,
	}
	return tokens
}

func allocateCacheCreationBillingTokens(tokens UsageTokens) (int, int) {
	cacheCreation5mTokens, cacheCreation1hTokens := normalizeCacheCreationBreakdown(tokens)
	if tokens.CacheCreationTokens > 0 {
		cacheCreation5mTokens += tokens.CacheCreationTokens - cacheCreation5mTokens - cacheCreation1hTokens
	}
	return cacheCreation5mTokens, cacheCreation1hTokens
}

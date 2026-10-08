package openai_ws_v2

import (
	"github.com/tidwall/gjson"
	"strings"
)

func parseUsageAndAccumulate(
	state *relayState,
	message []byte,
	eventType string,
	onParseFailure func(eventType string, usageRaw string),
) Usage {
	if state == nil || len(message) == 0 || !shouldParseUsage(eventType) {
		return Usage{}
	}
	usageResult := gjson.GetBytes(message, "response.usage")
	if !usageResult.Exists() {
		return Usage{}
	}
	usageRaw := strings.TrimSpace(usageResult.Raw)
	if usageRaw == "" || !strings.HasPrefix(usageRaw, "{") {
		recordUsageParseFailure()
		if onParseFailure != nil {
			onParseFailure(eventType, usageRaw)
		}
		return Usage{}
	}

	inputResult := gjson.GetBytes(message, "response.usage.input_tokens")
	if !inputResult.Exists() {
		inputResult = gjson.GetBytes(message, "response.usage.prompt_tokens")
	}
	outputResult := gjson.GetBytes(message, "response.usage.output_tokens")
	if !outputResult.Exists() {
		outputResult = gjson.GetBytes(message, "response.usage.completion_tokens")
	}
	cachedResult := gjson.GetBytes(message, "response.usage.input_tokens_details.cached_tokens")
	if !cachedResult.Exists() {
		cachedResult = gjson.GetBytes(message, "response.usage.prompt_tokens_details.cached_tokens")
	}
	imageTokens := usageResult.Get("output_tokens_details.image_tokens").Int()
	if imageTokens == 0 {
		imageTokens = usageResult.Get("completion_tokens_details.image_tokens").Int()
	}
	imageInputTokens := usageResult.Get("input_tokens_details.image_tokens").Int()
	if imageInputTokens <= 0 {
		imageInputTokens = usageResult.Get("prompt_tokens_details.image_tokens").Int()
	}
	imageGen := gjson.GetBytes(message, "response.tool_usage.image_gen")
	if !imageGen.Exists() {
		imageGen = gjson.GetBytes(message, "tool_usage.image_gen")
	}
	if imageTokens <= 0 {
		imageTokens = max(imageGen.Get("output_tokens_details.image_tokens").Int(), 0)
	}
	if imageInputTokens <= 0 {
		imageInputTokens = max(imageGen.Get("input_tokens_details.image_tokens").Int(), 0)
	}

	inputTokens, inputOK := parseUsageIntField(inputResult, true)
	outputTokens, outputOK := parseUsageIntField(outputResult, true)
	cachedTokens, cachedOK := parseUsageIntField(cachedResult, false)
	if !inputOK || !outputOK || !cachedOK {
		recordUsageParseFailure()
		if onParseFailure != nil {
			onParseFailure(eventType, usageRaw)
		}
		// 解析失败时不做部分字段累加，避免计费 usage 出现“半有效”状态。
		return Usage{}
	}
	parsedUsage := Usage{
		InputTokens:              inputTokens,
		ImageInputTokens:         int(imageInputTokens),
		OutputTokens:             outputTokens,
		CacheCreationInputTokens: openAICacheCreationTokensFromUsage(usageResult),
		CacheReadInputTokens:     cachedTokens,
		ImageOutputTokens:        int(imageTokens),
	}

	state.usage.InputTokens += parsedUsage.InputTokens
	state.usage.ImageInputTokens += parsedUsage.ImageInputTokens
	state.usage.OutputTokens += parsedUsage.OutputTokens
	state.usage.CacheCreationInputTokens += parsedUsage.CacheCreationInputTokens
	state.usage.CacheReadInputTokens += parsedUsage.CacheReadInputTokens
	state.usage.ImageOutputTokens += parsedUsage.ImageOutputTokens
	return parsedUsage
}

func parseUsageIntField(value gjson.Result, required bool) (int, bool) {
	if !value.Exists() {
		return 0, !required
	}
	if value.Type != gjson.Number {
		return 0, false
	}
	return int(value.Int()), true
}

func openAICacheCreationTokensFromUsage(value gjson.Result) int {
	for _, field := range []string{
		"input_tokens_details.cache_write_tokens",
		"prompt_tokens_details.cache_write_tokens",
		"input_tokens_details.cache_creation_tokens",
		"prompt_tokens_details.cache_creation_tokens",
	} {
		result := value.Get(field)
		if result.Exists() {
			return max(int(result.Int()), 0)
		}
	}
	for _, field := range []string{
		"cache_write_tokens",
		"cache_creation_input_tokens",
		"cache_write_input_tokens",
		"cache_creation_tokens",
	} {
		if tokens := int(value.Get(field).Int()); tokens > 0 {
			return tokens
		}
	}
	return 0
}

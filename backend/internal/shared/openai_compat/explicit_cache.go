package openai_compat

import (
	"strings"

	"github.com/tidwall/gjson"
)

const ExplicitCacheParam = "explicit_cache"

// ShouldRejectGPT56ResponsesExplicitCache reports whether a Responses request
// uses the legacy top-level explicit_cache field with GPT-5.6 series models.
func ShouldRejectGPT56ResponsesExplicitCache(model string, body []byte) bool {
	return IsGPT56SeriesModel(model) && gjson.GetBytes(body, ExplicitCacheParam).Exists()
}

// IsGPT56SeriesModel matches GPT-5.6 model names, including provider-prefixed
// aliases such as "openai/gpt-5.6-sol".
func IsGPT56SeriesModel(model string) bool {
	normalized := strings.TrimSpace(model)
	if slash := strings.LastIndex(normalized, "/"); slash >= 0 {
		normalized = normalized[slash+1:]
	}
	const prefix = "gpt-5.6"
	if len(normalized) < len(prefix) || !strings.EqualFold(normalized[:len(prefix)], prefix) {
		return false
	}
	return len(normalized) == len(prefix) || normalized[len(prefix)] == '-'
}

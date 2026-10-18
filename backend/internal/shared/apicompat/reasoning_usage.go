package apicompat

import "github.com/tidwall/gjson"

func reasoningTokensFromJSON(data []byte) *int {
	return ReasoningTokensFromUsage(gjson.ParseBytes(data))
}

func (u *ResponsesUsage) ReasoningTokenCount() *int {
	if u == nil {
		return nil
	}
	if u.RecordedReasoningTokens != nil {
		return u.RecordedReasoningTokens
	}
	if u.OutputTokensDetails != nil && u.OutputTokensDetails.ReasoningTokens > 0 && u.OutputTokensDetails.ReasoningTokens <= 2147483647 {
		return &u.OutputTokensDetails.ReasoningTokens
	}
	return nil
}

// ReasoningTokensFromUsage preserves missing counts and explicit zeroes.
// It reads counts only; reasoning remains part of the existing output total.
func ReasoningTokensFromUsage(usage gjson.Result) *int {
	for _, path := range []string{"output_tokens_details.reasoning_tokens", "completion_tokens_details.reasoning_tokens", "reasoning_tokens", "thoughtsTokenCount"} {
		value := usage.Get(path)
		if !value.Exists() {
			continue
		}
		if value.Type != gjson.Number || value.Float() < 0 || value.Float() > 2147483647 || value.Float() != float64(value.Int()) {
			return nil
		}
		count := int(value.Int())
		return &count
	}
	return nil
}

func SumReasoningTokens(a, b *int) *int {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	count := *a + *b
	return &count
}

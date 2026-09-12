package service

import (
	"context"
	"encoding/json"
	"math"
	"strings"
)

type accountTestUsageContextKey struct{}

func withAccountTestUsage(ctx context.Context) context.Context {
	return context.WithValue(ctx, accountTestUsageContextKey{}, true)
}

// reasoningTokensFromPayload accepts only explicit provider usage counters.
// Output text length and total output tokens are never used as substitutes.
func reasoningTokensFromPayload(payload map[string]any) *int64 {
	paths := [][]string{
		{"response", "usage", "output_tokens_details", "reasoning_tokens"},
		{"response", "usage", "completion_tokens_details", "reasoning_tokens"},
		{"response", "usage", "reasoning_tokens"},
		{"usage", "output_tokens_details", "reasoning_tokens"},
		{"usage", "completion_tokens_details", "reasoning_tokens"},
		{"usage", "reasoning_tokens"},
		{"usageMetadata", "thoughtsTokenCount"},
		{"response", "usageMetadata", "thoughtsTokenCount"},
	}
	for _, path := range paths {
		var value any = payload
		for _, key := range path {
			object, ok := value.(map[string]any)
			if !ok {
				value = nil
				break
			}
			value = object[key]
		}
		if tokens := validReasoningTokenCount(value); tokens != nil {
			return tokens
		}
	}
	return nil
}

func validReasoningTokenCount(value any) *int64 {
	const maxExactInteger = 1<<53 - 1
	var count int64
	switch value := value.(type) {
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > maxExactInteger || math.Trunc(value) != value {
			return nil
		}
		count = int64(value)
	case int:
		count = int64(value)
	case int64:
		count = value
	case json.Number:
		parsed, err := value.Int64()
		if err != nil {
			return nil
		}
		count = parsed
	default:
		return nil
	}
	if count < 0 || count > maxExactInteger {
		return nil
	}
	return &count
}

func parseTestSSEOutputWithReasoning(body string) (string, string, *int64) {
	text, message := parseTestSSEOutput(body)
	var reasoning *int64
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event TestEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err != nil {
			continue
		}
		if event.Type == "test_complete" && event.ReasoningTokens != nil {
			value := *event.ReasoningTokens
			reasoning = &value
		}
	}
	return text, message, reasoning
}

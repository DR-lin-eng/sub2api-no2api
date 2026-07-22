package service

import (
	"encoding/json"
	"strings"
	"unicode"
)

const upstreamBalanceSignalMaxDepth = 4

type upstreamBalanceSignals struct {
	markers  []string
	messages []string
}

// isUpstreamInsufficientBalanceError deliberately requires a durable billing
// signal. Generic quota exhaustion and rate limits are excluded because they
// normally recover on their own and must not permanently stop scheduling.
func isUpstreamInsufficientBalanceError(statusCode int, body []byte) bool {
	if statusCode > 0 && statusCode < 400 {
		return false
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return false
	}

	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		return hasInsufficientBalancePhrase(trimmed)
	}

	signals := upstreamBalanceSignals{}
	collectUpstreamBalanceSignals(payload, 0, &signals)
	for _, marker := range signals.markers {
		if isDirectInsufficientBalanceMarker(marker) {
			return true
		}
	}
	for _, message := range signals.messages {
		if hasInsufficientBalancePhrase(message) {
			return true
		}
	}

	billingMarker := false
	for _, marker := range signals.markers {
		if normalizeUpstreamBalanceMarker(marker) == "billing_error" {
			billingMarker = true
			break
		}
	}
	if billingMarker {
		for _, message := range signals.messages {
			if hasBillingExhaustionPhrase(message) {
				return true
			}
		}
	}
	return false
}

func collectUpstreamBalanceSignals(value any, depth int, signals *upstreamBalanceSignals) {
	if signals == nil || depth > upstreamBalanceSignalMaxDepth {
		return
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalizedKey := strings.ToLower(strings.TrimSpace(key))
			switch normalizedKey {
			case "type", "code", "status", "error_code":
				if text, ok := child.(string); ok {
					signals.markers = append(signals.markers, text)
				}
			case "message", "detail", "error_description", "reason":
				if text, ok := child.(string); ok {
					signals.messages = append(signals.messages, text)
					collectEmbeddedUpstreamBalanceJSON(text, depth+1, signals)
				}
			case "error":
				if text, ok := child.(string); ok {
					signals.messages = append(signals.messages, text)
					collectEmbeddedUpstreamBalanceJSON(text, depth+1, signals)
				}
			}
			if isUpstreamBalanceContainerKey(normalizedKey) {
				collectUpstreamBalanceSignals(child, depth+1, signals)
			}
		}
	case []any:
		for _, child := range typed {
			collectUpstreamBalanceSignals(child, depth+1, signals)
		}
	}
}

func collectEmbeddedUpstreamBalanceJSON(value string, depth int, signals *upstreamBalanceSignals) {
	trimmed := strings.TrimSpace(value)
	if depth > upstreamBalanceSignalMaxDepth || len(trimmed) < 2 || len(trimmed) > 64*1024 ||
		(trimmed[0] != '{' && trimmed[0] != '[') {
		return
	}
	var nested any
	if json.Unmarshal([]byte(trimmed), &nested) == nil {
		collectUpstreamBalanceSignals(nested, depth, signals)
	}
}

func isUpstreamBalanceContainerKey(key string) bool {
	switch key {
	case "error", "errors", "response", "detail", "cause", "data":
		return true
	default:
		return false
	}
}

func normalizeUpstreamBalanceMarker(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	builder.Grow(len(value))
	lastUnderscore := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			_, _ = builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			_ = builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(builder.String(), "_")
}

func isDirectInsufficientBalanceMarker(value string) bool {
	switch normalizeUpstreamBalanceMarker(value) {
	case "insufficient_balance",
		"balance_insufficient",
		"insufficient_funds",
		"insufficient_credit",
		"insufficient_credits",
		"credit_balance_exhausted",
		"credit_balance_depleted",
		"billing_hard_limit_reached",
		"billing_limit_reached":
		return true
	default:
		return false
	}
}

func hasInsufficientBalancePhrase(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return false
	}
	for _, phrase := range []string{
		"insufficient balance",
		"insufficient credit",
		"insufficient credits",
		"insufficient funds",
		"not enough balance",
		"not enough credit",
		"not enough credits",
		"not enough funds",
		"no remaining balance",
		"no remaining credit",
		"no remaining credits",
		"out of credit",
		"out of credits",
		"out of funds",
		"余额不足",
		"余额已用尽",
		"余额耗尽",
		"余额为零",
		"可用额度不足",
		"账户额度不足",
		"账号额度不足",
		"积分不足",
		"点数不足",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}

	balanceTerm := strings.Contains(lower, "balance") || strings.Contains(lower, "credit balance")
	exhaustedTerm := strings.Contains(lower, "exhausted") || strings.Contains(lower, "depleted") ||
		strings.Contains(lower, "empty") || strings.Contains(lower, "too low") ||
		strings.Contains(lower, "used up")
	return balanceTerm && exhaustedTerm
}

func hasBillingExhaustionPhrase(value string) bool {
	if hasInsufficientBalancePhrase(value) {
		return true
	}
	lower := strings.ToLower(strings.TrimSpace(value))
	for _, phrase := range []string{
		"payment required",
		"billing limit reached",
		"billing limit exceeded",
		"billing quota exceeded",
		"add funds",
		"top up",
		"充值后",
		"需要充值",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

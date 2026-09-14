package openai_ws_v2

import "strings"

func isTerminalEvent(eventType string) bool {
	switch eventType {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	default:
		return false
	}
}

func shouldParseUsage(eventType string) bool {
	switch eventType {
	case "response.completed", "response.done", "response.failed", "response.incomplete", "response.cancelled", "response.canceled":
		return true
	default:
		return false
	}
}

func isTokenEvent(eventType string) bool {
	eventType = strings.TrimSpace(eventType)
	return strings.HasSuffix(eventType, ".delta") ||
		eventType == "response.output_text.done" ||
		eventType == "response.function_call_arguments.done"
}

func isFirstEventTTFTEvent(eventType string) bool {
	eventType = strings.ToLower(strings.TrimSpace(eventType))
	if isRateLimitsEvent(eventType) {
		return true
	}
	return eventType == "codex.response.metadata"
}

func isRateLimitsEvent(eventType string) bool {
	eventType = strings.ToLower(strings.TrimSpace(eventType))
	return eventType == "codex.rate_limits" ||
		eventType == "rate_limits" ||
		eventType == "response.rate_limits" ||
		strings.HasPrefix(eventType, "rate_limits.") ||
		strings.HasPrefix(eventType, "response.rate_limits.")
}

func isSuccessfulLocalTTFTTerminal(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done":
		return true
	default:
		return false
	}
}

func isTTFTEvent(eventType string, legacy bool) bool {
	eventType = strings.TrimSpace(eventType)
	// Codex may emit rate-limit/metadata frames before any model output. They
	// are not semantic output, but they are valid local first-event latency
	// samples. The relay's caller still keeps replay/failover decisions on the
	// semantic-output classifier.
	if isFirstEventTTFTEvent(eventType) {
		return true
	}
	if !legacy {
		return isTokenEvent(eventType)
	}
	if eventType == "" {
		return false
	}
	switch eventType {
	case "response.created", "response.in_progress", "response.output_item.added", "response.output_item.done":
		return false
	}
	if strings.Contains(eventType, ".delta") ||
		strings.HasPrefix(eventType, "response.output_text") ||
		strings.HasPrefix(eventType, "response.output") {
		return true
	}
	return eventType == "response.completed" || eventType == "response.done"
}

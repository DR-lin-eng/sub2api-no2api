package service

import (
	"context"
	"strings"

	"github.com/tidwall/gjson"
)

type openAIVisibleOutputTTFTContextKey struct{}

func withOpenAIVisibleOutputTTFT(ctx context.Context, enabled bool) context.Context {
	return context.WithValue(ctx, openAIVisibleOutputTTFTContextKey{}, enabled)
}

func (s *OpenAIGatewayService) useOpenAIVisibleOutputTTFT(ctx context.Context) bool {
	if ctx != nil {
		if enabled, ok := ctx.Value(openAIVisibleOutputTTFTContextKey{}).(bool); ok {
			return enabled
		}
	}
	if s == nil || s.settingService == nil {
		return true
	}
	return s.settingService.IsOpenAIVisibleOutputTTFTEnabled(ctx)
}

// openAIStreamDataStartsLocalFirstEventTTFT identifies the Codex control
// events that are useful as a local first-event timestamp.  They are not
// semantic model output and must never be used as semantic progress. The
// rate-limit member may be flushed immediately because it is replay-safe; the
// timestamp still remains separate from watchdog/failover classification.
func openAIStreamDataStartsLocalFirstEventTTFT(trimmedData, eventType string) bool {
	trimmedData = strings.TrimSpace(trimmedData)
	if trimmedData == "" || trimmedData == "[DONE]" {
		return false
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		eventType = strings.TrimSpace(gjson.Get(trimmedData, "type").String())
	}
	return isOpenAILocalFirstEventType(eventType)
}

func isOpenAILocalFirstEventType(eventType string) bool {
	eventType = strings.ToLower(strings.TrimSpace(eventType))
	if isOpenAIRateLimitsEventType(eventType) {
		return true
	}
	return eventType == "codex.response.metadata"
}

// Only the rate-limit preamble is sent ahead of semantic output. Metadata may
// contain turn-state headers, so it remains attempt-local until the stream is
// known to be safe to commit.
func isOpenAIImmediateFirstEventType(eventType string) bool {
	return isOpenAIRateLimitsEventType(eventType)
}

func isOpenAIRateLimitsEventType(eventType string) bool {
	eventType = strings.ToLower(strings.TrimSpace(eventType))
	return eventType == "codex.rate_limits" ||
		eventType == "rate_limits" ||
		eventType == "response.rate_limits" ||
		strings.HasPrefix(eventType, "rate_limits.") ||
		strings.HasPrefix(eventType, "response.rate_limits.")
}

func openAIStreamDataStartsTTFT(trimmedData, eventType string, visibleOutput bool) bool {
	trimmedData = strings.TrimSpace(trimmedData)
	eventType = strings.TrimSpace(eventType)
	if openAIStreamDataStartsLocalFirstEventTTFT(trimmedData, eventType) {
		return true
	}
	if visibleOutput {
		return openAIStreamDataStartsVisibleOutput(trimmedData, eventType)
	}
	return openAIStreamDataStartsClientOutputTrimmed(trimmedData, eventType)
}

func isOpenAIWSTTFTEvent(eventType string, visibleOutput bool) bool {
	if isOpenAILocalFirstEventType(eventType) {
		return true
	}
	if visibleOutput {
		return isOpenAIWSTokenEvent(eventType)
	}
	return isLegacyOpenAIWSTokenEvent(eventType)
}

// isLegacyOpenAIWSTokenEvent preserves the 0.1.179 classifier used by the
// regular WS, WS ingress, and HTTP bridge paths.
func isLegacyOpenAIWSTokenEvent(eventType string) bool {
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		return false
	}
	switch eventType {
	case "response.created", "response.in_progress", "response.output_item.added", "response.output_item.done":
		return false
	}
	if strings.Contains(eventType, ".delta") {
		return true
	}
	if strings.HasPrefix(eventType, "response.output_text") {
		return true
	}
	if strings.HasPrefix(eventType, "response.output") {
		return true
	}
	return false
}

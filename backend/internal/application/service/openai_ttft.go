package service

import (
	"context"
	"strings"
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

func openAIStreamDataStartsTTFT(trimmedData, eventType string, visibleOutput bool) bool {
	if visibleOutput {
		return openAIStreamDataStartsVisibleOutput(trimmedData, eventType)
	}
	return openAIStreamDataStartsClientOutputTrimmed(strings.TrimSpace(trimmedData), strings.TrimSpace(eventType))
}

func isOpenAIWSTTFTEvent(eventType string, visibleOutput bool) bool {
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

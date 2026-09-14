package service

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

type openAIVisibleOutputTTFTContextKey struct{}

const openAIRequestFirstEventTTFTKey = "openai_request_first_event_ttft"

type openAIRequestFirstEventTTFTState struct {
	startedAt time.Time
	firstMs   *int
}

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

// BeginOpenAIRequestFirstEventTTFT establishes one clock for all account
// attempts made by a single HTTP Responses request. A replay-safe control frame
// can be delivered by an attempt that later fails, so attempt-local clocks are
// insufficient for the client-observed first-event latency.
func BeginOpenAIRequestFirstEventTTFT(c *gin.Context, startedAt time.Time) {
	if c == nil || startedAt.IsZero() {
		return
	}
	c.Set(openAIRequestFirstEventTTFTKey, &openAIRequestFirstEventTTFTState{startedAt: startedAt})
}

// recordOpenAIRequestFirstEventDelivered captures only an event that has
// crossed a complete downstream flush boundary. Buffered metadata and
// keepalives do not call this function.
func recordOpenAIRequestFirstEventDelivered(c *gin.Context) {
	if c == nil {
		return
	}
	value, exists := c.Get(openAIRequestFirstEventTTFTKey)
	if !exists {
		return
	}
	state, ok := value.(*openAIRequestFirstEventTTFTState)
	if !ok || state == nil || state.firstMs != nil || state.startedAt.IsZero() {
		return
	}
	elapsed := time.Since(state.startedAt).Milliseconds()
	if elapsed < 0 {
		return
	}
	firstMs := int(elapsed)
	state.firstMs = &firstMs
}

// PreferOpenAIRequestFirstEventTTFT keeps the first client-observed control
// event across failover attempts. Attempt-local TTFT values use a different
// clock origin, so they must not be numerically compared with this request-level
// duration. It is called only for the final successful result, so a failed
// request still does not create a successful TTFT sample.
func PreferOpenAIRequestFirstEventTTFT(c *gin.Context, current *int) *int {
	if c == nil {
		return current
	}
	value, exists := c.Get(openAIRequestFirstEventTTFTKey)
	if !exists {
		return current
	}
	state, ok := value.(*openAIRequestFirstEventTTFTState)
	if !ok || state == nil || state.firstMs == nil || *state.firstMs < 0 {
		return current
	}
	resolved := *state.firstMs
	return &resolved
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

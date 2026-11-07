package service

import (
	"encoding/json"
	"fmt"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/tidwall/gjson"
)

// Keep only output state, rather than retaining every WS frame while an HTTP
// client waits for JSON. Prewarm events are consumed before this collector starts.
type openAIWSNonStreamingResponse struct {
	output    *apicompat.BufferedResponseAccumulator
	doneItems *responsesStreamOutputItems
	createdAt responsesCreatedAtStamper
	readBytes int64
	maxBytes  int64
	response  []byte
}

func newOpenAIWSNonStreamingResponse(cfg *config.Config) *openAIWSNonStreamingResponse {
	return &openAIWSNonStreamingResponse{
		output: apicompat.NewBufferedResponseAccumulator(), doneItems: newResponsesStreamOutputItems(),
		maxBytes: resolveUpstreamResponseReadLimit(cfg),
	}
}

func (r *openAIWSNonStreamingResponse) Observe(message []byte, eventType string) error {
	if r == nil {
		return nil
	}
	if int64(len(message)) > r.maxBytes-r.readBytes {
		return fmt.Errorf("%w: limit=%d", ErrUpstreamResponseBodyTooLarge, r.maxBytes)
	}
	r.readBytes += int64(len(message))
	r.createdAt.Observe(message, eventType)
	r.doneItems.Observe(message)
	switch eventType {
	case "response.output_text.delta", "response.refusal.delta", "response.output_item.added",
		"response.function_call_arguments.delta", "response.function_call_arguments.done",
		"response.custom_tool_call_input.delta", "response.custom_tool_call_input.done",
		"response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		var event apicompat.ResponsesStreamEvent
		if err := json.Unmarshal(message, &event); err == nil {
			r.output.ProcessEvent(&event)
		}
	}
	// A created/in_progress snapshot is never a final HTTP response, including
	// when a malformed terminal event omits its response object.
	if !isOpenAIWSTerminalEvent(eventType) || !gjson.GetBytes(message, "response").IsObject() {
		return nil
	}
	if normalized, changed := normalizeResponsesStreamingTerminalOutput(message, r.output, r.doneItems, nil); changed {
		message = normalized
	}
	if stamped, changed := r.createdAt.Stamp(message, eventType); changed {
		message = stamped
	}
	r.response = []byte(gjson.GetBytes(message, "response").Raw)
	return nil
}

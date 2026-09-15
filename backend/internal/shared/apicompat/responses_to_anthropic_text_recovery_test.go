package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func anthropicTextDeltas(events []AnthropicStreamEvent) string {
	text := ""
	for _, event := range events {
		if event.Delta != nil && event.Delta.Type == "text_delta" {
			text += event.Delta.Text
		}
	}
	return text
}

func TestResponsesEventToAnthropicEventsRecoversTextFromDone(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.output_text.done", OutputIndex: 0, ContentIndex: 0, Text: "complete",
	}, state)
	require.Equal(t, "complete", anthropicTextDeltas(events))
	require.False(t, state.ContentBlockOpen)
}

func TestResponsesEventToAnthropicEventsRecoversOnlyMissingDoneTail(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	delta := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.output_text.delta", OutputIndex: 0, ContentIndex: 0, Delta: "hel",
	}, state)
	done := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.output_text.done", OutputIndex: 0, ContentIndex: 0, Text: "hello",
	}, state)
	require.Equal(t, "hel", anthropicTextDeltas(delta))
	require.Equal(t, "lo", anthropicTextDeltas(done))
}

func TestResponsesEventToAnthropicEventsRecoversTerminalTextAfterOtherBlocks(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	state.ContentBlockIndex = 1
	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type: "response.completed",
		Response: &ResponsesResponse{Output: []ResponsesOutput{{
			Type: "message", Content: []ResponsesContentPart{{Type: "output_text", Text: "terminal"}},
		}}},
	}, state)
	require.Equal(t, "terminal", anthropicTextDeltas(events))
}

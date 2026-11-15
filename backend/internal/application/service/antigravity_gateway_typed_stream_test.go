package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/antigravity"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/apicompat"
	"github.com/stretchr/testify/require"
)

type antigravityConvertedEventSink struct {
	events []apicompat.AnthropicStreamEvent
}

func (s *antigravityConvertedEventSink) ConsumeClaudeStreamEvent(event antigravity.ClaudeStreamEvent) error {
	s.events = append(s.events, convertAntigravityClaudeStreamEvent(event))
	return nil
}

func TestAntigravityTypedStreamMatchesByteLifecycle(t *testing.T) {
	lines := []string{
		`data: {"response":{"responseId":"resp_full","candidates":[{"content":{"parts":[{"text":"thinking","thought":true,"thoughtSignature":"sig_thinking"}]}}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":1,"thoughtsTokenCount":2}}}`,
		`data: {"response":{"responseId":"resp_full","candidates":[{"content":{"parts":[{"text":"answer"}]}}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":2,"thoughtsTokenCount":2}}}`,
		`data: {"response":{"responseId":"resp_full","candidates":[{"content":{"parts":[{"functionCall":{"id":"call_1","name":"lookup","args":{"q":"value"}},"thoughtSignature":"sig_tool"}]}}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":3,"thoughtsTokenCount":2}}}`,
		`data: {"response":{"responseId":"resp_full","candidates":[{"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":11,"candidatesTokenCount":4,"thoughtsTokenCount":2}}}`,
	}

	byteProcessor := antigravity.NewStreamingProcessor("gemini-test")
	var byteOutput []byte
	for _, line := range lines {
		byteOutput = append(byteOutput, byteProcessor.ProcessLine(line)...)
	}
	finalBytes, _ := byteProcessor.Finish()
	byteOutput = append(byteOutput, finalBytes...)
	byteEvents := parseAnthropicSSEEventsForTest(t, byteOutput)

	typedProcessor := antigravity.NewStreamingProcessor("gemini-test")
	typedSink := &antigravityConvertedEventSink{}
	for _, line := range lines {
		require.NoError(t, typedProcessor.ProcessLineEvents(line, typedSink))
	}
	_, err := typedProcessor.FinishEvents(typedSink)
	require.NoError(t, err)

	require.Equal(t, byteEvents, typedSink.events)
	requireEventTypePresent(t, typedSink.events, "message_start")
	requireEventTypePresent(t, typedSink.events, "message_delta")
	requireEventTypePresent(t, typedSink.events, "message_stop")
	requireDeltaTypePresent(t, typedSink.events, "thinking_delta")
	requireDeltaTypePresent(t, typedSink.events, "signature_delta")
	requireDeltaTypePresent(t, typedSink.events, "input_json_delta")
}

func parseAnthropicSSEEventsForTest(t *testing.T, data []byte) []apicompat.AnthropicStreamEvent {
	t.Helper()
	var events []apicompat.AnthropicStreamEvent
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		var event apicompat.AnthropicStreamEvent
		require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &event))
		events = append(events, event)
	}
	return events
}

func requireEventTypePresent(t *testing.T, events []apicompat.AnthropicStreamEvent, eventType string) {
	t.Helper()
	for _, event := range events {
		if event.Type == eventType {
			return
		}
	}
	t.Fatalf("event type %q not found", eventType)
}

func requireDeltaTypePresent(t *testing.T, events []apicompat.AnthropicStreamEvent, deltaType string) {
	t.Helper()
	for _, event := range events {
		if event.Delta != nil && event.Delta.Type == deltaType {
			return
		}
	}
	t.Fatalf("delta type %q not found", deltaType)
}

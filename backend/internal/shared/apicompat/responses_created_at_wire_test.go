package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func responseObjectWithCreatedAt(t *testing.T, event ResponsesStreamEvent) map[string]any {
	t.Helper()
	wire := marshalEvent(t, event)
	response, ok := wire["response"].(map[string]any)
	require.True(t, ok, "event must carry a response object: %v", wire)
	return response
}

func requireResponseCreatedAt(t *testing.T, response map[string]any) int64 {
	t.Helper()
	raw, ok := response["created_at"]
	require.True(t, ok, "response object must carry created_at")
	value, ok := raw.(float64)
	require.True(t, ok, "created_at must be numeric, got %T", raw)
	require.Greater(t, int64(value), int64(0))
	return int64(value)
}

func TestResponsesResponseWireCreatedAtPresentAtZero(t *testing.T) {
	response := responseObjectWithCreatedAt(t, ResponsesStreamEvent{
		Type: "response.created",
		Response: &ResponsesResponse{
			ID: "resp_1", Object: "response", Status: "in_progress",
		},
	})

	require.Contains(t, response, "created_at")
	require.EqualValues(t, 0, response["created_at"])
}

func TestChatCompletionsResponseToResponsesCarriesCreatedAt(t *testing.T) {
	t.Run("upstream timestamp", func(t *testing.T) {
		response := ChatCompletionsResponseToResponses(&ChatCompletionsResponse{
			ID:      "chatcmpl_1",
			Created: 1_700_000_000,
			Model:   "deepseek-v4-flash",
			Choices: []ChatChoice{{Message: ChatMessage{Role: "assistant", Content: json.RawMessage(`"hi"`)}}},
		}, "deepseek-v4-flash", nil, false, nil)

		require.EqualValues(t, 1_700_000_000, response.CreatedAt)
	})

	for _, test := range []struct {
		name     string
		response *ChatCompletionsResponse
	}{
		{
			name: "missing timestamp",
			response: &ChatCompletionsResponse{
				ID:      "chatcmpl_2",
				Model:   "deepseek-v4-flash",
				Choices: []ChatChoice{{Message: ChatMessage{Role: "assistant", Content: json.RawMessage(`"hi"`)}}},
			},
		},
		{name: "nil response"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := ChatCompletionsResponseToResponses(test.response, "deepseek-v4-flash", nil, false, nil)
			require.Greater(t, response.CreatedAt, int64(0))
		})
	}
}

func TestChatCompletionsToResponsesStreamCreatedAtStable(t *testing.T) {
	state := NewChatCompletionsToResponsesStreamState("deepseek-v4-flash")
	var chunk ChatCompletionsChunk
	require.NoError(t, json.Unmarshal(
		[]byte(`{"choices":[{"index":0,"delta":{"content":"hi"}}]}`), &chunk,
	))

	events := ChatCompletionsChunkToResponsesEvents(&chunk, state)
	events = append(events, FinalizeChatCompletionsResponsesStream(state)...)

	seen := map[string]int64{}
	for _, event := range events {
		if event.Response != nil {
			seen[event.Type] = requireResponseCreatedAt(t, responseObjectWithCreatedAt(t, event))
		}
	}
	require.Equal(t, state.Created, seen["response.created"])
	require.Equal(t, seen["response.created"], seen["response.completed"])
}

func TestAnthropicToResponsesCreatedAt(t *testing.T) {
	response := AnthropicToResponsesResponse(&AnthropicResponse{
		ID:      "msg_1",
		Type:    "message",
		Role:    "assistant",
		Model:   "claude-sonnet-4-20250514",
		Content: []AnthropicContentBlock{{Type: "text", Text: "hi"}},
	})
	require.Greater(t, response.CreatedAt, int64(0))
}

func TestAnthropicToResponsesStreamCreatedAtStable(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	state.Model = "claude-sonnet-4-20250514"

	var events []ResponsesStreamEvent
	for _, raw := range []string{
		`{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","content":[],"usage":{"input_tokens":3,"output_tokens":0}}}`,
		`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
		`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
		`{"type":"content_block_stop","index":0}`,
		`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
		`{"type":"message_stop"}`,
	} {
		var event AnthropicStreamEvent
		require.NoError(t, json.Unmarshal([]byte(raw), &event))
		events = append(events, AnthropicEventToResponsesEvents(&event, state)...)
	}
	events = append(events, FinalizeAnthropicResponsesStream(state)...)

	seen := map[string]int64{}
	for _, event := range events {
		if event.Response != nil {
			seen[event.Type] = requireResponseCreatedAt(t, responseObjectWithCreatedAt(t, event))
		}
	}
	require.Equal(t, state.Created, seen["response.created"])
	require.Equal(t, seen["response.created"], seen["response.completed"])
}

func TestResponsesStreamEventCreatedAtSurvivesRemarshal(t *testing.T) {
	raw := []byte(`{"type":"response.completed","response":{"id":"resp_9","object":"response","created_at":1700000123,"model":"gpt-5.5","status":"completed","output":[]}}`)
	var event ResponsesStreamEvent
	require.NoError(t, json.Unmarshal(raw, &event))
	require.EqualValues(t, 1_700_000_123, event.Response.CreatedAt)
	require.EqualValues(t, 1_700_000_123, requireResponseCreatedAt(t, responseObjectWithCreatedAt(t, event)))
}

package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResponsesToAnthropicIgnoresStaleToolEventsByOutputIndex(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	addTool := func(outputIndex int, name string) []AnthropicStreamEvent {
		return ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
			Type:        "response.output_item.added",
			OutputIndex: outputIndex,
			Item:        &ResponsesOutput{Type: "function_call", CallID: "call_" + name, Name: name},
		}, state)
	}

	require.Len(t, addTool(10, "first"), 1)
	require.Len(t, addTool(11, "second"), 2)
	require.True(t, state.ContentBlockOpen)
	require.Equal(t, 1, state.ContentBlockIndex)
	require.Equal(t, "second", state.CurrentToolName)

	staleEvents := []*ResponsesStreamEvent{
		{Type: "response.function_call_arguments.delta", OutputIndex: 10, Delta: `{"stale":true}`},
		{Type: "response.function_call_arguments.done", OutputIndex: 10, Arguments: `{"stale":true}`},
		{Type: "response.output_item.done", OutputIndex: 10, Item: &ResponsesOutput{Type: "function_call"}},
		{Type: "response.output_item.done", OutputIndex: 99, Item: &ResponsesOutput{Type: "unknown"}},
	}
	for _, event := range staleEvents {
		require.Empty(t, ResponsesEventToAnthropicEvents(event, state))
		require.True(t, state.ContentBlockOpen)
		require.Equal(t, 1, state.ContentBlockIndex)
		require.Equal(t, "second", state.CurrentToolName)
		require.False(t, state.CurrentToolHadDelta)
	}

	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.delta",
		OutputIndex: 11,
		Delta:       `{"current":true}`,
	}, state)
	require.Len(t, events, 1)
	require.Equal(t, 1, *events[0].Index)

	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.function_call_arguments.done",
		OutputIndex: 11,
		Arguments:   `{"current":true}`,
	}, state)
	require.Len(t, events, 1)
	require.Equal(t, "content_block_stop", events[0].Type)
	require.Equal(t, 1, *events[0].Index)
}

func TestResponsesToAnthropicIgnoresStaleReasoningDoneByOutputIndex(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	addReasoning := func(outputIndex int) []AnthropicStreamEvent {
		return ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
			Type:        "response.output_item.added",
			OutputIndex: outputIndex,
			Item:        &ResponsesOutput{Type: "reasoning"},
		}, state)
	}

	require.Len(t, addReasoning(20), 1)
	require.Len(t, addReasoning(21), 2)
	require.Empty(t, ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.reasoning_summary_text.done",
		OutputIndex: 20,
	}, state))
	require.True(t, state.ContentBlockOpen)
	require.Equal(t, 1, state.ContentBlockIndex)

	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.reasoning_summary_text.done",
		OutputIndex: 21,
	}, state)
	require.Empty(t, events, "thinking stays open until encrypted_content can arrive on output_item.done")
	require.True(t, state.ContentBlockOpen)

	require.Empty(t, ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.output_item.done",
		OutputIndex: 20,
		Item:        &ResponsesOutput{Type: "reasoning", EncryptedContent: "stale-signature"},
	}, state))
	require.True(t, state.ContentBlockOpen)

	events = ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{
		Type:        "response.output_item.done",
		OutputIndex: 21,
		Item:        &ResponsesOutput{Type: "reasoning", EncryptedContent: "current-signature"},
	}, state)
	require.Len(t, events, 2)
	require.Equal(t, "signature_delta", events[0].Delta.Type)
	require.Equal(t, "current-signature", events[0].Delta.Signature)
	require.Equal(t, "content_block_stop", events[1].Type)
	require.Equal(t, 1, *events[1].Index)
}

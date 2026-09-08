package apicompat

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatArgumentsDone(t *testing.T) {
	for _, toolType := range []string{"function_call", "custom_tool_call"} {
		for _, prefix := range []string{"", `{"x":`, `{"x":1}`} {
			t.Run(toolType+prefix, func(t *testing.T) {
				state := NewResponsesEventToChatState()
				ResponsesEventToChatChunks(&ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: 3,
					Item: &ResponsesOutput{Type: toolType, CallID: "call_a", Name: "lookup"}}, state)
				deltaType, doneType := "response.function_call_arguments.delta", "response.function_call_arguments.done"
				if toolType == "custom_tool_call" {
					deltaType, doneType = "response.custom_tool_call_input.delta", "response.custom_tool_call_input.done"
				}
				ResponsesEventToChatChunks(&ResponsesStreamEvent{Type: deltaType, OutputIndex: 3, Delta: prefix}, state)
				done := &ResponsesStreamEvent{Type: doneType, OutputIndex: 3, Arguments: `{"x":1}`, Input: `{"x":1}`}
				chunks := ResponsesEventToChatChunks(done, state)
				if prefix == done.Arguments {
					require.Empty(t, chunks)
				} else {
					require.Len(t, chunks, 1)
					require.Equal(t, done.Arguments[len(prefix):], chunks[0].Choices[0].Delta.ToolCalls[0].Function.Arguments)
				}
				require.Empty(t, ResponsesEventToChatChunks(done, state))
				require.Empty(t, ResponsesEventToChatChunks(&ResponsesStreamEvent{Type: deltaType, OutputIndex: 3, Delta: "late"}, state))
			})
		}
	}
}

func TestChatArgumentsDoneRejectsConflictingPrefix(t *testing.T) {
	state := NewResponsesEventToChatState()
	state.OutputIndexToToolIndex[0] = 0
	resToChatHandleFuncArgsDelta(&ResponsesStreamEvent{OutputIndex: 0, Delta: "abc"}, state)
	for _, value := range []string{"ab", "wrong"} {
		require.Empty(t, resToChatHandleFuncArgsDone(&ResponsesStreamEvent{Arguments: value}, state))
	}
	require.Empty(t, resToChatHandleFuncArgsDone(&ResponsesStreamEvent{OutputIndex: 9, Arguments: "abc"}, state))
	state.Finalized = true
	require.Empty(t, resToChatHandleFuncArgsDone(&ResponsesStreamEvent{Arguments: "abcd"}, state))
}

func TestBufferedArgumentsDoneIdentityAndGrowth(t *testing.T) {
	a := NewBufferedResponseAccumulator()
	for i := 0; i < 64; i++ {
		call := fmt.Sprintf("call_%d", i)
		a.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: i,
			Item: &ResponsesOutput{Type: "function_call", CallID: call, Name: "lookup"}})
		a.ProcessEvent(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: i, Delta: `{"x":`})
	}
	// Appending other tools must not copy an already-used strings.Builder.
	a.ProcessEvent(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: "0}"})
	a.ProcessEvent(&ResponsesStreamEvent{Type: "response.function_call_arguments.done", OutputIndex: 63, Arguments: `{"x":63}`})
	response := &ResponsesResponse{Output: []ResponsesOutput{
		{Type: "function_call", CallID: "call_63", Name: "lookup"},
		{Type: "function_call", CallID: "unrelated", Name: "lookup"},
		{Type: "function_call", CallID: "call_0", Name: "lookup", Arguments: "explicit"},
	}}
	a.SupplementResponseOutput(response)
	require.Equal(t, `{"x":63}`, response.Output[0].Arguments)
	require.Empty(t, response.Output[1].Arguments)
	require.Equal(t, "explicit", response.Output[2].Arguments)
	empty := &ResponsesResponse{}
	a.SupplementResponseOutput(empty)
	require.Equal(t, `{"x":0}`, empty.Output[0].Arguments)
}

var argumentBenchmarkSink int

func BenchmarkUpstreamSyncArgumentTracking(b *testing.B) {
	fragment := strings.Repeat("x", 64)
	completed := strings.Repeat(fragment, 1024)
	b.Run("upstream_concat", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			current := ""
			for range 1024 {
				current += fragment
			}
			if !strings.HasPrefix(completed, current) {
				b.Fatal("prefix mismatch")
			}
			argumentBenchmarkSink = len(current)
		}
	})
	b.Run("bounded_digest", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			state := &ResponsesEventToChatState{OutputIndexToToolIndex: map[int]int{0: 0}}
			progress := state.argumentProgress(0)
			for range 1024 {
				progress.append(fragment)
			}
			resToChatHandleFuncArgsDone(&ResponsesStreamEvent{Arguments: completed}, state)
			if !progress.done {
				b.Fatal("prefix mismatch")
			}
			argumentBenchmarkSink = progress.bytes
		}
	})
}

package apicompat

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamAccumulationReadRawArgumentsAndReset(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	startTool := func(idx int) {
		ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: idx, Item: &ResponsesOutput{Type: "function_call", CallID: fmt.Sprintf("call_%d", idx), Name: "Read"}}, state)
	}
	startTool(0)
	state.CurrentToolArgs = `{"file_path":"`
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: "/tmp/one}"}, state)
	require.Equal(t, `{"file_path":"/tmp/one}`, state.CurrentToolArgs)
	require.NotNil(t, state.currentToolArgsValidation)
	require.Equal(t, state.CurrentToolArgs, string(state.currentToolArgsValidation.data))
	firstSnapshot := state.CurrentToolArgs
	firstValidationSnapshot := state.currentToolArgsValidation.data
	// Same-length caller replacement must also be respected by this special
	// path, whose externally visible raw JSON differs from sanitized output.
	state.CurrentToolArgs = `{"file_path":"/tmp/two}`
	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: `","pages":""}`}, state)
	require.Len(t, events, 1)
	require.JSONEq(t, `{"file_path":"/tmp/two}"}`, events[0].Delta.PartialJSON)
	require.Equal(t, `{"file_path":"/tmp/two}","pages":""}`, state.CurrentToolArgs)
	require.Equal(t, state.CurrentToolArgs, string(state.currentToolArgsValidation.data))
	require.Equal(t, firstSnapshot, string(firstValidationSnapshot), "external replacement must not overwrite a borrowed validation prefix")
	rawSnapshot := state.CurrentToolArgs
	require.Equal(t, `{"file_path":"/tmp/one}`, firstSnapshot)

	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.function_call_arguments.done", OutputIndex: 0}, state)
	require.Empty(t, state.CurrentToolArgs)
	require.Nil(t, state.currentToolArgsValidation)
	startTool(1)
	require.Nil(t, state.currentToolArgsValidation)
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 1, Delta: `{"file_path":"/tmp/new"}`}, state)
	require.Equal(t, `{"file_path":"/tmp/new"}`, state.CurrentToolArgs)
	require.Equal(t, state.CurrentToolArgs, string(state.currentToolArgsValidation.data))
	require.Equal(t, `{"file_path":"/tmp/two}","pages":""}`, rawSnapshot)
	require.JSONEq(t, `{"file_path":"/tmp/two}"}`, events[0].Delta.PartialJSON)
}

func TestStreamAccumulationReadValidationCopiedStateBranches(t *testing.T) {
	original := NewResponsesEventToAnthropicState()
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: 0, Item: &ResponsesOutput{Type: "function_call", CallID: "call_read", Name: "Read"}}, original)
	appendDelta := func(state *ResponsesEventToAnthropicState, value string) {
		t.Helper()
		events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: value}, state)
		require.Empty(t, events, "unfinished strings must not pass the complete-JSON gate")
		require.NotNil(t, state.currentToolArgsValidation)
		require.Equal(t, state.CurrentToolArgs, string(state.currentToolArgsValidation.data))
		require.Equal(t, state.CurrentToolArgs, state.currentToolArgsValidation.snapshot)
	}
	original.CurrentToolArgs = `{"file_path":"`
	appendDelta(original, "base}")
	baseSnapshot := original.CurrentToolArgs
	baseValidationSnapshot := original.currentToolArgsValidation.data
	copied := *original

	appendDelta(original, "/left}")
	leftValidationSnapshot := original.currentToolArgsValidation.data
	appendDelta(&copied, "/right}")
	require.Equal(t, `{"file_path":"base}/left}`, original.CurrentToolArgs)
	require.Equal(t, original.CurrentToolArgs, string(original.currentToolArgsValidation.data))
	require.Equal(t, `{"file_path":"base}/right}`, copied.CurrentToolArgs)
	appendDelta(original, "/again}")
	require.Equal(t, `{"file_path":"base}/left}/again}`, original.CurrentToolArgs)
	require.Equal(t, `{"file_path":"base}/right}`, string(copied.currentToolArgsValidation.data))
	require.Equal(t, baseSnapshot, string(baseValidationSnapshot))
	require.Equal(t, `{"file_path":"base}/left}`, string(leftValidationSnapshot))

	beforeReset := original.currentToolArgsValidation.data
	original.CurrentToolArgs = ""
	appendDelta(original, `{"file_path":"reset}`)
	require.Equal(t, `{"file_path":"reset}`, original.CurrentToolArgs)
	require.Equal(t, `{"file_path":"base}/left}/again}`, string(beforeReset))
	require.Equal(t, `{"file_path":"base}/right}`, string(copied.currentToolArgsValidation.data))
}

func TestStreamAccumulationReadValidationStopsAfterComplete(t *testing.T) {
	state := NewResponsesEventToAnthropicState()
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: 0, Item: &ResponsesOutput{Type: "function_call", CallID: "call_read", Name: "Read"}}, state)
	const raw = `{"file_path":"/tmp/file","pages":""}`
	events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: raw}, state)
	require.Len(t, events, 1)
	require.True(t, state.CurrentToolHadDelta)
	validation := state.currentToolArgsValidation
	require.NotNil(t, validation)
	require.Equal(t, raw, string(validation.data))
	validationCapacity := cap(validation.data)

	for _, trailing := range []string{" ", strings.Repeat("x", 1024)} {
		previous := state.CurrentToolArgs
		events := ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: trailing}, state)
		require.Empty(t, events, "a completed Read tool must not emit another sanitized delta")
		require.Equal(t, previous+trailing, state.CurrentToolArgs, "the existing public string still accumulates late deltas")
		require.Same(t, validation, state.currentToolArgsValidation)
		require.Equal(t, raw, string(validation.data))
		require.Equal(t, raw, validation.snapshot)
		require.Equal(t, validationCapacity, cap(validation.data))
	}
	require.JSONEq(t, `{"file_path":"/tmp/file"}`, events[0].Delta.PartialJSON)
	ResponsesEventToAnthropicEvents(&ResponsesStreamEvent{Type: "response.function_call_arguments.done", OutputIndex: 0}, state)
	require.Nil(t, state.currentToolArgsValidation)
	require.Empty(t, state.CurrentToolArgs)
}

func TestStreamAccumulationDeferredAnthropicTools(t *testing.T) {
	state := NewChatCompletionsToAnthropicStreamState("model")
	var events []AnthropicStreamEvent
	feed := func(idx int, name, args string) {
		events = append(events, ChatCompletionsChunkToAnthropicEvents(streamAccumulationChatToolChunk(idx, name, args), state)...)
	}
	feed(0, "", `{"a":`)
	feed(1, "", `{"b":`)
	feed(0, "", "1")
	feed(1, "", "2")
	feed(0, "first", "}")
	feed(1, "second", "}")
	events = append(events, FinalizeChatCompletionsAnthropicStream(state)...)
	blocks := assembleToolUseBlocks(events)
	require.Len(t, blocks, 2)
	require.Equal(t, "first", blocks[0].Name)
	require.Equal(t, `{"a":1}`, blocks[0].Input)
	require.Equal(t, "second", blocks[1].Name)
	require.Equal(t, `{"b":2}`, blocks[1].Input)
	require.Empty(t, FinalizeChatCompletionsAnthropicStream(state))
}

var streamAccumulationBenchmarkSink string

// ReportAllocs measures cumulative allocation traffic, not retained heap/RSS.
// A fixed 64-byte delta makes total-size scaling expose repeated prefix copies.
func BenchmarkStreamAccumulationRead64ByteDeltas(b *testing.B) {
	for _, size := range []int{32 << 10, 128 << 10, 512 << 10} {
		b.Run(fmt.Sprintf("%dKiB", size>>10), func(b *testing.B) {
			const prefix = `{"file_path":"`
			firstEvent := &ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: prefix + strings.Repeat("x", 64-len(prefix))}
			nextEvent := &ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: strings.Repeat("x", 64)}
			b.ReportAllocs()
			b.SetBytes(int64(size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				state := NewResponsesEventToAnthropicState()
				state.MessageStartSent = true
				state.ContentBlockOpen = true
				state.CurrentBlockType = "tool_use"
				state.CurrentToolName = "Read"
				state.OutputIndexToBlockIdx[0] = 0
				ResponsesEventToAnthropicEvents(firstEvent, state)
				for received := 64; received < size; received += 64 {
					ResponsesEventToAnthropicEvents(nextEvent, state)
				}
				streamAccumulationBenchmarkSink = state.CurrentToolArgs
				if len(streamAccumulationBenchmarkSink) != size || state.CurrentToolHadDelta {
					b.Fatalf("unfinished Read arguments: bytes=%d, emitted=%v", len(streamAccumulationBenchmarkSink), state.CurrentToolHadDelta)
				}
			}
		})
	}
}

func streamAccumulationChatToolChunk(idx int, name, arguments string) *ChatCompletionsChunk {
	return &ChatCompletionsChunk{Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{{Index: &idx, ID: fmt.Sprintf("call_%d", idx), Type: "function", Function: ChatFunctionCall{Name: name, Arguments: arguments}}}}}}}
}

func FuzzStreamJSONCompletionPrecheck(f *testing.F) {
	for _, raw := range []string{`{"file_path":"abc"}`, `["}",2]`, `"text"`, `"escaped\\"quote"`, `null`, `false`, `123`, `1.25e2`, " \r\n {}\t", `{"file_path":"unfinished`, "", `{}`, `""`} {
		f.Add(raw)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		if json.Valid([]byte(raw)) && !streamJSONMayBeComplete(raw) {
			t.Fatalf("valid JSON skipped: %q", raw)
		}
	})
}

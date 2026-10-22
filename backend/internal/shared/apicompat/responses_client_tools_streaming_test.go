package apicompat

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestResponsesClientToolStreamRestorer_CustomInputBeforeDone(t *testing.T) {
	r := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}})
	r.Restore(ResponsesStreamEvent{Type: "response.output_item.added", SequenceNumber: 7,
		Item: &ResponsesOutput{Type: "function_call", ID: "fc_exec", CallID: "call_exec", Name: "exec"}})
	var input strings.Builder
	for _, fragment := range []string{`{"input":"hello`, ` world`, `"}`} {
		for _, event := range r.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.delta", ItemID: "fc_exec", Delta: fragment}) {
			require.Equal(t, "response.custom_tool_call_input.delta", event.Type)
			require.Equal(t, "ctc_exec", event.ItemID)
			_, _ = input.WriteString(event.Delta)
		}
	}
	t.Logf("before_done=%q", input.String())
	require.Equal(t, "hello world", input.String(), "tool input must reach the client while the upstream is still open")
	done := r.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.done", ItemID: "fc_exec", Arguments: `{"input":"hello world"}`})
	require.Len(t, done, 1, "done must not replay input already sent as deltas")
	require.Equal(t, "hello world", done[0].Input)
	t.Logf("done=%q duplicate_delta=false", done[0].Input)
}

func TestResponsesClientToolStreamRestorer_CustomInputEverySplit(t *testing.T) {
	cases := []string{
		`{"input":"hello world"}`,
		`{"input":"你好🚀"}`,
		`{"input":"a\n\r\t\b\f\"\\\/\u4f60\u597d\ud83d\ude80z"}`,
		`{"meta":{"input":"wrong","nested":["\\\"",{"x":true}]},"\u0069nput":"right","tail":"ignored"}`,
		`{"input":"\ud800x\udc00\ud800\ud800\udc00"}`,
		` { "input" : "" } `,
	}
	for _, arguments := range cases {
		for split := 0; split <= len(arguments); split++ {
			t.Run(fmt.Sprintf("%s/%d", arguments, split), func(t *testing.T) {
				assertCustomInputFragments(t, arguments, []string{arguments[:split], arguments[split:]})
			})
		}
		fragments := make([]string, len(arguments))
		// Internal chunks can split UTF-8 as well as JSON escape sequences.
		for i := 0; i < len(arguments); i++ {
			fragments[i] = arguments[i : i+1]
		}
		assertCustomInputFragments(t, arguments, fragments)
	}
}

func assertCustomInputFragments(t *testing.T, arguments string, fragments []string) {
	t.Helper()
	r := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}})
	r.Restore(ResponsesStreamEvent{Type: "response.output_item.added", SequenceNumber: 20, OutputIndex: 4,
		Item: &ResponsesOutput{Type: "function_call", ID: "fc_exec", CallID: "call_exec", Name: "exec"}})
	var input strings.Builder
	sequence := 21
	for _, fragment := range fragments {
		for _, event := range r.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 4, Delta: fragment}) {
			require.Equal(t, "response.custom_tool_call_input.delta", event.Type)
			require.True(t, utf8.ValidString(event.Delta))
			require.Equal(t, sequence, event.SequenceNumber)
			require.Equal(t, "ctc_exec", event.ItemID)
			require.Equal(t, 4, event.OutputIndex)
			_, _ = input.WriteString(event.Delta)
			sequence++
		}
	}
	require.Equal(t, extractCustomToolCallInput(arguments), input.String(), "all valid input must arrive before done")
	done := r.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.done", OutputIndex: 4, Arguments: arguments})
	require.Len(t, done, 1)
	require.Equal(t, sequence, done[0].SequenceNumber)
	require.Equal(t, input.String(), done[0].Input)
	require.Empty(t, r.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.done", OutputIndex: 4, Arguments: arguments}))
	closed := r.Restore(ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 4,
		Item: &ResponsesOutput{Type: "function_call", ID: "fc_exec", CallID: "call_exec", Name: "exec", Arguments: arguments}})
	require.Len(t, closed, 1)
	require.Equal(t, sequence+1, closed[0].SequenceNumber)
	require.Equal(t, input.String(), closed[0].Item.Input)
}

func TestResponsesClientToolStreamRestorer_CustomInputParallelCalls(t *testing.T) {
	r := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}})
	for i := 0; i < 2; i++ {
		r.Restore(ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: i,
			Item: &ResponsesOutput{Type: "function_call", ID: fmt.Sprintf("fc_%d", i), CallID: fmt.Sprintf("call_%d", i), Name: "exec"}})
	}
	for _, fragment := range []struct {
		index int
		value string
		want  string
	}{
		{0, `{"input":"left`, "left"},
		{1, `{"input":"right`, "right"},
		{0, ` side"}`, " side"},
		{1, ` side"}`, " side"},
	} {
		events := r.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: fragment.index, Delta: fragment.value})
		require.Len(t, events, 1)
		require.Equal(t, fragment.want, events[0].Delta)
		require.Equal(t, fmt.Sprintf("ctc_%d", fragment.index), events[0].ItemID)
	}
	for i, want := range []string{"left side", "right side"} {
		events := r.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.done", OutputIndex: i})
		require.Len(t, events, 1)
		require.Equal(t, want, events[0].Input)
	}
}

func TestResponsesClientToolStreamRestorer_CustomInputCompletionFallback(t *testing.T) {
	for _, arguments := range []string{`{"input":"done only"}`, `raw input`, `{"input":42}`, `{}`, `{"other":true}`} {
		for _, completion := range []string{"response.function_call_arguments.done", "response.output_item.done"} {
			t.Run(completion+arguments, func(t *testing.T) {
				r := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}})
				r.Restore(ResponsesStreamEvent{Type: "response.output_item.added", Item: &ResponsesOutput{Type: "function_call", ID: "fc_x", Name: "exec"}})
				event := ResponsesStreamEvent{Type: completion, ItemID: "fc_x", Arguments: arguments}
				if completion == "response.output_item.done" {
					event.Item = &ResponsesOutput{Type: "function_call", ID: "fc_x", Name: "exec", Arguments: arguments}
				}
				events := r.Restore(event)
				var input strings.Builder
				for _, restored := range events {
					if restored.Type == "response.custom_tool_call_input.delta" {
						_, _ = input.WriteString(restored.Delta)
					}
					if restored.Type == "response.custom_tool_call_input.done" {
						require.Equal(t, extractCustomToolCallInput(arguments), restored.Input)
					}
				}
				require.Equal(t, extractCustomToolCallInput(arguments), input.String())
			})
		}
	}
}

func TestResponsesClientToolStreamRestorer_CustomRawInputFallback(t *testing.T) {
	for _, arguments := range []string{`echo {"input":"embedded"}`, `[{"input":"nested"}]`, `"raw string"`} {
		r := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}})
		r.Restore(ResponsesStreamEvent{Type: "response.output_item.added", Item: &ResponsesOutput{Type: "function_call", ID: "fc_x", Name: "exec"}})
		require.Empty(t, r.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.delta", ItemID: "fc_x", Delta: arguments}))
		done := r.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.done", ItemID: "fc_x", Arguments: arguments})
		require.Len(t, done, 2)
		require.Equal(t, extractCustomToolCallInput(arguments), done[0].Delta)
		require.Equal(t, done[0].Delta, done[1].Input)
	}
}

func TestResponsesClientToolStreamRestorer_CustomInputExplicitIdentity(t *testing.T) {
	r := NewResponsesClientToolStreamRestorer(ResponsesClientToolMapping{CustomTools: map[string]bool{"exec": true}})
	for i := 0; i < 2; i++ {
		r.Restore(ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: i,
			Item: &ResponsesOutput{Type: "function_call", ID: fmt.Sprintf("fc_%d", i), CallID: fmt.Sprintf("call_%d", i), Name: "exec"}})
	}
	events := r.Restore(ResponsesStreamEvent{Type: "response.function_call_arguments.delta", CallID: "call_1", Delta: `{"input":"second`})
	require.Len(t, events, 1)
	require.Equal(t, "ctc_1", events[0].ItemID)
	require.Equal(t, 1, events[0].OutputIndex)
	require.Equal(t, "second", events[0].Delta)
	// Unknown functions remain opaque, even with no output_index on the wire.
	payloads, _, err := r.RestoreEvent([]byte(`{"type":"response.function_call_arguments.delta","sequence_number":30,"item_id":"fc_other","call_id":"call_other","delta":"{}","extension":{"keep":true}}`))
	require.NoError(t, err)
	require.Len(t, payloads, 1)
	require.Equal(t, "response.function_call_arguments.delta", gjson.GetBytes(payloads[0], "type").String())
	require.True(t, gjson.GetBytes(payloads[0], "extension.keep").Bool())
}

func TestChatCompletionsCustomInputBeforeFinish(t *testing.T) {
	for _, lateName := range []bool{false, true} {
		t.Run(fmt.Sprint(lateName), func(t *testing.T) {
			state := NewChatCompletionsToResponsesStreamState("test-model")
			state.CustomTools = map[string]bool{"exec": true}
			index := 0
			var input strings.Builder
			var itemID string
			for i, fragment := range []string{`{"input":"hello`, `\n\ud83d`, `\ude80 world"}`} {
				name := ""
				if (!lateName && i == 0) || (lateName && i == 1) {
					name = "exec"
				}
				chunk := &ChatCompletionsChunk{Choices: []ChatChunkChoice{{Delta: ChatDelta{ToolCalls: []ChatToolCall{{
					Index: &index, ID: "call_exec", Function: ChatFunctionCall{Name: name, Arguments: fragment},
				}}}}}}
				for _, event := range ChatCompletionsChunkToResponsesEvents(chunk, state) {
					if event.Type == "response.output_item.added" && event.Item.Type == "custom_tool_call" {
						itemID = event.Item.ID
					}
					if event.Type == "response.custom_tool_call_input.delta" {
						require.NotEmpty(t, itemID, "added must precede every input delta")
						require.Equal(t, itemID, event.ItemID)
						_, _ = input.WriteString(event.Delta)
					}
				}
			}
			require.Equal(t, "hello\n🚀 world", input.String(), "fallback must stream input before finish_reason or EOF")
			for _, event := range FinalizeChatCompletionsResponsesStream(state) {
				require.NotEqual(t, "response.custom_tool_call_input.delta", event.Type, "finalize must not repeat streamed input")
				if event.Type == "response.custom_tool_call_input.done" {
					require.Equal(t, input.String(), event.Input)
				}
			}
		})
	}
}

func BenchmarkResponsesCustomInputStream(b *testing.B) {
	for _, size := range []int{4096, 1024 * 1024} {
		arguments := `{"input":"` + strings.Repeat("x", size) + `"}`
		b.Run(fmt.Sprint(size), func(b *testing.B) {
			b.SetBytes(int64(len(arguments)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var stream responsesCustomInputStream
				for end := 64; end < len(arguments); end += 64 {
					stream.delta(arguments[:end])
				}
				stream.delta(arguments)
			}
		})
	}
}

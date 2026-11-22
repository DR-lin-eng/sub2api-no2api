package service

import (
	"encoding/json"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/apicompat"
	"github.com/stretchr/testify/require"
)

func TestOpenAIResponsesPlainOutputTextDelta(t *testing.T) {
	plain := []string{
		`{"type":"response.output_text.delta","sequence_number":7,"item_id":"msg_1","output_index":0,"content_index":0,"delta":"hello"}`,
		`{"type":"response.output_text.delta","delta":"","logprobs":[],"obfuscation":"aB3"}`,
		`{"type":"response.output_text.delta","delta":"line\n\"quoted\" \u4f60\ud83d\ude00 \\ end","logprobs":null}`,
		`{"type":"response.output_text.delta","delta":"你好 \"tool_calls\" image_generation_call"}`,
		`{"type":"response.output_text.delta","delta":"lone \ud83d surrogate"}`,
		`{"type":"response.output_text.delta" , "delta" : "spaced" }`,
		`{"type":"response.output_text.delta"}`,
	}
	for _, data := range plain {
		delta, ok := openAIResponsesPlainOutputTextDelta(data)
		require.True(t, ok, data)
		var event apicompat.ResponsesStreamEvent
		require.NoError(t, json.Unmarshal([]byte(data), &event), data)
		require.Equal(t, event.Delta, delta, data)
		require.Equal(t, openAIResponsesOutputTextDeltaEventType, event.Type)
	}

	notPlain := []string{
		``,
		`[DONE]`,
		`{"type":"response.output_text.done","text":"x"}`,
		`{"type":"response.output_text.delta.x","delta":"x"}`,
		`{"type":"response.output_text.delta","delta":"x","response":{"id":"resp_1"}}`,
		`{"type":"response.output_text.delta","delta":"x","id":"resp_1"}`,
		`{"type":"response.output_text.delta","delta":"x","model":"gpt-5"}`,
		`{"type":"response.output_text.delta","delta":"x","usage":{"images":1}}`,
		`{"type":"response.output_text.delta","delta":"x","item":{"type":"function_call"}}`,
		`{"type":"response.output_text.delta","delta":"x","error":{"message":"boom"}}`,
		`{"type":"response.output_text.delta","delta":"x","logprobs":[{"token":"x"}]}`,
		`{"type":"response.output_text.delta","delta":"x","delta":"y"}`,
		`{"type":"response.output_text.delta","delta":"x","type":"error"}`,
		`{"type":"response.output_text.delta","delta":"x","Delta":"y"}`,
		`{"type":"response.output_text.delta","del\u0074a":"x"}`,
		`{"type":"response.output_text.delta","delta":{"text":"x"}}`,
		`{"type":"response.output_text.delta","delta":1}`,
		`{"type":"response.output_text.delta","delta":"x","output_index":"0"}`,
		`{"type":"response.output_text.delta","delta":"x","output_index":-1}`,
		`{"type":"response.output_text.delta","delta":"x","sequence_number":1e3}`,
		`{"type":"response.output_text.delta","delta":"x","sequence_number":99999999999999999999}`,
		`{"type":"response.output_text.delta","delta":"x","item_id":null}`,
		`{"delta":"x","type":"response.output_text.delta"}`,
		` {"type":"response.output_text.delta","delta":"x"}`,
		`{"type":"response.output_text.delta","delta":"x"} `,
		`{"type":" response.output_text.delta","delta":"x"}`,
	}
	for _, data := range notPlain {
		_, ok := openAIResponsesPlainOutputTextDelta(data)
		require.False(t, ok, data)
	}
}

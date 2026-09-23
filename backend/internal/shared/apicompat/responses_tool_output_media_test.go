package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLiftResponsesToolOutputMedia(t *testing.T) {
	var input any
	require.NoError(t, json.Unmarshal([]byte(`[
		{"type":"function_call","call_id":"call_image","name":"view_image","arguments":"{}"},
		{"type":"function_call_output","call_id":"call_image","output":[{"type":"input_text","text":"ok"},{"type":"input_image","image_url":"data:image/png;base64,AQID"}]}
	]`), &input))
	lifted, changed := LiftResponsesToolOutputMedia(input)
	require.True(t, changed)
	encoded, err := json.Marshal(lifted)
	require.NoError(t, err)
	require.JSONEq(t, `[
		{"type":"function_call","call_id":"call_image","name":"view_image","arguments":"{}"},
		{"type":"function_call_output","call_id":"call_image","output":"[{\"text\":\"ok\",\"type\":\"input_text\"},{\"text\":\"[Tool output media moved to the following user message]\",\"type\":\"input_text\"}]"},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"[Tool output media for call call_image]"},{"type":"input_image","image_url":"data:image/png;base64,AQID"}]}
	]`, string(encoded))
}

func TestLiftResponsesToolOutputMediaLeavesPlainOutputUntouched(t *testing.T) {
	input := []any{map[string]any{"type": "function_call_output", "call_id": "call_text", "output": "plain output"}}
	lifted, changed := LiftResponsesToolOutputMedia(input)
	require.False(t, changed)
	require.Equal(t, input, lifted)
}

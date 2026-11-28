package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeCodexPageContext_TerminalInfo(t *testing.T) {
	raw := []byte(`{
		"model": "gpt-5.4",
		"input": [
			{
				"type": "function_call_output",
				"namespace": "agent",
				"call_id": "",
				"name": "terminal_info",
				"output": "cwd=/workspace\nshell=zsh"
			},
			{
				"type": "function_call_output",
				"namespace": "agent",
				"call_id": "",
				"name": "terminal_info",
				"output": "   "
			}
		]
	}`)

	normalized, changed := NormalizeCodexPageContext(raw)
	require.True(t, changed)

	var decoded struct {
		Input []map[string]any `json:"input"`
	}
	require.NoError(t, json.Unmarshal(normalized, &decoded))
	require.Len(t, decoded.Input, 2)
	require.Equal(t, "message", decoded.Input[0]["type"])
	require.Equal(t, "user", decoded.Input[0]["role"])
	content, ok := decoded.Input[0]["content"].([]any)
	require.True(t, ok)
	require.Len(t, content, 1)
	part, ok := content[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "<external_terminal_info>cwd=/workspace\nshell=zsh</external_terminal_info>", part["text"])
	require.Equal(t, "function_call_output", decoded.Input[1]["type"], "whitespace-only terminal_info must not rewrite")
}

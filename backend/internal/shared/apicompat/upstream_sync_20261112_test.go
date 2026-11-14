package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSealedCallMetadataSurvivesEmptyTerminalAndLateDone(t *testing.T) {
	for _, marker := range []string{`null`, `[]`, `{"sealed":"opaque"}`} {
		acc := NewBufferedResponseAccumulator()
		var added ResponsesStreamEvent
		require.NoError(t, json.Unmarshal([]byte(`{"type":"response.output_item.added","output_index":0,"item":{"type":"function_call","call_id":"call_1","name":"run","namespace":"tools","encrypted_function_args":`+marker+`}}`), &added))
		acc.ProcessEvent(&added)
		acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: 0, Delta: "ciphertext"})
		terminal := &ResponsesResponse{}
		acc.SupplementResponseOutput(terminal)
		wire, err := json.Marshal(terminal.Output)
		require.NoError(t, err)
		require.Equal(t, marker, gjson.GetBytes(wire, "0.encrypted_function_args").Raw)
		require.Equal(t, "tools", terminal.Output[0].Namespace)
		acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 0, Item: &ResponsesOutput{EncryptedFunctionArgs: json.RawMessage(`[]`)}})
		require.JSONEq(t, `[]`, string(acc.BuildOutput()[0].EncryptedFunctionArgs))
	}
}

func TestCodexPageSnapshotUsesSharedHTTPAndWSBootstrapOwner(t *testing.T) {
	for _, previous := range []string{"", `,"previous_response_id":"resp_1"`} {
		body := []byte(`{"model":"gpt-5.4","input":[{"type":"function_call_output","namespace":"agent","name":"codex_apps_open_page","output":"{\"page_id\":\"page-1\",\"text\":\"untrusted text\"}"}]` + previous + `}`)
		normalized, kind, changed := NormalizeCodexCallOutputBootstrap(body)
		require.True(t, changed)
		require.Equal(t, "page_context", kind)
		require.Equal(t, "user", gjson.GetBytes(normalized, "input.0.role").String())
		require.Contains(t, gjson.GetBytes(normalized, "input.0.content.0.text").String(), "<external_codex_apps_open_page>")
		_, _, again := NormalizeCodexCallOutputBootstrap(normalized)
		require.False(t, again)
	}
	for _, suffix := range []string{`,"call_id":"real-call"`, `,"call_id":null`, `,"namespace":"other"`} {
		body := []byte(`{"input":[{"type":"function_call_output","namespace":"agent","name":"codex_apps_open_page","output":"{\"page_id\":null}"` + suffix + `}]}`)
		got, _, changed := NormalizeCodexCallOutputBootstrap(body)
		require.False(t, changed)
		require.Equal(t, body, got)
	}
}

func TestClaude52SignedThinkingReplay(t *testing.T) {
	for _, model := range []string{"claude-fable-5-2", "claude-mythos-5-2"} {
		response := AnthropicToResponsesResponse(&AnthropicResponse{Model: model, Content: []AnthropicContentBlock{{Type: "thinking", Thinking: "summary", Signature: "provider-signed"}, {Type: "text", Text: "answer"}}})
		require.NotEmpty(t, response.Output[0].EncryptedContent)
		raw, err := json.Marshal(response.Output)
		require.NoError(t, err)
		replay, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: model, Input: raw})
		require.NoError(t, err)
		require.Equal(t, "adaptive", replay.Thinking.Type)
		require.Equal(t, "high", replay.OutputConfig.Effort)
		require.Contains(t, string(replay.Messages[0].Content), "provider-signed")
	}
}

package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSyncBufferedEncryptedCallContentAcrossEventShapes(t *testing.T) {
	acc := NewBufferedResponseAccumulator()
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_item.added", OutputIndex: 0, Item: &ResponsesOutput{Type: "function_call", CallID: "call_fixture", Name: "fixture", EncryptedContent: "added"}})
	require.Equal(t, "added", acc.BuildOutput()[0].EncryptedContent)
	acc.ProcessEvent(&ResponsesStreamEvent{Type: "response.output_item.done", OutputIndex: 0, Item: &ResponsesOutput{Type: "function_call", CallID: "call_fixture", Name: "fixture", EncryptedContent: "done"}})
	require.Equal(t, "done", acc.BuildOutput()[0].EncryptedContent)
	response := &ResponsesResponse{Output: []ResponsesOutput{{Type: "function_call", CallID: "call_fixture", Name: "fixture"}}}
	acc.SupplementResponseOutput(response)
	require.Equal(t, "done", response.Output[0].EncryptedContent)
	response.Output[0].EncryptedContent = "terminal-authoritative"
	acc.SupplementResponseOutput(response)
	require.Equal(t, "terminal-authoritative", response.Output[0].EncryptedContent)
}

func TestSyncExternalContextNeverPromotesRealToolResults(t *testing.T) {
	for _, name := range []string{"browser_info", "goal_objective", "untrusted_context"} {
		body := []byte(`{"number":9007199254740993,"input":[{"type":"function_call_output","namespace":"agent","name":"` + name + `","output":"external"}]}`)
		out, changed := NormalizeCodexPageContext(body)
		require.True(t, changed)
		require.Contains(t, string(out), `"role":"user"`)
		require.Contains(t, string(out), "9007199254740993")
		real := []byte(`{"input":[{"type":"function_call_output","namespace":"agent","call_id":"call_real","name":"` + name + `","output":"external"}]}`)
		out, changed = NormalizeCodexPageContext(real)
		require.False(t, changed)
		require.Equal(t, real, out)
	}
}

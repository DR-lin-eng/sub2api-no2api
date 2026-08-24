//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestNormalizeResponsesStreamingTerminalOutputPreservesReportedItems(t *testing.T) {
	doneItems := newResponsesStreamOutputItems()
	doneItems.Observe([]byte(`{"type":"response.output_item.done","output_index":0,"item":{"id":"rs_1","type":"reasoning","encrypted_content":"opaque"}}`))
	doneItems.Observe([]byte(`{"type":"response.output_item.done","output_index":1,"item":{"id":"msg_1","type":"message","status":"completed","phase":"final_answer","content":[{"type":"output_text","text":"shipped"}]}}`))

	normalized, changed := normalizeResponsesStreamingTerminalOutput(
		[]byte(`{"type":"response.completed","response":{"status":"completed","output":[]}}`),
		nil,
		doneItems,
		nil,
	)
	require.True(t, changed)
	require.Len(t, gjson.GetBytes(normalized, "response.output").Array(), 2)
	require.Equal(t, "opaque", gjson.GetBytes(normalized, "response.output.0.encrypted_content").String())
	require.Equal(t, "msg_1", gjson.GetBytes(normalized, "response.output.1.id").String())
	require.Equal(t, "final_answer", gjson.GetBytes(normalized, "response.output.1.phase").String())
}

func TestResponsesStreamOutputItemsOrderByOutputIndex(t *testing.T) {
	doneItems := newResponsesStreamOutputItems()
	doneItems.Observe([]byte(`{"type":"response.output_item.done","output_index":2,"item":{"id":"c","type":"message"}}`))
	doneItems.Observe([]byte(`{"type":"response.output_item.done","output_index":0,"item":{"id":"a","type":"reasoning"}}`))

	built, ok := doneItems.BuildOutput()
	require.True(t, ok)
	require.Equal(t, "a", gjson.GetBytes(built, "0.id").String())
	require.Equal(t, "c", gjson.GetBytes(built, "1.id").String())
}

func TestNormalizeResponsesStreamingTerminalOutputLeavesCompleteOutputAlone(t *testing.T) {
	doneItems := newResponsesStreamOutputItems()
	doneItems.Observe([]byte(`{"type":"response.output_item.done","output_index":0,"item":{"id":"msg_real","type":"message","status":"completed"}}`))
	raw := []byte(`{"type":"response.completed","response":{"output":[{"id":"msg_upstream","type":"message","status":"completed"}]}}`)
	normalized, changed := normalizeResponsesStreamingTerminalOutput(raw, nil, doneItems, nil)
	require.False(t, changed)
	require.Equal(t, string(raw), string(normalized))
}

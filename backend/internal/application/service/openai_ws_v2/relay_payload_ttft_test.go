package openai_ws_v2

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPayloadTTFTClassifierRunsOncePerMessage(t *testing.T) {
	start := time.Now()
	payload := []byte(`{"type":"response.output_text.delta","response_id":"resp_payload","delta":"answer"}`)
	calls := 0
	state := &relayState{isFirstTokenEvent: func(eventType string, message []byte) bool {
		calls++
		require.Equal(t, "response.output_text.delta", eventType)
		require.Equal(t, payload, message)
		return true
	}}
	observeUpstreamMessage(state, payload, start, func() time.Time { return start.Add(25 * time.Millisecond) }, nil)
	require.Equal(t, 1, calls)
	require.NotNil(t, state.firstTokenMs)
	require.Equal(t, 25, *state.firstTokenMs)
}

func TestPayloadTTFTClassifierCannotSuppressLocalFirstEventOrCountTerminal(t *testing.T) {
	calls := 0
	state := &relayState{isFirstTokenEvent: func(string, []byte) bool {
		calls++
		return false
	}}
	require.True(t, state.startsFirstToken("codex.response.metadata", nil))
	require.Zero(t, calls)
	state.isFirstTokenEvent = func(string, []byte) bool { return true }
	require.False(t, state.startsFirstToken("response.completed", nil))
}

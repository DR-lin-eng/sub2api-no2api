package openai_ws_v2

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestOpenAITimingRelayTurnIsolation(t *testing.T) {
	state := &relayState{}
	start := time.Now()
	now := start
	observe := func(payload string) observedUpstreamEvent {
		now = now.Add(10 * time.Millisecond)
		return observeUpstreamMessage(state, []byte(payload), start, func() time.Time { return now }, nil)
	}
	observe(`{"type":"response.created","response":{"id":"resp_first"}}`)
	observe(`{"type":"response.output_text.delta","delta":"hi"}`)
	telemetry := `{"type":"responsesapi.websocket_timing","timing_metrics":{"response_id":"resp_first","engine_service_ttft_total_ms":690.87897,"total_turn_time_s":1.047620254}}`
	observe(telemetry)
	first := observe(`{"type":"response.completed","response":{"id":"resp_first"}}`)
	require.Equal(t, 691, *first.openAITiming.FirstTokenMs())
	require.NotNil(t, first.firstToken)
	require.NotEqual(t, 691, *first.firstToken)
	emitTurnComplete(func(turn RelayTurnResult) {
		require.Equal(t, 1048, *turn.OpenAITiming.DurationMs())
	}, state, first)
	observe(`{"type":"response.created","response":{"id":"resp_second"}}`)
	observe(telemetry)
	require.Len(t, state.turnTimingByID, 1)
	second := observe(`{"type":"response.completed","response":{"id":"resp_second"}}`)
	require.Nil(t, second.openAITiming)
	require.Nil(t, state.lastOpenAITiming)
}

package openaitiming

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

const sample = `{"type":"responsesapi.websocket_timing","timing_metrics":{"response_id":"resp_test","timing_scope":"logical_turn","first_sampled_message_ttft_ms":470.0,"engine_service_ttft_total_ms":690.87897,"engine_queue_max_ms":74.0,"total_turn_time_s":1.047620254,"num_engine_calls":1,"responsesapi_duration_excl_client_tools_ms":1413.618964}}`

func TestCollectorSample(t *testing.T) {
	c := &Collector{}
	c.Observe([]byte(`{"response":{"id":"resp_test"}}`), "response.created")
	c.Observe([]byte(sample), EventType)
	c.Observe([]byte(`{"response":{"id":"resp_test"}}`), "response.completed")
	m := c.Snapshot()
	require.NotNil(t, m)
	require.Equal(t, 691, *m.FirstTokenMs())
	require.Equal(t, 1048, *m.DurationMs())
	require.Equal(t, 470.0, *m.FirstSampledMessageTTFTMs)
	require.Equal(t, 74.0, *m.EngineQueueMaxMs)
	require.Equal(t, 690.87897, *m.EngineServiceTTFTTotalMs)
	require.Equal(t, 1413.618964, *m.ResponsesAPIDurationMs)
}

func TestCollectorInvalidValuesAndPartialMetrics(t *testing.T) {
	for _, invalid := range []string{`null`, `"690"`, `true`, `-1`, `1e100`, `{}`, `[]`} {
		t.Run(invalid, func(t *testing.T) {
			c := &Collector{}
			c.Observe([]byte(fmt.Sprintf(`{"timing_metrics":{"engine_service_ttft_total_ms":%s,"total_turn_time_s":1.048}}`, invalid)), EventType)
			require.Nil(t, c.Snapshot().FirstTokenMs())
			require.Equal(t, 1048, *c.Snapshot().DurationMs())
		})
	}
	for _, invalid := range []string{`{`, `{"timing_metrics":{}}`, `{"timing_metrics":null}`, `{"timing_metrics":{"total_turn_time_s":-1}}`, `{"timing_metrics":{"total_turn_time_s":1,"timing_scope":"unknown"}}`} {
		c := &Collector{}
		c.Observe([]byte(invalid), EventType)
		require.Nil(t, c.Snapshot(), invalid)
	}
	c := &Collector{}
	c.Observe([]byte(`{"timing_metrics":{"engine_service_ttft_total_ms":0,"total_turn_time_s":0}}`), EventType)
	require.Zero(t, *c.Snapshot().FirstTokenMs())
	require.Zero(t, *c.Snapshot().DurationMs())
}

func TestCollectorResponseIsolationAndLateTelemetry(t *testing.T) {
	c := &Collector{}
	c.Observe([]byte(`{"response":{"id":"resp_other"}}`), "response.created")
	c.Observe([]byte(sample), EventType)
	require.Nil(t, c.Snapshot())
	c = &Collector{}
	c.Observe([]byte(sample), EventType)
	c.Observe([]byte(`{"response":{"id":"resp_other"}}`), "response.completed")
	require.Nil(t, c.Snapshot())
	c = &Collector{}
	c.Observe([]byte(`{"response":{"id":"resp_test"}}`), "response.completed")
	c.Observe([]byte(sample), EventType)
	require.Nil(t, c.Snapshot())
}

func TestMultiEngineTTFTIsNotTurnTTFT(t *testing.T) {
	c := &Collector{}
	c.Observe([]byte(`{"timing_metrics":{"engine_service_ttft_total_ms":900,"num_engine_calls":2,"total_turn_time_s":2}}`), EventType)
	require.Nil(t, c.Snapshot().FirstTokenMs())
	require.Equal(t, 2000, *c.Snapshot().DurationMs())
}

func BenchmarkCollectorDelta(b *testing.B) {
	c := &Collector{}
	payload := []byte(`{"type":"response.output_text.delta","delta":"hello"}`)
	b.ReportAllocs()
	for b.Loop() {
		c.Observe(payload, "response.output_text.delta")
	}
}

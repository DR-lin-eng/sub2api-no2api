package dto

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestUsageLogFromServiceOpenAITiming(t *testing.T) {
	var log service.UsageLog
	require.NoError(t, json.Unmarshal([]byte(`{"FirstTokenMs":1250,"DurationMs":1800,"OpenAITiming":{"first_sampled_message_ttft_ms":470,"engine_service_ttft_total_ms":690.87897,"engine_queue_max_ms":74,"total_turn_time_s":1.047620254,"num_engine_calls":1}}`), &log))
	user := UsageLogFromService(&log)
	require.NotNil(t, user.FirstTokenMs)
	require.Equal(t, 691, *user.FirstTokenMs)
	require.Equal(t, 1048, *user.DurationMs)
	encoded, err := json.Marshal(user)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "local_first_token_ms")
	require.NotContains(t, string(encoded), "first_sampled_message_ttft_ms")
	require.NotContains(t, string(encoded), "first_token_source")
	admin, err := json.Marshal(UsageLogFromServiceAdmin(&log))
	require.NoError(t, err)
	require.Contains(t, string(admin), `"first_token_source":"openai"`)
	require.Contains(t, string(admin), `"local_first_token_ms":1250`)
	require.Contains(t, string(admin), `"local_duration_ms":1800`)
	require.Contains(t, string(admin), `"engine_service_ttft_total_ms":690.87897`)
	t.Log("user_ttft_ms=691 user_duration_ms=1048 local_ttft_ms=1250 local_duration_ms=1800 openai_engine_ttft_ms=690.87897")
}

func TestOpenAITimingDisplayFallback(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		first    int
		duration int
	}{
		{"legacy", `{}`, 1250, 1800},
		{"partial", `{"OpenAITiming":{"total_turn_time_s":1.048}}`, 1250, 1048},
		{"invalid", `{"OpenAITiming":{"engine_service_ttft_total_ms":-1,"total_turn_time_s":-1}}`, 1250, 1800},
		{"multi-engine", `{"OpenAITiming":{"engine_service_ttft_total_ms":1000,"num_engine_calls":2,"total_turn_time_s":2}}`, 1250, 2000},
		{"image", `{"ImageCount":1,"OpenAITiming":{"engine_service_ttft_total_ms":690}}`, 1250, 1800},
		{"video", `{"VideoCount":1,"OpenAITiming":{"engine_service_ttft_total_ms":690}}`, 1250, 1800},
	} {
		t.Run(tc.name, func(t *testing.T) {
			first, duration := 1250, 1800
			log := service.UsageLog{FirstTokenMs: &first, DurationMs: &duration}
			require.NoError(t, json.Unmarshal([]byte(tc.input), &log))
			user := UsageLogFromService(&log)
			require.Equal(t, tc.first, *user.FirstTokenMs)
			require.Equal(t, tc.duration, *user.DurationMs)
			require.Equal(t, 1250, *log.FirstTokenMs)
			require.Equal(t, 1800, *log.DurationMs)
		})
	}
}

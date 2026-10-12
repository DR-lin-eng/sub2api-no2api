package openai_ws_v2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPassthroughUsageIncludesImageInputAndToolFallback(t *testing.T) {
	state := &relayState{}
	first := parseUsageAndAccumulate(state, []byte(`{"response":{"usage":{"input_tokens":100,"output_tokens":200,"input_tokens_details":{"image_tokens":20},"output_tokens_details":{"image_tokens":40}}}}`), "response.completed", nil)
	require.Equal(t, 20, first.ImageInputTokens)
	require.Equal(t, 40, first.ImageOutputTokens)
	second := parseUsageAndAccumulate(state, []byte(`{"response":{"usage":{"input_tokens":100,"output_tokens":200},"tool_usage":{"image_gen":{"input_tokens_details":{"image_tokens":30},"output_tokens_details":{"image_tokens":50}}}}}`), "response.completed", nil)
	require.Equal(t, 30, second.ImageInputTokens)
	require.Equal(t, 50, second.ImageOutputTokens)
	require.Equal(t, 50, state.usage.ImageInputTokens)
	require.Equal(t, 90, state.usage.ImageOutputTokens)
}

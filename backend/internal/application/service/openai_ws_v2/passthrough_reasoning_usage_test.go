package openai_ws_v2

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReasoningUsagePerTurnAndAccumulation(t *testing.T) {
	state := &relayState{}
	first := parseUsageAndAccumulate(state, []byte(`{"response":{"usage":{"input_tokens":10,"output_tokens":30,"output_tokens_details":{"reasoning_tokens":24}}}}`), "response.completed", nil)
	second := parseUsageAndAccumulate(state, []byte(`{"response":{"usage":{"input_tokens":10,"output_tokens":30,"completion_tokens_details":{"reasoning_tokens":0}}}}`), "response.completed", nil)
	missing := parseUsageAndAccumulate(state, []byte(`{"response":{"usage":{"input_tokens":10,"output_tokens":30}}}`), "response.completed", nil)
	count := 24
	zero := 0
	require.Equal(t, &count, first.ReasoningTokens)
	require.Equal(t, &zero, second.ReasoningTokens)
	require.Nil(t, missing.ReasoningTokens)
	require.Equal(t, &count, state.usage.ReasoningTokens)
	require.Equal(t, 90, state.usage.OutputTokens)
}

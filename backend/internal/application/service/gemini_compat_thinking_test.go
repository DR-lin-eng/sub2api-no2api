//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGeminiThinkingConfigMapsCompatibilityEffort(t *testing.T) {
	body := applyGeminiCompatThinkingConfig(
		[]byte(`{"contents":[]}`),
		[]byte(`{"thinking":{"type":"adaptive"},"output_config":{"effort":"high"}}`),
		"gemini-3.1-flash",
		true,
	)
	require.Equal(t, "high", gjson.GetBytes(body, "generationConfig.thinkingConfig.thinkingLevel").String())
	require.True(t, gjson.GetBytes(body, "generationConfig.thinkingConfig.includeThoughts").Bool())

	disabled := applyGeminiCompatThinkingConfig(
		[]byte(`{"contents":[]}`),
		[]byte(`{"thinking":{"type":"disabled"}}`),
		"gemini-2.5-flash",
		true,
	)
	require.Equal(t, int64(0), gjson.GetBytes(disabled, "generationConfig.thinkingConfig.thinkingBudget").Int())
	require.False(t, gjson.GetBytes(disabled, "generationConfig.thinkingConfig.includeThoughts").Bool())
}

func TestGeminiThinkingDropsHistoryAndKeepsDeltaSemantics(t *testing.T) {
	body := dropClaudeHistoryThinkingBlocks([]byte(`{"messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"private"},{"type":"text","text":"visible"}]}]}`))
	require.Equal(t, "visible", gjson.GetBytes(body, "messages.0.content.0.text").String())
	require.False(t, gjson.GetBytes(body, "messages.0.content.0.thinking").Exists())

	delta, seen := computeGeminiTextDelta("hello", "hello world")
	require.Equal(t, " world", delta)
	require.Equal(t, "hello world", seen)
	require.Equal(t, "high", *extractGeminiReasoningEffortFromBody([]byte(`{"generationConfig":{"thinkingConfig":{"thinkingLevel":"high"}}}`)))
}

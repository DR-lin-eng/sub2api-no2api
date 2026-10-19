package antigravity

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeminiCacheOnlyUsageIsNonNegative(t *testing.T) {
	const body = `{"candidates":[{"content":{"parts":[{"text":"ok"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"cachedContentTokenCount":30}}`
	_, usage, err := TransformGeminiToClaude([]byte(body), "gemini-3.1-pro-preview")
	require.NoError(t, err)
	require.Zero(t, usage.InputTokens)
	require.Equal(t, 30, usage.CacheReadInputTokens)
	p := NewStreamingProcessor("gemini-3.1-pro-preview")
	out := p.ProcessLine(`data: {"response":` + body + `}`)
	require.True(t, strings.Contains(string(out), `"message_start"`))
	require.NotContains(t, string(out), `"input_tokens":-`)
	_, usage = p.Finish()
	require.Zero(t, usage.InputTokens)
	require.Equal(t, 30, usage.CacheReadInputTokens)
}

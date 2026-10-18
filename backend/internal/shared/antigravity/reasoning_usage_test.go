package antigravity

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReasoningUsageNonStreamingAndStreaming(t *testing.T) {
	zero, count := 0, 7
	for _, tc := range []struct {
		metadata string
		want     *int
	}{
		{`"thoughtsTokenCount":7,`, &count},
		{`"thoughtsTokenCount":0,`, &zero},
		{"", nil},
	} {
		payload := `{"response":{"usageMetadata":{` + tc.metadata + `"promptTokenCount":10,"candidatesTokenCount":5},"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}}`
		_, usage, err := TransformGeminiToClaude([]byte(payload), "gemini")
		require.NoError(t, err)
		require.Equal(t, tc.want, usage.ReasoningTokens)
		processor := NewStreamingProcessor("gemini")
		processor.ProcessLine("data: " + payload)
		_, streamed := processor.Finish()
		require.Equal(t, tc.want, streamed.ReasoningTokens)
		require.Equal(t, usage.OutputTokens, streamed.OutputTokens)
	}
}

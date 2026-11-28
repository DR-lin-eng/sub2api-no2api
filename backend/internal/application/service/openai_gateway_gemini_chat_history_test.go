package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGeminiStudioChatHistoryKeepsSignedParallelBatches(t *testing.T) {
	body := []byte(`{"messages":[
		{"role":"assistant","tool_calls":[{"type":"function","function":{"name":"lookup"}},{"type":"function","function":{"name":"search"}}]},
		{"role":"tool","content":"ok"},
		{"role":"assistant","tool_calls":[{"type":"function","function":{"name":"search"},"extra_content":{"google":{"thought_signature":"provider-original"}}}]}
	]}`)
	got := prepareGeminiStudioChatHistory(geminiStudioChatEndpoint, body)
	require.True(t, gjson.ValidBytes(got))
	require.Equal(t, geminiDummyThoughtSignature, gjson.GetBytes(got, "messages.0.tool_calls.0.extra_content.google.thought_signature").Str)
	require.False(t, gjson.GetBytes(got, "messages.0.tool_calls.1.extra_content").Exists())
	require.Equal(t, "provider-original", gjson.GetBytes(got, "messages.2.tool_calls.0.extra_content.google.thought_signature").Str)
	require.Equal(t, body, prepareGeminiStudioChatHistory("https://example.invalid/v1/chat/completions", body))
}

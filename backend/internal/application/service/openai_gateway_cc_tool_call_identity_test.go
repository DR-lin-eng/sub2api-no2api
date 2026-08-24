//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestStripEmptyChatToolCallIdentityKeepsFirstIdentity(t *testing.T) {
	payload := []byte(`{"choices":[{"delta":{"tool_calls":[{"id":"call_1","type":"function","function":{"name":"lookup","arguments":""}}]}}]}`)
	rewritten, changed := stripEmptyChatToolCallIdentity(payload)
	require.False(t, changed)
	require.Equal(t, string(payload), string(rewritten))
}

func TestStripEmptyChatToolCallIdentityRemovesOnlyEmptyFollowUpFields(t *testing.T) {
	payload := []byte(`{"choices":[{"delta":{"tool_calls":[{"id":"","type":"function","function":{"name":"","arguments":"{\"q\":"}}]}}]}`)
	rewritten, changed := stripEmptyChatToolCallIdentity(payload)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.id").Exists())
	require.False(t, gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.function.name").Exists())
	require.Equal(t, `{"q":`, gjson.GetBytes(rewritten, "choices.0.delta.tool_calls.0.function.arguments").String())
}

func TestStripEmptyChatToolCallIdentityFromSSELinePreservesPrefix(t *testing.T) {
	line := `data: {"choices":[{"delta":{"tool_calls":[{"id":"","function":{"name":"","arguments":"{}"}}]}}]}`
	got := stripEmptyChatToolCallIdentityFromSSELine(line)
	require.Contains(t, got, "data: ")
	payload, ok := extractOpenAISSEDataLine(got)
	require.True(t, ok)
	require.False(t, gjson.Get(payload, "choices.0.delta.tool_calls.0.id").Exists())
	require.Equal(t, "{}", gjson.Get(payload, "choices.0.delta.tool_calls.0.function.arguments").String())
}

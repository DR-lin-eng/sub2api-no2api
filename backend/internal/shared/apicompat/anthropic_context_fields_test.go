package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAnthropicRequestRetainsContextControlExtensions(t *testing.T) {
	var req AnthropicRequest
	err := json.Unmarshal([]byte(`{
		"model":"claude-sonnet-5",
		"max_tokens":32,
		"context_management":{"edits":[{"type":"clear_tool_uses_20250919"}]},
		"context_hint":{"enabled":true,"target_tokens_saved":1200},
		"messages":[{"role":"user","content":"hello"}]
	}`), &req)
	require.NoError(t, err)
	require.JSONEq(t, `{"edits":[{"type":"clear_tool_uses_20250919"}]}`, string(req.ContextManagement))
	require.JSONEq(t, `{"enabled":true,"target_tokens_saved":1200}`, string(req.ContextHint))
}

func TestAnthropicToResponsesConsumesExtensionsWithoutSendingUnknownGPTFields(t *testing.T) {
	req := &AnthropicRequest{
		Model:             "gpt-5.6-sol",
		MaxTokens:         32,
		ContextManagement: json.RawMessage(`{"edits":[{"type":"clear_tool_uses_20250919"}]}`),
		ContextHint:       json.RawMessage(`{"enabled":true}`),
		Messages:          []AnthropicMessage{{Role: "user", Content: json.RawMessage(`"hello"`)}},
	}
	converted, err := AnthropicToResponses(req)
	require.NoError(t, err)
	wire, err := json.Marshal(converted)
	require.NoError(t, err)
	require.NotContains(t, string(wire), "context_management")
	require.NotContains(t, string(wire), "context_hint")
}

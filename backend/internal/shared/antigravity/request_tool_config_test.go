package antigravity

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

// TestToolConfigAlwaysPresent ensures toolConfig is always emitted, including for
// reasoning models without any tools: upstream rejects requests that omit it.
func TestToolConfigAlwaysPresent(t *testing.T) {
	cases := []struct {
		name  string
		model string
		tools []ClaudeTool
	}{
		{name: "reasoning model without tools", model: "gemini-3.1-pro-high"},
		{name: "reasoning model with tools", model: "gemini-3.1-pro-high", tools: []ClaudeTool{{
			Name:        "web_search",
			Description: "Search the web",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		}}},
		{name: "non-reasoning model without tools", model: "gemini-3.1-pro"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claudeReq := &ClaudeRequest{
				Model:    tc.model,
				Messages: []ClaudeMessage{{Role: "user", Content: json.RawMessage(`"Hello"`)}},
				Tools:    tc.tools,
			}
			body, err := TransformClaudeToGeminiWithOptions(claudeReq, "project-1", tc.model, DefaultTransformOptions())
			require.NoError(t, err)

			var req V1InternalRequest
			require.NoError(t, json.Unmarshal(body, &req))
			require.NotNil(t, req.Request.ToolConfig, "toolConfig must be present")
			require.NotNil(t, req.Request.ToolConfig.FunctionCallingConfig)
			require.Equal(t, "VALIDATED", req.Request.ToolConfig.FunctionCallingConfig.Mode)
		})
	}
}

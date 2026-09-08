package apicompat

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestFlattenResponsesNamespacesAllowedTools(t *testing.T) {
	choiceTool := map[string]any{"type": "function", "namespace": "tools", "name": "lookup"}
	req := map[string]any{
		"tools":       []any{map[string]any{"type": "namespace", "name": "tools", "tools": []any{map[string]any{"type": "function", "name": "lookup"}}}},
		"tool_choice": map[string]any{"type": "allowed_tools", "mode": "required", "tools": []any{choiceTool}},
	}
	_, changed, err := FlattenResponsesNamespaces(req)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "tools__lookup", choiceTool["name"])
	require.NotContains(t, choiceTool, "namespace")
	require.Equal(t, "required", req["tool_choice"].(map[string]any)["mode"])
}

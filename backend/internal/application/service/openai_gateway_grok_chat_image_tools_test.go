//go:build unit

package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestStripRedundantGrokViewImageTool(t *testing.T) {
	body := []byte(`{"input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AA=="}]}],"tools":[{"type":"function","name":"view_image"},{"type":"function","name":"shell_command"}],"tool_choice":"auto","parallel_tool_calls":true}`)
	patched, err := stripRedundantGrokViewImageTool(body)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(patched, `tools.#(name=="view_image")`).Exists())
	require.True(t, gjson.GetBytes(patched, `tools.#(name=="shell_command")`).Exists())

	unchanged := []byte(`{"input":[{"type":"message","role":"user","content":[{"type":"input_image","image_url":"data:image/png;base64,AA=="}]}],"tools":[{"type":"function","name":"view_image"}],"tool_choice":{"type":"function","name":"view_image"}}`)
	got, err := stripRedundantGrokViewImageTool(unchanged)
	require.NoError(t, err)
	require.Equal(t, unchanged, got)
}

func TestStripRedundantGrokChatViewImageTool(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":[{"type":"text","text":"inspect"},{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}]}],"tools":[{"type":"function","function":{"name":"view_image"}}],"tool_choice":"auto","parallel_tool_calls":true}`)
	patched, err := stripRedundantGrokChatViewImageTool(body)
	require.NoError(t, err)
	require.False(t, gjson.GetBytes(patched, "tools").Exists())
	require.False(t, gjson.GetBytes(patched, "tool_choice").Exists())
	require.False(t, gjson.GetBytes(patched, "parallel_tool_calls").Exists())
}

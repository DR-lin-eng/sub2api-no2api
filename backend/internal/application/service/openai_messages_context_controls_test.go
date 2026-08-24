package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestApplyOpenAIAnthropicContextHintClearsOldToolResultsAndKeepsRecent(t *testing.T) {
	messages := make([]apicompat.AnthropicMessage, 0, 7)
	for i := 0; i < 7; i++ {
		content, err := json.Marshal([]map[string]any{{
			"type":        "tool_result",
			"tool_use_id": "toolu_" + string(rune('a'+i)),
			"content":     "large tool result " + string(rune('a'+i)),
		}})
		require.NoError(t, err)
		messages = append(messages, apicompat.AnthropicMessage{Role: "user", Content: content})
	}
	req := &apicompat.AnthropicRequest{
		Messages:    messages,
		ContextHint: json.RawMessage(`{"enabled":true}`),
	}

	stats := applyOpenAIAnthropicContextControls(req)
	require.Equal(t, 2, stats.ToolResultsCleared)
	require.Contains(t, string(req.Messages[0].Content), contextHintToolResultPlaceholder)
	require.Contains(t, string(req.Messages[1].Content), contextHintToolResultPlaceholder)
	require.Equal(t, "large tool result f", gjson.GetBytes(req.Messages[5].Content, "0.content").String())
	require.Equal(t, "large tool result g", gjson.GetBytes(req.Messages[6].Content, "0.content").String())
}

func TestApplyOpenAIAnthropicContextManagementClearsThinkingButKeepsLatestAssistantTurn(t *testing.T) {
	oldThinking := json.RawMessage(`[{"type":"thinking","thinking":"old"},{"type":"text","text":"old answer"}]`)
	latestThinking := json.RawMessage(`[{"type":"thinking","thinking":"latest"},{"type":"text","text":"latest answer"}]`)
	req := &apicompat.AnthropicRequest{
		Messages: []apicompat.AnthropicMessage{
			{Role: "assistant", Content: oldThinking},
			{Role: "assistant", Content: latestThinking},
		},
		ContextManagement: json.RawMessage(`{"edits":[{"type":"clear_thinking_20251015","keep":{"type":"thinking_turns","value":1}}]}`),
	}

	stats := applyOpenAIAnthropicContextControls(req)
	require.Equal(t, 1, stats.ThinkingBlocksCleared)
	require.NotContains(t, string(req.Messages[0].Content), `"type":"thinking"`)
	require.Contains(t, string(req.Messages[0].Content), `"text":"old answer"`)
	require.Contains(t, string(req.Messages[1].Content), `"type":"thinking"`)
}

func TestApplyOpenAIAnthropicContextControlsLeavesKeepAllAndNoHintUnchanged(t *testing.T) {
	content := json.RawMessage(`[{"type":"tool_result","tool_use_id":"toolu_1","content":"keep"}]`)
	req := &apicompat.AnthropicRequest{
		Messages:          []apicompat.AnthropicMessage{{Role: "user", Content: content}},
		ContextManagement: json.RawMessage(`{"edits":[{"type":"clear_thinking_20251015","keep":"all"}]}`),
	}
	original := string(req.Messages[0].Content)
	stats := applyOpenAIAnthropicContextControls(req)
	require.False(t, stats.changed())
	require.Equal(t, original, string(req.Messages[0].Content))
}

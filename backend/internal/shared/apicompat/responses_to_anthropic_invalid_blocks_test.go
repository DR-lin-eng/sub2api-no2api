package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var anthropicInboundBlockTypes = map[string]bool{
	"text":              true,
	"image":             true,
	"document":          true,
	"tool_use":          true,
	"tool_result":       true,
	"thinking":          true,
	"redacted_thinking": true,
}

func responsesToAnthropicMessages(t *testing.T, input string) []AnthropicMessage {
	t.Helper()
	var req ResponsesRequest
	require.NoError(t, json.Unmarshal([]byte(`{"model":"glm-5.2","input":`+input+`}`), &req))
	out, err := ResponsesToAnthropicRequest(&req)
	require.NoError(t, err)
	return out.Messages
}

func requireAnthropicMessagesAreSendable(t *testing.T, messages []AnthropicMessage) {
	t.Helper()
	for i, message := range messages {
		raw := strings.TrimSpace(string(message.Content))
		require.NotContains(t, []string{"", "null", `""`, "[]"}, raw, "messages[%d] has empty content", i)

		var text string
		if err := json.Unmarshal(message.Content, &text); err == nil {
			require.NotEmpty(t, strings.TrimSpace(text), "messages[%d] has blank string content", i)
			continue
		}
		blocks := parseContentBlocks(message.Content)
		require.NotEmpty(t, blocks, "messages[%d] has no parseable content blocks", i)
		for j, block := range blocks {
			require.True(t, anthropicInboundBlockTypes[block.Type],
				"messages[%d].content[%d] has unsupported block type %q", i, j, block.Type)
			if block.Type == "text" {
				require.NotEmpty(t, strings.TrimSpace(block.Text),
					"messages[%d].content[%d] has blank text", i, j)
			}
		}
	}
}

func TestResponsesToAnthropicReasoningItemWithContentIsDropped(t *testing.T) {
	messages := responsesToAnthropicMessages(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"run a shell command"}]},
		{"type":"reasoning","id":"rs_1","summary":[],"content":[{"type":"reasoning_text","text":"let me think"}]}
	]`)

	requireAnthropicMessagesAreSendable(t, messages)
	require.Len(t, messages, 1)
	require.NotContains(t, string(messages[0].Content), "reasoning_text")
	require.NotContains(t, string(messages[0].Content), "let me think")
}

func TestResponsesToAnthropicReasoningItemSummaryOnlyStillDropped(t *testing.T) {
	messages := responsesToAnthropicMessages(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},
		{"type":"reasoning","id":"rs_1","summary":[{"type":"summary_text","text":"s"}],"encrypted_content":"gAAAA"}
	]`)

	requireAnthropicMessagesAreSendable(t, messages)
	require.Len(t, messages, 1)
	require.NotContains(t, string(messages[0].Content), "gAAAA")
}

func TestResponsesToAnthropicUnknownItemTypeContentIsSanitized(t *testing.T) {
	messages := responsesToAnthropicMessages(t, `[
		{"type":"web_search_call","id":"ws_1","content":[{"type":"web_search_result","text":"payload"}]}
	]`)

	requireAnthropicMessagesAreSendable(t, messages)
	require.Empty(t, messages)
}

func TestResponsesToAnthropicUnknownItemTypeKeepsRecognizableText(t *testing.T) {
	messages := responsesToAnthropicMessages(t, `[
		{"type":"some_future_item","content":[
			{"type":"input_text","text":"keep me"},
			{"type":"reasoning_text","text":"drop me"}
		]}
	]`)

	requireAnthropicMessagesAreSendable(t, messages)
	require.Len(t, messages, 1)
	require.Contains(t, string(messages[0].Content), "keep me")
	require.NotContains(t, string(messages[0].Content), "drop me")
}

func TestResponsesToAnthropicUserMessageWithOnlyUnknownPartsIsDropped(t *testing.T) {
	messages := responsesToAnthropicMessages(t, `[
		{"type":"message","role":"user","content":[{"type":"input_file","file_id":"file_1"}]}
	]`)

	requireAnthropicMessagesAreSendable(t, messages)
	require.Empty(t, messages)
}

func TestResponsesToAnthropicAssistantMessageWithOnlyUnknownPartsIsDropped(t *testing.T) {
	messages := responsesToAnthropicMessages(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]},
		{"type":"message","role":"assistant","content":[{"type":"refusal","refusal":"no"}]}
	]`)

	requireAnthropicMessagesAreSendable(t, messages)
	require.Len(t, messages, 1)
	require.Equal(t, "user", messages[0].Role)
}

func TestResponsesToAnthropicCodexToolRoundStaysIntactAndSendable(t *testing.T) {
	messages := responsesToAnthropicMessages(t, `[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"run ls"}]},
		{"type":"reasoning","id":"rs_1","summary":[],"content":[{"type":"reasoning_text","text":"plan"}],"encrypted_content":"gAAAA"},
		{"type":"function_call","id":"fc_1","call_id":"call_1","name":"shell","arguments":"{\"cmd\":\"ls\"}"},
		{"type":"function_call_output","call_id":"call_1","output":"file1"},
		{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"}]}
	]`)

	requireAnthropicMessagesAreSendable(t, messages)
	var sawToolUse, sawToolResult bool
	for _, message := range messages {
		for _, block := range parseContentBlocks(message.Content) {
			switch block.Type {
			case "tool_use":
				sawToolUse = true
				require.Equal(t, "call_1", block.ID)
				require.Equal(t, "shell", block.Name)
			case "tool_result":
				sawToolResult = true
				require.Equal(t, "call_1", block.ToolUseID)
			}
		}
	}
	require.True(t, sawToolUse)
	require.True(t, sawToolResult)
	encoded, err := json.Marshal(messages)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "reasoning_text")
	require.NotContains(t, string(encoded), "gAAAA")
}

func TestAnthropicContentIsEmpty(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{``, true},
		{`""`, true},
		{`null`, true},
		{`[]`, true},
		{`  []  `, true},
		{`"hi"`, false},
		{`[{"type":"text","text":"hi"}]`, false},
	} {
		require.Equal(t, tc.want, anthropicContentIsEmpty(json.RawMessage(tc.raw)), "raw=%q", tc.raw)
	}
}

func TestAnthropicContentIsOnlyBlankText(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want bool
	}{
		{`[{"type":"text","text":""}]`, true},
		{`[{"type":"text","text":"   "}]`, true},
		{`[{"type":"text","text":""},{"type":"text","text":" "}]`, true},
		{`[{"type":"text","text":"hi"}]`, false},
		{`[{"type":"text","text":""},{"type":"image","source":{}}]`, false},
		{`[]`, false},
	} {
		require.Equal(t, tc.want, anthropicContentIsOnlyBlankText(json.RawMessage(tc.raw)), "raw=%q", tc.raw)
	}
}

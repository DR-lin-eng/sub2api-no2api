package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestIsCompatCompactionRequest(t *testing.T) {
	compact := []byte(`{"model":"m","input":[{"type":"message","role":"user","content":[]},{"type":"compaction_trigger"}]}`)
	require.True(t, isCompatCompactionRequest(compact))

	normal := []byte(`{"model":"m","input":[{"type":"message","role":"user","content":[]}]}`)
	require.False(t, isCompatCompactionRequest(normal))
}

func TestRewriteCompatCompactRequestBody_TriggerBecomesInstruction(t *testing.T) {
	body := []byte(`{"model":"m","stream":true,"tools":[{"type":"function","name":"exec"}],"input":[
		{"type":"message","role":"user","content":[{"type":"input_text","text":"hello"}]},
		{"type":"compaction_trigger"}
	]}`)

	out, err := rewriteCompatCompactRequestBody(body)
	require.NoError(t, err)

	input := gjson.GetBytes(out, "input").Array()
	require.Len(t, input, 2)
	require.Equal(t, "hello", input[0].Get("content.0.text").String())
	require.Equal(t, "message", input[1].Get("type").String())
	require.Equal(t, grokCompactSummaryPrompt, input[1].Get("content.0.text").String())
	require.False(t, gjson.GetBytes(out, "stream").Bool())
	require.Equal(t, "none", gjson.GetBytes(out, "tool_choice").String())
}

func TestRewriteCompatCompactRequestBody_ReplaysEncodedSummaryWithoutNewTrigger(t *testing.T) {
	encrypted := encodeCompatCompactSummary("earlier work")
	body, err := json.Marshal(map[string]any{
		"model":  "m",
		"stream": true,
		"input": []any{
			map[string]any{"type": "compaction", "encrypted_content": encrypted},
			map[string]any{"type": "message", "role": "user", "content": []any{map[string]any{"type": "input_text", "text": "next"}}},
		},
	})
	require.NoError(t, err)

	out, err := rewriteCompatCompactRequestBody(body)
	require.NoError(t, err)

	input := gjson.GetBytes(out, "input").Array()
	require.Len(t, input, 2)
	require.Contains(t, input[0].Get("content.0.text").String(), "<"+compatCompactSummaryTag+">\nearlier work\n")
	require.True(t, gjson.GetBytes(out, "stream").Bool(), "ordinary turns after compaction must keep streaming")
	require.False(t, gjson.GetBytes(out, "tool_choice").Exists())
}

func TestRewriteCompatCompactRequestBody_ReplaysLegacyVisibleSummary(t *testing.T) {
	body := []byte(`{"model":"m","input":[
		{"type":"compaction","summary":[{"type":"summary_text","text":"legacy summary"}]},
		{"type":"compaction_trigger"}
	]}`)

	out, err := rewriteCompatCompactRequestBody(body)
	require.NoError(t, err)
	require.Contains(t, gjson.GetBytes(out, "input.0.content.0.text").String(), "legacy summary")
}

func TestRewriteCompatCompactRequestBody_RejectsUnknownOpaqueSummary(t *testing.T) {
	body := []byte(`{"model":"m","input":[{"type":"compaction","encrypted_content":"foreign-opaque-value"}]}`)
	_, err := rewriteCompatCompactRequestBody(body)
	require.ErrorContains(t, err, "another upstream")
}

func TestRewriteCompatCompactRequestBody_NoCompactionItemsIsUnchanged(t *testing.T) {
	body := []byte(`{"model":"m","stream":true,"input":[{"type":"message","role":"user","content":[]}]}`)
	out, err := rewriteCompatCompactRequestBody(body)
	require.NoError(t, err)
	require.Equal(t, body, out)
}

func TestBuildCompatCompactResponse_SingleReplayableCompactionItem(t *testing.T) {
	content, err := json.Marshal("summary text")
	require.NoError(t, err)
	resp := &apicompat.ChatCompletionsResponse{
		ID:      "chatcmpl-1",
		Created: 123,
		Choices: []apicompat.ChatChoice{{
			Message:      apicompat.ChatMessage{Role: "assistant", Content: content},
			FinishReason: "stop",
		}},
		Usage: &apicompat.ChatUsage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14},
	}

	out, err := buildCompatCompactResponse(resp, "gpt-5.6-sol")
	require.NoError(t, err)
	require.Equal(t, int64(123), out.CreatedAt)
	require.Equal(t, "gpt-5.6-sol", out.Model)
	require.Len(t, out.Output, 1)
	require.Equal(t, "compaction", out.Output[0].Type)
	require.True(t, strings.HasPrefix(out.Output[0].ID, "cmp_"))
	require.NotEmpty(t, out.Output[0].EncryptedContent)
	replayed, recognized, err := decodeCompatCompactSummary(out.Output[0].EncryptedContent)
	require.NoError(t, err)
	require.True(t, recognized)
	require.Equal(t, "summary text", replayed)
	require.Equal(t, "summary text", out.Output[0].Summary[0].Text)
	require.NotNil(t, out.Usage)
	require.Equal(t, 10, out.Usage.InputTokens)
}

func TestBuildCompatCompactResponse_FallsBackToReasoningAndRequiredMetadata(t *testing.T) {
	resp := &apicompat.ChatCompletionsResponse{
		Model: "mapped-model",
		Choices: []apicompat.ChatChoice{{
			Message: apicompat.ChatMessage{Role: "assistant", ReasoningContent: "thought summary"},
		}},
	}
	out, err := buildCompatCompactResponse(resp, "")
	require.NoError(t, err)
	require.Equal(t, "mapped-model", out.Model)
	require.Positive(t, out.CreatedAt)
	require.True(t, strings.HasPrefix(out.ID, "resp_"))
	require.Equal(t, "thought summary", out.Output[0].Summary[0].Text)
}

func TestBuildCompatCompactResponse_RejectsIncompleteOrEmptySummary(t *testing.T) {
	content, err := json.Marshal("partial")
	require.NoError(t, err)
	_, err = buildCompatCompactResponse(&apicompat.ChatCompletionsResponse{
		Choices: []apicompat.ChatChoice{{
			Message:      apicompat.ChatMessage{Content: content},
			FinishReason: "length",
		}},
	}, "m")
	require.ErrorContains(t, err, "did not finish")

	_, err = buildCompatCompactResponse(&apicompat.ChatCompletionsResponse{
		Choices: []apicompat.ChatChoice{{Message: apicompat.ChatMessage{Role: "assistant"}}},
	}, "m")
	require.ErrorContains(t, err, "no summary text")
}

func TestBuildCompatCompactResponse_FeedsCompactSSEBridge(t *testing.T) {
	content, err := json.Marshal("summary text")
	require.NoError(t, err)
	resp := &apicompat.ChatCompletionsResponse{
		ID:      "chatcmpl-1",
		Choices: []apicompat.ChatChoice{{Message: apicompat.ChatMessage{Role: "assistant", Content: content}}},
		Usage:   &apicompat.ChatUsage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14},
	}
	compactResp, err := buildCompatCompactResponse(resp, "m")
	require.NoError(t, err)
	encoded, err := json.Marshal(compactResp)
	require.NoError(t, err)

	payload, ok := buildOpenAICompactSSEPayload(encoded)
	require.True(t, ok)
	text := string(payload)
	require.Equal(t, 1, strings.Count(text, "event: response.output_item.done"))
	require.Contains(t, text, `"type":"compaction"`)
	require.Contains(t, text, `"encrypted_content":"`+compatCompactEnvelopePrefix)
	require.Contains(t, text, "event: response.completed")
}

func TestChatMessagePlainText(t *testing.T) {
	stringContent, err := json.Marshal("plain")
	require.NoError(t, err)
	require.Equal(t, "plain", chatMessagePlainText(stringContent))

	partsContent := json.RawMessage(`[{"type":"text","text":"a"},{"type":"text","text":"b"}]`)
	require.Equal(t, "a\nb", chatMessagePlainText(partsContent))
	require.Equal(t, "", chatMessagePlainText(nil))
}

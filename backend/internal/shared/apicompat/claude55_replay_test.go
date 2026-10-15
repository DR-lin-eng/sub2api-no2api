package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClaude55BufferedSignedReplayOrder(t *testing.T) {
	for _, model := range []string{"claude-opus-5-5", "claude-sonnet-5-5", "claude-haiku-5-5"} {
		t.Run(model, func(t *testing.T) {
			content := []AnthropicContentBlock{{Type: "thinking", Thinking: "check", Signature: "provider-signature"}, {Type: "text", Text: "before"}, {Type: "tool_use", ID: "toolu_1", Name: "read", Input: json.RawMessage(`{"path":"x"}`)}, {Type: "redacted_thinking", Data: "opaque-redaction"}, {Type: "text", Text: "after"}}
			response := AnthropicToResponsesResponse(&AnthropicResponse{ID: "msg_signed", Model: model, Content: content})
			require.Len(t, response.Output, 5)
			require.Equal(t, []string{"reasoning", "message", "function_call", "reasoning", "message"}, []string{response.Output[0].Type, response.Output[1].Type, response.Output[2].Type, response.Output[3].Type, response.Output[4].Type})
			for i, j := range map[int]int{0: 0, 3: 3} {
				raw, _ := json.Marshal([]ResponsesOutput{response.Output[i]})
				request, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: model, Input: raw, Reasoning: &ResponsesReasoning{Effort: "high"}})
				require.NoError(t, err)
				require.Equal(t, "adaptive", request.Thinking.Type)
				var replay []AnthropicContentBlock
				require.NoError(t, json.Unmarshal(request.Messages[0].Content, &replay))
				require.Equal(t, content[j], replay[0])
			}
		})
	}
	old := AnthropicToResponsesResponse(&AnthropicResponse{Model: "claude-sonnet-4-6", Content: []AnthropicContentBlock{{Type: "thinking", Thinking: "old", Signature: "opaque"}}})
	require.Empty(t, old.Output[0].EncryptedContent)
}

func TestClaude55StreamSignedReplayAndTerminalConsistency(t *testing.T) {
	state := NewAnthropicEventToResponsesState()
	feed := func(event *AnthropicStreamEvent) []ResponsesStreamEvent {
		return AnthropicEventToResponsesEvents(event, state)
	}
	feed(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{Model: "claude-sonnet-5-5", ID: "msg_1"}})
	blocks := []AnthropicContentBlock{{Type: "thinking"}, {Type: "text"}, {Type: "redacted_thinking", Data: "redacted"}, {Type: "thinking"}}
	var done []ResponsesOutput
	for _, block := range blocks {
		feed(&AnthropicStreamEvent{Type: "content_block_start", ContentBlock: &block})
		switch block.Type {
		case "thinking":
			for _, text := range []string{"think", " carefully"} {
				feed(&AnthropicStreamEvent{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "thinking_delta", Thinking: text}})
			}
			for _, sig := range []string{"signed-", "fragment"} {
				feed(&AnthropicStreamEvent{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "signature_delta", Signature: sig}})
			}
		case "text":
			feed(&AnthropicStreamEvent{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "text_delta", Text: "answer"}})
		}
		events := feed(&AnthropicStreamEvent{Type: "content_block_stop"})
		for _, event := range events {
			if event.Type == "response.reasoning_summary_text.done" && block.Type == "thinking" {
				require.Equal(t, "think carefully", event.Text)
			}
			if event.Type == "response.output_item.done" {
				done = append(done, *event.Item)
			}
		}
	}
	terminal := feed(&AnthropicStreamEvent{Type: "message_stop"})
	require.NotEmpty(t, terminal)
	response := terminal[len(terminal)-1].Response
	require.Equal(t, "completed", response.Status)
	require.Len(t, response.Output, 4)
	for _, item := range response.Output {
		if item.Type != "reasoning" {
			continue
		}
		block, marked, err := decodeAnthropicThinking(item.EncryptedContent)
		require.NoError(t, err)
		require.True(t, marked)
		if block.Type == "thinking" {
			require.Equal(t, "signed-fragment", block.Signature)
			require.Equal(t, "think carefully", block.Thinking)
		} else {
			require.Equal(t, "redacted", block.Data)
		}
	}
	require.Equal(t, response.Output[0], done[0])
	require.Empty(t, FinalizeAnthropicResponsesStream(state))
}

func TestClaude55RejectsMalformedReplayAndForeignCiphertext(t *testing.T) {
	for _, value := range []string{anthropicThinkingEnvelopePrefix + "bad!", anthropicThinkingEnvelopePrefix + "e30", anthropicThinkingEnvelopePrefix + strings.Repeat("x", 12<<20)} {
		input, _ := json.Marshal([]map[string]any{{"type": "reasoning", "encrypted_content": value}})
		_, err := ResponsesToAnthropicRequest(&ResponsesRequest{Model: "claude-opus-5-5", Input: input})
		require.Error(t, err)
	}
	_, marked, err := decodeAnthropicThinking("openai-encrypted-ciphertext")
	require.NoError(t, err)
	require.False(t, marked)
	encoded := encodeAnthropicThinking(AnthropicContentBlock{Type: "thinking", Signature: "skip_thought_signature_validator"})
	_, _, err = decodeAnthropicThinking(encoded)
	require.Error(t, err)
	response := AnthropicToResponsesResponse(&AnthropicResponse{Model: "claude-opus-5-5", Content: []AnthropicContentBlock{{Type: "thinking", Thinking: strings.Repeat("x", 8<<20), Signature: "sig"}}})
	require.Equal(t, "failed", response.Status)
	require.Equal(t, "thinking_replay_limit", response.Error.Code)
}

func BenchmarkClaude55FragmentedThinking(b *testing.B) {
	fragment := strings.Repeat("x", 1024)
	b.ReportAllocs()
	b.SetBytes(1 << 20)
	for i := 0; i < b.N; i++ {
		state := NewAnthropicEventToResponsesState()
		AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "message_start", Message: &AnthropicResponse{Model: "claude-opus-5-5"}}, state)
		AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_start", ContentBlock: &AnthropicContentBlock{Type: "thinking"}}, state)
		for j := 0; j < 1024; j++ {
			AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "thinking_delta", Thinking: fragment}}, state)
		}
		AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_delta", Delta: &AnthropicDelta{Type: "signature_delta", Signature: "sig"}}, state)
		AnthropicEventToResponsesEvents(&AnthropicStreamEvent{Type: "content_block_stop"}, state)
	}
}

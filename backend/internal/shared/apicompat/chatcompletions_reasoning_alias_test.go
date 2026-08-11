package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChatReasoningAliasNonStreamingBridges(t *testing.T) {
	var response ChatCompletionsResponse
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"chatcmpl-alias","model":"reasoning-model",
		"choices":[{"index":0,"message":{"role":"assistant","reasoning":"fallback reasoning","content":"final answer"},"finish_reason":"stop"}]
	}`), &response))

	anthropic := ChatCompletionsResponseToAnthropic(&response, "claude-sonnet-4")
	require.Len(t, anthropic.Content, 2)
	require.Equal(t, "thinking", anthropic.Content[0].Type)
	require.Equal(t, "fallback reasoning", anthropic.Content[0].Thinking)

	responses := ChatCompletionsResponseToResponses(&response, "reasoning-model", nil, false, nil)
	require.Len(t, responses.Output, 2)
	require.Equal(t, "reasoning", responses.Output[0].Type)
	require.Equal(t, "fallback reasoning", responses.Output[0].Summary[0].Text)
}

func TestChatReasoningAliasStreamingBridges(t *testing.T) {
	var chunk ChatCompletionsChunk
	require.NoError(t, json.Unmarshal([]byte(`{
		"id":"chatcmpl-alias","model":"reasoning-model",
		"choices":[{"index":0,"delta":{"reasoning":"streamed fallback"},"finish_reason":null}]
	}`), &chunk))

	anthropicEvents := ChatCompletionsChunkToAnthropicEvents(&chunk, NewChatCompletionsToAnthropicStreamState("reasoning-model"))
	var thinking string
	for _, event := range anthropicEvents {
		if event.Delta != nil && event.Delta.Type == "thinking_delta" {
			thinking += event.Delta.Thinking
		}
	}
	require.Equal(t, "streamed fallback", thinking)

	responsesEvents := ChatCompletionsChunkToResponsesEvents(&chunk, NewChatCompletionsToResponsesStreamState("reasoning-model"))
	var deltas []string
	for _, event := range responsesEvents {
		if event.Type == "response.reasoning_summary_text.delta" {
			deltas = append(deltas, event.Delta)
		}
	}
	require.Equal(t, []string{"streamed fallback"}, deltas)
}

func TestChatReasoningContentTakesPrecedenceOverAlias(t *testing.T) {
	preferred := "preferred reasoning"
	fallback := "fallback reasoning"
	chunk := ChatCompletionsChunk{Choices: []ChatChunkChoice{{
		Delta: ChatDelta{ReasoningContent: &preferred, Reasoning: &fallback},
	}}}

	events := ChatCompletionsChunkToResponsesEvents(&chunk, NewChatCompletionsToResponsesStreamState("reasoning-model"))
	var deltas []string
	for _, event := range events {
		if event.Type == "response.reasoning_summary_text.delta" {
			deltas = append(deltas, event.Delta)
		}
	}
	require.Equal(t, []string{preferred}, deltas)
}

package apicompat

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponsesInputToChatMessages_DeveloperRoleMapsToSystem(t *testing.T) {
	messages, err := responsesInputToChatMessages("", json.RawMessage(`[{"role":"developer","content":"follow project instructions"}]`))
	require.NoError(t, err)
	require.Len(t, messages, 1)

	assert.Equal(t, "system", messages[0].Role)
	assert.JSONEq(t, `"follow project instructions"`, string(messages[0].Content))
}

func TestResponsesInputToChatMessages_KeepsChatCompletionRoles(t *testing.T) {
	input := json.RawMessage(`[
		{"role":"system","content":"system message"},
		{"role":"user","content":"user message"},
		{"role":"assistant","content":"assistant message"},
		{"role":"tool","content":"tool message"}
	]`)

	messages, err := responsesInputToChatMessages("", input)
	require.NoError(t, err)
	require.Len(t, messages, 4)

	assert.Equal(t, []string{"system", "user", "assistant", "tool"}, chatMessageRoles(messages))
}

func TestResponsesInputToChatMessages_EmptyRoleFallsBackToUser(t *testing.T) {
	messages, err := responsesInputToChatMessages("", json.RawMessage(`[{"role":"","content":"hello"}]`))
	require.NoError(t, err)
	require.Len(t, messages, 1)

	assert.Equal(t, "user", messages[0].Role)
}

func TestResponsesInputToChatMessages_DeveloperRoleTrimAndCaseInsensitive(t *testing.T) {
	input := json.RawMessage(`[
		{"role":" Developer ","content":"one"},
		{"role":"\tDEVELOPER\n","content":"two"}
	]`)

	messages, err := responsesInputToChatMessages("", input)
	require.NoError(t, err)
	require.Len(t, messages, 2)

	assert.Equal(t, []string{"system", "system"}, chatMessageRoles(messages))
}

func TestResponsesToChatCompletionsRequest_InstructionsAndInputDeveloperRole(t *testing.T) {
	req := &ResponsesRequest{
		Model:        "gpt-4o",
		Instructions: "Use concise answers.",
		Input: json.RawMessage(`[
			{"role":"developer","content":[{"type":"input_text","text":"Prefer JSON."}]},
			{"role":"user","content":"Hello"}
		]`),
	}

	out, err := ResponsesToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.Len(t, out.Messages, 3)

	assert.Equal(t, []string{"system", "system", "user"}, chatMessageRoles(out.Messages))
	assert.JSONEq(t, `"Use concise answers."`, string(out.Messages[0].Content))
	assert.JSONEq(t, `"Prefer JSON."`, string(out.Messages[1].Content))
	assert.JSONEq(t, `"Hello"`, string(out.Messages[2].Content))
}

func TestResponsesToChatCompletionsRequest_TextFormatJsonObject(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`[
			{"role":"user","content":"Return JSON"}
		]`),
		Text: &ResponsesText{
			Format: json.RawMessage(`{"type":"json_object"}`),
		},
	}

	out, err := ResponsesToChatCompletionsRequest(req)
	require.NoError(t, err)
	assert.JSONEq(t, `{"type":"json_object"}`, string(out.ResponseFormat))
}

func TestResponsesToChatCompletionsRequest_TextFormatJsonSchema(t *testing.T) {
	req := &ResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`[
			{"role":"user","content":"Return structured JSON"}
		]`),
		Text: &ResponsesText{
			Format: json.RawMessage(`{
				"type":"json_schema",
				"name":"answer",
				"schema":{
					"type":"object",
					"properties":{"ok":{"type":"boolean"}},
					"required":["ok"],
					"additionalProperties":false
				},
				"strict":true
			}`),
		},
	}

	out, err := ResponsesToChatCompletionsRequest(req)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"type":"json_schema",
		"json_schema":{
			"name":"answer",
			"schema":{
				"type":"object",
				"properties":{"ok":{"type":"boolean"}},
				"required":["ok"],
				"additionalProperties":false
			},
			"strict":true
		}
	}`, string(out.ResponseFormat))
}

func TestResponsesToChatCompletionsRequest_ParallelToolCalls(t *testing.T) {
	parallel := false
	req := &ResponsesRequest{
		Model: "gpt-4o",
		Input: json.RawMessage(`[
			{"role":"user","content":"Use tools"}
		]`),
		ParallelToolCalls: &parallel,
	}

	out, err := ResponsesToChatCompletionsRequest(req)
	require.NoError(t, err)
	require.NotNil(t, out.ParallelToolCalls)
	assert.False(t, *out.ParallelToolCalls)

	payload, err := json.Marshal(out)
	require.NoError(t, err)
	assert.Contains(t, string(payload), `"parallel_tool_calls":false`)
}

func TestResponsesToChatCompletionsRequest_ChainedToolCallsReplayTurnReasoning(t *testing.T) {
	req := &ResponsesRequest{
		Model: "deepseek-reasoner",
		Input: json.RawMessage(`[
			{"type":"reasoning","id":"r1","summary":[{"type":"summary_text","text":"turn thinking"}]},
			{"type":"function_call","call_id":"call_a","name":"exec","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_a","output":"ok"},
			{"type":"function_call","call_id":"call_b","name":"exec","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_b","output":"ok"},
			{"type":"message","role":"user","content":[{"type":"input_text","text":"next"}]},
			{"type":"reasoning","id":"r2","summary":[{"type":"summary_text","text":"second turn"}]},
			{"type":"function_call","call_id":"call_c","name":"exec","arguments":"{}"},
			{"type":"function_call_output","call_id":"call_c","output":"ok"}
		]`),
	}

	out, err := ResponsesToChatCompletionsRequest(req)
	require.NoError(t, err)
	byCall := make(map[string]ChatMessage)
	for _, message := range out.Messages {
		for _, call := range message.ToolCalls {
			byCall[call.ID] = message
		}
	}
	require.Equal(t, "turn thinking", byCall["call_a"].ReasoningContent)
	require.Equal(t, "turn thinking", byCall["call_b"].ReasoningContent)
	require.Equal(t, "second turn", byCall["call_c"].ReasoningContent)
}

func TestResponsesInputToChatMessages_MovesToolOutputImagesToUserMessage(t *testing.T) {
	input := json.RawMessage(`[
		{"type":"function_call","call_id":"call_1","name":"inspect","arguments":"{}"},
		{"type":"function_call_output","call_id":"call_1","output":[
			{"type":"input_text","text":"visible result"},
			{"type":"input_image","image_url":"data:image/png;base64,AAAA"}
		]}
	]`)

	messages, err := responsesInputToChatMessages("", input)
	require.NoError(t, err)
	require.Len(t, messages, 3)
	assert.Equal(t, []string{"assistant", "tool", "user"}, chatMessageRoles(messages))
	assert.Contains(t, string(messages[1].Content), toolOutputMediaMarker)

	var parts []ChatContentPart
	require.NoError(t, json.Unmarshal(messages[2].Content, &parts))
	require.Len(t, parts, 2)
	assert.Equal(t, "[Tool output media for call call_1]", parts[0].Text)
	require.NotNil(t, parts[1].ImageURL)
	assert.Equal(t, "data:image/png;base64,AAAA", parts[1].ImageURL.URL)
}

func TestResponsesInputToChatMessages_PreservesMediaFreeToolOutput(t *testing.T) {
	input := json.RawMessage(`[
		{"type":"function_call","call_id":"call_1","name":"lookup","arguments":"{}"},
		{"type":"function_call_output","call_id":"call_1","output":{"count":12,"ok":true}}
	]`)

	messages, err := responsesInputToChatMessages("", input)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	assert.JSONEq(t, `"{\"count\":12,\"ok\":true}"`, string(messages[1].Content))
}

func TestResponsesInputToChatMessages_DropsOrphanedToolOutputMedia(t *testing.T) {
	input := json.RawMessage(`[
		{"type":"function_call_output","call_id":"missing","output":"data:image/png;base64,AAAA"}
	]`)

	messages, err := responsesInputToChatMessages("", input)
	require.NoError(t, err)
	assert.Empty(t, messages)
}

func chatMessageRoles(messages []ChatMessage) []string {
	roles := make([]string, 0, len(messages))
	for _, message := range messages {
		roles = append(roles, message.Role)
	}
	return roles
}

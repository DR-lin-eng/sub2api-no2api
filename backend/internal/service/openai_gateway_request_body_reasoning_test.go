package service

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSanitizeOpenAIResponsesInputIDs_APIKeyRemovesInvalidPrefixes(t *testing.T) {
	body := []byte(`{
		"type":"response.create",
		"model":"gpt-5.4",
		"input":[
			{"type":"message","role":"user","content":[{"type":"input_text","text":"turn 0","nonce":9007199254740993}]},
			{"type":"message","role":"assistant","content":"turn 1"},
			{"type":"message","role":"user","content":"turn 2"},
			{"type":"message","role":"assistant","content":"turn 3"},
			{"type":"message","role":"user","content":"turn 4"},
			{"type":"message","role":"assistant","content":"turn 5"},
			{"type":"message","role":"user","content":"turn 6"},
			{"type":"message","role":"assistant","content":"turn 7"},
			{"type":"message","role":"user","content":"turn 8"},
			{"type":"message","role":"assistant","content":"turn 9"},
			{"type":"reasoning","id":"item_aaf212cbed95cf83ae9f2d5a","summary":[],"encrypted_content":"cipher"},
			{"type":"reasoning","id":"rs_persisted","summary":[]},
			{"type":"message","id":"item_bad_message","role":"assistant","content":"answer"},
			{"type":"message","id":"msg_persisted","role":"assistant","content":"answer"},
			{"type":"function_call","id":"item_bad_call","call_id":"fc_call","name":"tool","arguments":"{}"},
			{"type":"function_call","id":"fc_persisted","call_id":"fc_call_2","name":"tool","arguments":"{}"},
			{"type":"function_call_output","id":"item_output_is_not_constrained","call_id":"fc_call","output":"ok"},
			{"type":"item_reference","id":"item_reference_is_semantic"}
		]
	}`)

	sanitized, changed, err := sanitizeOpenAIResponsesInputIDs(body, false)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "9007199254740993", gjson.GetBytes(sanitized, "input.0.content.0.nonce").Raw)
	require.False(t, gjson.GetBytes(sanitized, "input.10.id").Exists())
	require.Equal(t, "cipher", gjson.GetBytes(sanitized, "input.10.encrypted_content").String())
	require.True(t, gjson.GetBytes(sanitized, "input.10.summary").IsArray())
	require.Equal(t, "rs_persisted", gjson.GetBytes(sanitized, "input.11.id").String())
	require.False(t, gjson.GetBytes(sanitized, "input.12.id").Exists())
	require.Equal(t, "msg_persisted", gjson.GetBytes(sanitized, "input.13.id").String())
	require.False(t, gjson.GetBytes(sanitized, "input.14.id").Exists())
	require.Equal(t, "fc_persisted", gjson.GetBytes(sanitized, "input.15.id").String())
	require.Equal(t, "item_output_is_not_constrained", gjson.GetBytes(sanitized, "input.16.id").String())
	require.Equal(t, "item_reference_is_semantic", gjson.GetBytes(sanitized, "input.17.id").String())
}

func TestSanitizeOpenAIResponsesInputIDs_OAuthStripsAllReasoningIDs(t *testing.T) {
	body := []byte(`{
		"input":[
			{"type":"reasoning","id":"rs_not_persisted","summary":[],"encrypted_content":"cipher"},
			{"type":"message","id":"msg_persisted","role":"assistant","content":"answer"},
			{"type":"function_call","id":"fc_persisted","call_id":"fc_call","name":"tool","arguments":"{}"}
		]
	}`)

	sanitized, changed, err := sanitizeOpenAIResponsesInputIDs(body, true)

	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(sanitized, "input.0.id").Exists())
	require.Equal(t, "cipher", gjson.GetBytes(sanitized, "input.0.encrypted_content").String())
	require.Equal(t, "msg_persisted", gjson.GetBytes(sanitized, "input.1.id").String())
	require.Equal(t, "fc_persisted", gjson.GetBytes(sanitized, "input.2.id").String())
}

func TestSanitizeOpenAIResponsesInputIDs_CompactsOverlongCallIDsAndPreservesPairing(t *testing.T) {
	overlongCallID := "srvtoolu_" + strings.Repeat("x", 69)
	boundaryCallID := "toolu_" + strings.Repeat("y", codexCallIDMaxLength-len("toolu_"))
	body := []byte(fmt.Sprintf(`{
		"model":"gpt-5.4",
		"input":[
			{"type":"function_call","call_id":%q,"name":"tool","arguments":"{}"},
			{"type":"function_call_output","call_id":%q,"output":"ok"},
			{"type":"function_call","call_id":%q,"name":"boundary","arguments":"{}"}
		]
	}`, overlongCallID, overlongCallID, boundaryCallID))

	sanitized, changed, err := sanitizeOpenAIResponsesInputIDs(body, false)

	require.NoError(t, err)
	require.True(t, changed)
	compacted := gjson.GetBytes(sanitized, "input.0.call_id").String()
	require.Len(t, compacted, codexCallIDMaxLength)
	require.True(t, strings.HasPrefix(compacted, codexCallIDPrefix))
	require.Equal(t, sanitizeOpenAIResponsesCallID(overlongCallID), compacted)
	require.Equal(t, compacted, gjson.GetBytes(sanitized, "input.1.call_id").String())
	require.Equal(t, boundaryCallID, gjson.GetBytes(sanitized, "input.2.call_id").String())
}

func TestSanitizeOpenAIResponsesInputIDs_SingleInputObject(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":{"type":"reasoning","id":"item_invalid","summary":[]}}`)

	sanitized, changed, err := sanitizeOpenAIResponsesInputIDs(body, false)

	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(sanitized, "input.id").Exists())
	require.Equal(t, "reasoning", gjson.GetBytes(sanitized, "input.type").String())
}

func TestSanitizeOpenAIResponsesInputIDs_SingleInputObjectCompactsCallID(t *testing.T) {
	overlongCallID := "srvtoolu_" + strings.Repeat("z", 69)
	body := []byte(fmt.Sprintf(`{"model":"gpt-5.4","input":{"type":"function_call_output","call_id":%q,"output":"ok"}}`, overlongCallID))

	sanitized, changed, err := sanitizeOpenAIResponsesInputIDs(body, false)

	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, sanitizeOpenAIResponsesCallID(overlongCallID), gjson.GetBytes(sanitized, "input.call_id").String())
}

func TestSanitizeOpenAIResponsesInputIDs_NoCandidateIsNoop(t *testing.T) {
	body := []byte(`{"model":"gpt-5.4","input":[{"type":"message","role":"user","content":"hello"}]}`)

	sanitized, changed, err := sanitizeOpenAIResponsesInputIDs(body, false)

	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, sanitized)
}

func BenchmarkSanitizeOpenAIResponsesInputIDs(b *testing.B) {
	buildHistory := func(includeIDs bool, invalidLastID bool) []byte {
		var body strings.Builder
		_, _ = body.WriteString(`{"model":"gpt-5.4","input":[`)
		for i := 0; i < 64; i++ {
			if i > 0 {
				_ = body.WriteByte(',')
			}
			if includeIDs {
				id := fmt.Sprintf("msg_%d", i)
				if invalidLastID && i == 63 {
					id = "item_aaf212cbed95cf83ae9f2d5a"
				}
				_, _ = fmt.Fprintf(&body, `{"type":"message","id":%q,"role":"user","content":"turn %d"}`, id, i)
				continue
			}
			_, _ = fmt.Fprintf(&body, `{"type":"message","role":"user","content":"turn %d"}`, i)
		}
		_, _ = body.WriteString(`]}`)
		return []byte(body.String())
	}

	overlongCallIDBody := []byte(fmt.Sprintf(`{"model":"gpt-5.4","input":[{"type":"function_call","call_id":%q,"name":"tool","arguments":"{}"}]}`, "srvtoolu_"+strings.Repeat("x", 69)))
	benchmarks := []struct {
		name string
		body []byte
	}{
		{name: "no_ids", body: buildHistory(false, false)},
		{name: "valid_ids", body: buildHistory(true, false)},
		{name: "one_invalid_id", body: buildHistory(true, true)},
		{name: "one_overlong_call_id", body: overlongCallIDBody},
	}
	for _, benchmark := range benchmarks {
		b.Run(benchmark.name, func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(benchmark.body)))
			for i := 0; i < b.N; i++ {
				_, _, err := sanitizeOpenAIResponsesInputIDs(benchmark.body, false)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestTrimOpenAIEncryptedReasoningItems_ContentNull(t *testing.T) {
	reqBody := map[string]any{
		"model": "grok-4.5",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "hi"},
			map[string]any{
				"type":              "reasoning",
				"summary":           []any{map[string]any{"type": "summary_text", "text": "thinking..."}},
				"content":           nil,
				"encrypted_content": nil,
			},
			map[string]any{"type": "message", "role": "assistant", "content": "Hello!"},
		},
	}

	changed := trimOpenAIEncryptedReasoningItems(reqBody)
	require.True(t, changed)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 3)

	reasoning, ok := input[1].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "reasoning", reasoning["type"])
	assert.NotNil(t, reasoning["summary"])
	_, hasContent := reasoning["content"]
	assert.False(t, hasContent, "content: null should be stripped")
	_, hasEncrypted := reasoning["encrypted_content"]
	assert.False(t, hasEncrypted, "encrypted_content should be stripped")
}

func TestTrimOpenAIEncryptedReasoningItems_ContentNullOnly(t *testing.T) {
	reqBody := map[string]any{
		"model": "grok-4.5",
		"input": []any{
			map[string]any{
				"type":    "reasoning",
				"summary": []any{map[string]any{"type": "summary_text", "text": "ok"}},
				"content": nil,
			},
		},
	}

	changed := trimOpenAIEncryptedReasoningItems(reqBody)
	require.True(t, changed)

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	require.Len(t, input, 1)

	reasoning, ok := input[0].(map[string]any)
	require.True(t, ok)
	_, hasContent := reasoning["content"]
	assert.False(t, hasContent, "content: null should be stripped even without encrypted_content")
}

func TestTrimOpenAIEncryptedReasoningItems_ContentNonNull(t *testing.T) {
	reqBody := map[string]any{
		"model": "grok-4.5",
		"input": []any{
			map[string]any{
				"type":    "reasoning",
				"summary": []any{map[string]any{"type": "summary_text", "text": "ok"}},
				"content": "some actual content",
			},
		},
	}

	changed := trimOpenAIEncryptedReasoningItems(reqBody)
	assert.False(t, changed, "non-null content should not be stripped")

	input, ok := reqBody["input"].([]any)
	require.True(t, ok)
	reasoning, ok := input[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "some actual content", reasoning["content"])
}

func TestTrimOpenAIEncryptedReasoningItems_NoReasoningItems(t *testing.T) {
	reqBody := map[string]any{
		"model": "grok-4.5",
		"input": []any{
			map[string]any{"type": "message", "role": "user", "content": "hi"},
		},
	}

	changed := trimOpenAIEncryptedReasoningItems(reqBody)
	assert.False(t, changed)
}

func TestTrimOpenAIEncryptedReasoningItems_ContentNullDropsBareSkeleton(t *testing.T) {
	reqBody := map[string]any{
		"input": []any{
			map[string]any{"type": "reasoning", "content": nil},
		},
	}

	changed := trimOpenAIEncryptedReasoningItems(reqBody)
	require.True(t, changed)
	_, hasInput := reqBody["input"]
	assert.False(t, hasInput, "bare reasoning skeleton should be dropped, emptying input")
}

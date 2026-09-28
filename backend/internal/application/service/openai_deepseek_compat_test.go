package service

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func deepSeekCompatAccount() *Account {
	return &Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"api_protocol": APIProtocolResponses,
		"base_url":     "https://api.deepseek.com",
	}}
}

func TestEnsureDeepSeekChatReasoningPlaceholders(t *testing.T) {
	body := []byte(`{"model":"deepseek-reasoner","messages":[{"role":"assistant","content":"old"},{"role":"assistant","reasoning_content":"real","content":"new"}]}`)
	patched := ensureDeepSeekChatReasoningPlaceholders(deepSeekCompatAccount(), body)
	require.Equal(t, " ", gjson.GetBytes(patched, "messages.0.reasoning_content").String())
	require.Equal(t, "real", gjson.GetBytes(patched, "messages.1.reasoning_content").String())
}

func TestDeepSeekNonTargetBodyRemainsByteIdentical(t *testing.T) {
	body := []byte(`{"model":"gpt-5.6","messages":[{"role":"assistant","content":"old"}]}`)
	require.True(t, bytes.Equal(body, ensureDeepSeekChatReasoningPlaceholders(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, body)))
}

func TestNormalizeDeepSeekResponsesIsStatelessAndAliasesImages(t *testing.T) {
	body := []byte(`{"model":"deepseek-reasoner","store":true,"previous_response_id":"resp_old","input":[{"type":"message","role":"user","content":[{"type":"image_url","image_url":{"url":"https://example.test/a.png"}}]}]}`)
	patched := normalizeDeepSeekResponsesRequestBody(deepSeekCompatAccount(), body)
	require.False(t, gjson.GetBytes(patched, "store").Bool())
	require.False(t, gjson.GetBytes(patched, "previous_response_id").Exists())
	require.Equal(t, "input_image", gjson.GetBytes(patched, "input.0.content.0.type").String())
	require.Equal(t, "https://example.test/a.png", gjson.GetBytes(patched, "input.0.content.0.url").String())
}

func TestDeepSeekCompactResponseUsesPlainSummary(t *testing.T) {
	resp := &apicompat.ResponsesResponse{Model: "deepseek-reasoner", Output: []apicompat.ResponsesOutput{{
		Type: "message", Content: []apicompat.ResponsesContentPart{{Type: "output_text", Text: "summary"}},
	}}}
	compact := buildDeepSeekCompactResponse(resp, "summary")
	require.Equal(t, "completed", compact.Status)
	require.Equal(t, "compaction", compact.Output[0].Type)
	require.Equal(t, "summary", compact.Output[0].EncryptedContent)
}

func TestDeepSeekAPIHostRequiresExactHostname(t *testing.T) {
	require.True(t, isDeepSeekAPIHost("https://api.deepseek.com/v1"))
	require.True(t, isDeepSeekAPIHost("https://API.DEEPSEEK.COM:443/v1"))
	require.False(t, isDeepSeekAPIHost("https://api.deepseek.com.example.test/v1"))
	require.False(t, isDeepSeekAPIHost("https://other.test/api.deepseek.com"))
}

func TestDeepSeekChatFormatOnlyStripsUnsupportedSchema(t *testing.T) {
	body := []byte(`{"model":"deepseek-reasoner","response_format":{"type":"json_schema","json_schema":{"name":"value"}},"messages":[]}`)
	patched := stripDeepSeekUnsupportedChatResponseFormat(deepSeekCompatAccount(), body)
	require.False(t, gjson.GetBytes(patched, "response_format").Exists())
	require.Equal(t, "deepseek-reasoner", gjson.GetBytes(patched, "model").String())
	nonDeepSeek := []byte(`{"model":"gpt-5.6","response_format":{"type":"json_schema"},"messages":[]}`)
	require.True(t, bytes.Equal(nonDeepSeek, stripDeepSeekUnsupportedChatResponseFormat(&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, nonDeepSeek)))
}

func TestDeepSeekResponsesLiteChoosesChatBridgeOnlyForNativeDeepSeek(t *testing.T) {
	lite := []byte(`{"model":"deepseek-reasoner","input":[{"type":"message","role":"user","content":"hi"},{"type":"additional_tools","tools":[{"type":"function","name":"exec"}]}]}`)
	require.True(t, shouldForwardDeepSeekResponsesLiteViaChatCompletions(deepSeekCompatAccount(), lite))
	require.False(t, shouldForwardDeepSeekResponsesLiteViaChatCompletions(deepSeekCompatAccount(), []byte(`{"model":"deepseek-reasoner","input":[{"type":"additional_tools","tools":[]}]}`)))
	require.False(t, shouldForwardDeepSeekResponsesLiteViaChatCompletions(&Account{Platform: PlatformKimi, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_protocol": APIProtocolResponses}}, []byte(strings.ReplaceAll(string(lite), "deepseek-reasoner", "kimi-k2"))))
	require.False(t, shouldForwardDeepSeekResponsesLiteViaChatCompletions(&Account{Platform: PlatformDeepseek, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_protocol": APIProtocolChatCompletions}}, lite))
	require.False(t, shouldForwardDeepSeekResponsesLiteViaChatCompletions(&Account{Platform: PlatformDeepseek, Type: AccountTypeOAuth}, lite))
	mapped := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"gpt-5.6": "deepseek-reasoner"}}, Extra: map[string]any{"use_responses_api": true}}
	require.False(t, shouldForwardOpenAIResponsesViaRawChatCompletions(mapped))
	require.True(t, shouldForwardDeepSeekResponsesLiteViaChatCompletions(mapped, []byte(strings.ReplaceAll(string(lite), "deepseek-reasoner", "gpt-5.6"))))
}

func TestDeepSeekNativeCompactTriggerChoosesChatBridgeOnlyWhenMarked(t *testing.T) {
	body := []byte(`{"model":"deepseek-reasoner","input":[{"type":"message","role":"user","content":"hi"},{"type":"compaction_trigger"}]}`)
	require.True(t, shouldForwardDeepSeekResponsesCompactViaChatCompletions(deepSeekCompatAccount(), body))
	ctx, _ := newExcelBPSTestContext()
	require.False(t, shouldForwardDeepSeekCompatViaChatCompletions(ctx, deepSeekCompatAccount(), body))
	MarkOpenAINativeCompactionV2(ctx)
	require.True(t, shouldForwardDeepSeekCompatViaChatCompletions(ctx, deepSeekCompatAccount(), body))
	require.False(t, shouldForwardDeepSeekResponsesCompactViaChatCompletions(deepSeekCompatAccount(), []byte(`{"model":"deepseek-reasoner","input":"hi"}`)))
}

package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestRewriteOpenAIRequestEnvironmentReplacesOnlyCompleteTimezoneField(t *testing.T) {
	input := "\n <environment_context version=\"test\">\n  <current_date>2001-01-01</current_date>\n  <timezone source='client > clock'>\n \tAsia/Shanghai\u00a0\n</timezone>\n</environment_context> \t"
	want := strings.Replace(input, "Asia/Shanghai", "America/New_York", 1)

	got, previous, reason := rewriteOpenAIRequestEnvironment(input, "America/New_York")
	require.Equal(t, "replaced", reason)
	require.Equal(t, "Asia/Shanghai", previous)
	require.Equal(t, want, got)

	got, _, reason = rewriteOpenAIRequestEnvironment(got, "America/New_York")
	require.Equal(t, "already_target", reason)
	require.Equal(t, want, got)
}

func TestRewriteOpenAIRequestEnvironmentLeavesAmbiguousTextUnchanged(t *testing.T) {
	for _, input := range []string{
		"Timezone: Asia/Shanghai",
		"<environment_context><timezone /></environment_context>",
		"<environment_context><timezone>Asia/Shanghai</timezone><timezone>Asia/Tokyo</timezone></environment_context>",
		"<environment_context><timezone><value>Asia/Shanghai</value></timezone></environment_context>",
		"<environment_context><timezone>Asia/Shanghai</environment_context>",
	} {
		got, _, reason := rewriteOpenAIRequestEnvironment(input, "America/New_York")
		require.NotEqual(t, "replaced", reason, input)
		require.Equal(t, input, got, input)
	}
}

func TestNormalizeOpenAIRequestTimezoneUsesGatewayConfigAndUserInputOnly(t *testing.T) {
	environment := "<environment_context><current_date>2001-01-01</current_date><timezone>Asia/Shanghai</timezone></environment_context>"
	body, err := json.Marshal(map[string]any{
		"input": []any{
			map[string]any{"role": "user", "content": []any{
				map[string]any{"type": "input_text", "text": environment},
				map[string]any{"type": "output_text", "text": environment},
			}},
			map[string]any{"role": "assistant", "content": environment},
		},
		"tools": []any{
			map[string]any{"type": "web_search", "user_location": map[string]any{"timezone": "America/New_York", "city": "New York"}},
			map[string]any{"type": "function", "user_location": map[string]any{"timezone": "America/New_York"}},
		},
	})
	require.NoError(t, err)

	svc := &OpenAIGatewayService{cfg: &config.Config{Gateway: config.GatewayConfig{OpenAIRequestTimezone: "Asia/Tokyo"}}}
	account := &Account{Platform: PlatformOpenAI}
	out := svc.normalizeOpenAIRequestTimezone(account, body, "http")
	require.Equal(t, "<environment_context><current_date>2001-01-01</current_date><timezone>Asia/Tokyo</timezone></environment_context>", gjson.GetBytes(out, "input.0.content.0.text").String())
	require.Equal(t, environment, gjson.GetBytes(out, "input.0.content.1.text").String())
	require.Equal(t, environment, gjson.GetBytes(out, "input.1.content").String())
	require.Equal(t, "Asia/Tokyo", gjson.GetBytes(out, "tools.0.user_location.timezone").String())
	require.Equal(t, "America/New_York", gjson.GetBytes(out, "tools.1.user_location.timezone").String())

	// The same configured target is used on the websocket path and the second
	// pass is idempotent, so retries do not alter the request again.
	require.Equal(t, out, svc.normalizeOpenAIRequestTimezone(account, out, "ws"))
	for _, nonOpenAI := range []*Account{nil, {Platform: PlatformAnthropic}} {
		require.Equal(t, body, svc.normalizeOpenAIRequestTimezone(nonOpenAI, body, "http"))
	}
}

func TestNormalizeOpenAIRequestTimezoneDefaultsToSingapore(t *testing.T) {
	body := []byte(`{"input":[{"role":"user","content":"<environment_context><timezone>Asia/Shanghai</timezone></environment_context>"}]}`)
	out := (&OpenAIGatewayService{}).normalizeOpenAIRequestTimezone(&Account{Platform: PlatformOpenAI}, body, "http")
	require.Equal(t, "<environment_context><timezone>Asia/Singapore</timezone></environment_context>", gjson.GetBytes(out, "input.0.content").String())
}

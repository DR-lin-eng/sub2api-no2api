package service

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
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
	out := svc.normalizeOpenAIRequestTimezone(context.Background(), nil, account, body, "http")
	require.Equal(t, "<environment_context><current_date>2001-01-01</current_date><timezone>Asia/Tokyo</timezone></environment_context>", gjson.GetBytes(out, "input.0.content.0.text").String())
	require.Equal(t, environment, gjson.GetBytes(out, "input.0.content.1.text").String())
	require.Equal(t, environment, gjson.GetBytes(out, "input.1.content").String())
	require.Equal(t, "Asia/Tokyo", gjson.GetBytes(out, "tools.0.user_location.timezone").String())
	require.Equal(t, "America/New_York", gjson.GetBytes(out, "tools.1.user_location.timezone").String())

	// The same configured target is used on the websocket path and the second
	// pass is idempotent, so retries do not alter the request again.
	require.Equal(t, out, svc.normalizeOpenAIRequestTimezone(context.Background(), nil, account, out, "ws"))
	for _, nonOpenAI := range []*Account{nil, {Platform: PlatformAnthropic}} {
		require.Equal(t, body, svc.normalizeOpenAIRequestTimezone(context.Background(), nil, nonOpenAI, body, "http"))
	}
}

func TestNormalizeOpenAIRequestTimezoneDefaultsToSingapore(t *testing.T) {
	body := []byte(`{"input":[{"role":"user","content":"<environment_context><timezone>Asia/Shanghai</timezone></environment_context>"}]}`)
	out := (&OpenAIGatewayService{}).normalizeOpenAIRequestTimezone(context.Background(), nil, &Account{Platform: PlatformOpenAI}, body, "http")
	require.Equal(t, "<environment_context><timezone>Asia/Singapore</timezone></environment_context>", gjson.GetBytes(out, "input.0.content").String())
}

func TestNormalizeOpenAIRequestTimezoneForRequestMatchesLegacyWalk(t *testing.T) {
	environment := `<environment_context><timezone>Asia/Shanghai</timezone></environment_context>`
	escaped := `\u003cenvironment_context\u003e\u003ctimezone\u003eAsia/Shanghai\u003c/timezone\u003e\u003c/\u0065nvironment_context\u003e`
	bodies := []string{
		`{"input":[{"role":"user","content":"` + environment + `"}]}`,
		`{"input":[{"role":"user","content":"` + escaped + `"}]}`,
		`{"input":[{"role":"user","content":"plain text"},{"role":"user","content":[{"type":"input_text","text":"` + environment + `"},{"type":"input_text","text":7},"x",{"type":"output_text","text":"` + environment + `"}]}]}`,
		`{"input":[{"role":"assistant","content":"` + environment + `"},"text",{"role":"user","content":{"text":"` + environment + `"}}]}`,
		`{"input":[{"role":"user","content":"` + environment + `"}],"tools":[{"type":"web_search","user_location":{"timezone":"America/New_York"}},{"type":"function"},{"type":"web_search_preview","user_location":{"timezone":"Europe/Paris"}},{"type":"web_search","user_location":{"timezone":"Asia/Tokyo"}}]}`,
		`{"tools":[{"type":"web_search","user_location":{"timezone":"America/New_York"}}],"input":"` + environment + `"}`,
		`{"input":[{"role":"user","content":"a"}],"input":[{"role":"user","content":"` + environment + `"}],"tools":{"type":"web_search"}}`,
		`prefix {"input":[{"role":"user","content":"` + environment + `"}]}`,
		`[{"role":"user","content":"` + environment + `"}]`,
		`{"input":[{"role":"user","content":"` + environment + `"`,
		``,
	}
	for _, body := range bodies {
		want := legacyNormalizeOpenAIRequestTimezone([]byte(body), "Asia/Tokyo")
		require.Equal(t, string(want), string(normalizeOpenAIRequestTimezone([]byte(body), "Asia/Tokyo")), body)
		require.Equal(t, string(want), string(normalizeOpenAIRequestTimezoneForRequest(newOpenAIRequestBodyIndexTestContext(), []byte(body), "Asia/Tokyo")), body)
	}
}

// legacyNormalizeOpenAIRequestTimezone is the implementation that preceded the
// view-based walk, kept as the reference for byte-for-byte equivalence.
func legacyNormalizeOpenAIRequestTimezone(body []byte, target string) []byte {
	input := gjson.GetBytes(body, "input")
	if input.IsArray() {
		for inputIndex, item := range input.Array() {
			if item.Get("role").String() != "user" {
				continue
			}
			content := item.Get("content")
			path := "input." + strconv.Itoa(inputIndex) + ".content"
			if content.Type == gjson.String {
				body = rewriteOpenAIRequestText(body, path, content.String(), target)
				continue
			}
			if !content.IsArray() {
				continue
			}
			for contentIndex, part := range content.Array() {
				if part.Get("type").String() != "input_text" || part.Get("text").Type != gjson.String {
					continue
				}
				body = rewriteOpenAIRequestText(body, path+"."+strconv.Itoa(contentIndex)+".text", part.Get("text").String(), target)
			}
		}
	}

	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return body
	}
	for index, tool := range tools.Array() {
		kind := tool.Get("type").String()
		if kind != "web_search" && !strings.HasPrefix(kind, "web_search_") {
			continue
		}
		value := tool.Get("user_location.timezone")
		if value.Type != gjson.String || value.String() == target {
			continue
		}
		if updated, err := sjson.SetBytes(body, "tools."+strconv.Itoa(index)+".user_location.timezone", target); err == nil {
			body = updated
		}
	}
	return body
}

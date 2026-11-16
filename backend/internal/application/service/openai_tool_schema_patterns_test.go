package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestOpenAIToolRegexCompatibility(t *testing.T) {
	for _, pattern := range []string{`(?<name>abc)`, `(?'name'abc)`, `(?=abc)`, `\p{L}+`, `\R`, `\k<name>`, `abc\`} {
		t.Run(pattern, func(t *testing.T) {
			encoded, err := json.Marshal(pattern)
			require.NoError(t, err)
			body := []byte(`{"input":"pattern stays (?<name>x)","tools":[{"parameters":{"pattern":` + string(encoded) + `,"default":{"pattern":` + string(encoded) + `}}}]}`)
			original := append([]byte(nil), body...)
			got, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns(body)
			require.NoError(t, err)
			require.True(t, changed)
			require.True(t, json.Valid(got), string(got))
			require.Equal(t, ".*", gjson.GetBytes(got, "tools.0.parameters.pattern").String())
			require.Equal(t, pattern, gjson.GetBytes(got, "tools.0.parameters.default.pattern").String())
			require.Equal(t, original, body)
		})
	}
	for _, pattern := range []string{`(?P<name>abc)`, `(?:abc)`, `[()?<]`, `\\p`, `\d+\s\w\A\Z`, `[a-z]+`} {
		encoded, err := json.Marshal(pattern)
		require.NoError(t, err)
		body := []byte(`{"tools":[{"parameters":{"pattern":` + string(encoded) + `}}]}`)
		got, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns(body)
		require.NoError(t, err)
		require.False(t, changed, pattern)
		require.Equal(t, body, got)
	}
}

func TestOpenAIToolPatternPropertiesRemainRepresentable(t *testing.T) {
	bad := `"(?<bad>x)":{"pattern":"\\p{L}"}`
	good := `"^valid$":{"pattern":"(?<nested>abc)","default":{"pattern":"\\p{L}"}}`
	for _, entries := range []string{bad + "," + good, good + "," + bad, bad, bad + "," + good + "," + `"\\R":true`} {
		for _, additional := range []string{"false", `{"pattern":"\\p{L}"}`} {
			body := []byte(`{"tools":[{"parameters":{"additionalProperties":` + additional + `,"patternProperties":{` + entries + `},"properties":{"pattern":{"type":"string"}},"const":{"pattern":"\\p{L}"}}}]}`)
			got, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns(body)
			require.NoError(t, err)
			require.True(t, changed)
			require.True(t, json.Valid(got), string(got))
			require.Equal(t, "true", gjson.GetBytes(got, "tools.0.parameters.additionalProperties").Raw)
			patterns := gjson.GetBytes(got, "tools.0.parameters.patternProperties")
			patterns.ForEach(func(key, value gjson.Result) bool {
				require.Equal(t, "^valid$", key.String())
				require.Equal(t, ".*", value.Get("pattern").String())
				require.Equal(t, `\p{L}`, value.Get("default.pattern").String())
				return true
			})
			require.Equal(t, "string", gjson.GetBytes(got, "tools.0.parameters.properties.pattern.type").String())
			require.Equal(t, `\p{L}`, gjson.GetBytes(got, "tools.0.parameters.const.pattern").String())
		}
	}
}

func TestOpenAIToolRegexPlatformAndHistoryBoundaries(t *testing.T) {
	body := []byte(`{"input":[{"tools":[{"tools":[{"function":{"parameters":{"pat\u0074ern":"\\p{L}"}}}]}]}]}`)
	got, changed, err := sanitizeOpenAIResponsesToolSchemasForPlatform(body, PlatformOpenAI)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, ".*", gjson.GetBytes(got, "input.0.tools.0.tools.0.function.parameters.pattern").String())
	for _, platform := range []string{PlatformGrok, PlatformAnthropic, PlatformKimi} {
		got, changed, err := sanitizeOpenAIResponsesToolSchemasForPlatform(body, platform)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, body, got)
	}
	for _, malformed := range []string{`{"tools":[{"parameters":{"pattern":"\\p{L}"}}]`, `{"pattern":"\\p{L}"}`} {
		got, changed, err := sanitizeOpenAIResponsesToolSchemaPatterns([]byte(malformed))
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, malformed, string(got))
	}
}

func BenchmarkFollowupToolSchemaPatterns(b *testing.B) {
	for _, count := range []int{100, 1000} {
		body := []byte(`{"input":"` + strings.Repeat("x", 1<<20) + `","tools":[` + strings.TrimSuffix(strings.Repeat(`{"parameters":{"type":"object","pattern":"(?<name>abc)"}},`, count), ",") + `]}`)
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			b.ReportAllocs()
			b.SetBytes(int64(len(body)))
			for b.Loop() {
				_, _, _ = sanitizeOpenAIResponsesToolSchemaPatterns(body)
			}
		})
	}
}

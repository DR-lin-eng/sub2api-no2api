package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSanitizeOpenAIResponsesToolParameterTypes(t *testing.T) {
	t.Run("top level function tool", func(t *testing.T) {
		body := []byte(`{"model":"gpt-5.6-sol","input":"Reply with OK.","tools":[{"type":"function","name":"automation_update","description":"Update an automation.","parameters":{"type":null,"properties":{}}}]}`)

		sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, "object", gjson.GetBytes(sanitized, "tools.0.parameters.type").String())
		require.Equal(t, "automation_update", gjson.GetBytes(sanitized, "tools.0.name").String())
		require.Equal(t, "gpt-5.6-sol", gjson.GetBytes(sanitized, "model").String())
	})

	t.Run("nested historical and chat completions tools", func(t *testing.T) {
		body := []byte(`{"input":[{"type":"additional_tools","tools":[{"type":"namespace","tools":[{"type":"function","name":"nested","parameters":{"type":null}}]}]}],"tools":[{"type":"function","function":{"name":"legacy","parameters":{"type":null}}}]}`)

		sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, "object", gjson.GetBytes(sanitized, "input.0.tools.0.tools.0.parameters.type").String())
		require.Equal(t, "object", gjson.GetBytes(sanitized, "tools.0.function.parameters.type").String())
	})

	t.Run("valid and missing types stay byte identical", func(t *testing.T) {
		for _, body := range [][]byte{
			[]byte(`{"tools":[{"type":"function","parameters":{"type":"object"}}]}`),
			[]byte(`{"tools":[{"type":"function","parameters":{"properties":{}}}]}`),
		} {
			sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, body, sanitized)
		}
	})

	t.Run("malformed shapes are no ops", func(t *testing.T) {
		for _, body := range [][]byte{
			nil,
			[]byte(`{"tools":null}`),
			[]byte(`{"tools":{"type":"function"}}`),
			[]byte(`{"tools":["freeform"]}`),
			[]byte(`{"tools":[{"parameters":null}]}`),
			[]byte(`{"tools":[{"parameters":{"type":["object","null"]}}]}`),
		} {
			sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)
			require.NoError(t, err)
			require.False(t, changed)
			require.Equal(t, body, sanitized)
		}
	})

	t.Run("does not mutate input", func(t *testing.T) {
		body := []byte(`{"tools":[{"parameters":{"type":null}}]}`)
		original := append([]byte(nil), body...)

		sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)

		require.NoError(t, err)
		require.True(t, changed)
		require.Equal(t, original, body)
		require.NotEqual(t, original, sanitized)
	})
}

func buildToolSchemaNullTypeBody(t *testing.T, hits int) []byte {
	t.Helper()
	tools := make([]any, 0, hits)
	for i := 0; i < hits; i++ {
		tools = append(tools, map[string]any{
			"type":       "function",
			"name":       "automation_update",
			"parameters": map[string]any{"type": nil, "properties": map[string]any{}},
		})
	}
	body, err := json.Marshal(map[string]any{"model": "gpt-5.6-sol", "tools": tools})
	require.NoError(t, err)
	return body
}

func TestSanitizeOpenAIResponsesToolParameterTypes_RewriteCountIndependentOfHits(t *testing.T) {
	small := buildToolSchemaNullTypeBody(t, 4)
	large := buildToolSchemaNullTypeBody(t, 2000)

	smallAllocs := testing.AllocsPerRun(2, func() {
		_, _, _ = sanitizeOpenAIResponsesToolParameterTypes(small)
	})
	largeAllocs := testing.AllocsPerRun(2, func() {
		_, _, _ = sanitizeOpenAIResponsesToolParameterTypes(large)
	})

	require.Less(t, largeAllocs, smallAllocs+40)
	sanitized, changed, err := sanitizeOpenAIResponsesToolParameterTypes(large)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, 2000, int(gjson.GetBytes(sanitized, "tools.#").Int()))
}

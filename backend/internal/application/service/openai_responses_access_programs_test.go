package service

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestSanitizeOpenAIResponsesAccessPrograms(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantChanged   bool
		wantBody      string
		wantTopLevel  bool
		wantNestedKey bool
	}{
		{
			name:          "removes top-level server-owned selection",
			body:          `{"model":"gpt-5.6-sol","access_programs":{"cyber":"daybreak_red"},"input":"hello"}`,
			wantChanged:   true,
			wantBody:      `{"model":"gpt-5.6-sol","input":"hello"}`,
			wantTopLevel:  false,
			wantNestedKey: false,
		},
		{
			name:          "preserves nested user payload",
			body:          `{"model":"gpt-5.6-sol","input":{"access_programs":{"cyber":"user-data"}}}`,
			wantChanged:   false,
			wantBody:      `{"model":"gpt-5.6-sol","input":{"access_programs":{"cyber":"user-data"}}}`,
			wantTopLevel:  false,
			wantNestedKey: true,
		},
		{
			name:          "leaves payload without selection unchanged",
			body:          `{"model":"gpt-5.6-sol","input":"hello"}`,
			wantChanged:   false,
			wantBody:      `{"model":"gpt-5.6-sol","input":"hello"}`,
			wantTopLevel:  false,
			wantNestedKey: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed, err := sanitizeOpenAIResponsesAccessPrograms([]byte(tt.body))
			require.NoError(t, err)
			require.Equal(t, tt.wantChanged, changed)
			require.JSONEq(t, tt.wantBody, string(got))
			require.Equal(t, tt.wantTopLevel, gjson.GetBytes(got, "access_programs").Exists())
			require.Equal(t, tt.wantNestedKey, gjson.GetBytes(got, "input.access_programs").Exists())
		})
	}
}

func TestSanitizeOpenAIResponsesServerOwnedFieldsKeepsNestedUserData(t *testing.T) {
	body := []byte(`{"serviceName":"codex_work_desktop","daybreakEnabled":true,"cyberAccessProgram":"standard","disabledPluginIds":["plugin-a"],"approvalPolicy":"never","sandboxPolicy":{"type":"danger-full-access"},"input":{"serviceName":"user-data","access_programs":{"cyber":"user-data"}},"tools":[{"type":"function","function":{"name":"x","parameters":{"serviceTierForTurn":"user-data"}}}]}`)

	got, changed, err := sanitizeOpenAIResponsesServerOwnedFields(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(got, "serviceName").Exists())
	require.False(t, gjson.GetBytes(got, "daybreakEnabled").Exists())
	require.False(t, gjson.GetBytes(got, "cyberAccessProgram").Exists())
	require.False(t, gjson.GetBytes(got, "disabledPluginIds").Exists())
	require.False(t, gjson.GetBytes(got, "approvalPolicy").Exists())
	require.False(t, gjson.GetBytes(got, "sandboxPolicy").Exists())
	require.Equal(t, "user-data", gjson.GetBytes(got, "input.serviceName").String())
	require.Equal(t, "user-data", gjson.GetBytes(got, "input.access_programs.cyber").String())
	require.Equal(t, "user-data", gjson.GetBytes(got, "tools.0.function.parameters.serviceTierForTurn").String())
}

func TestSanitizeOpenAIResponsesServerOwnedFieldsIsCaseInsensitiveAtTopLevel(t *testing.T) {
	body := []byte(`{"ACCESS_PROGRAMS":{"cyber":"daybreak_red"},"ServiceName":"codex_work_desktop","input":{"ACCESS_PROGRAMS":{"cyber":"user-data"}}}`)
	got, changed, err := sanitizeOpenAIResponsesServerOwnedFields(body)
	require.NoError(t, err)
	require.True(t, changed)
	require.False(t, gjson.GetBytes(got, "ACCESS_PROGRAMS").Exists())
	require.False(t, gjson.GetBytes(got, "ServiceName").Exists())
	require.Equal(t, "user-data", gjson.GetBytes(got, "input.ACCESS_PROGRAMS.cyber").String())
}

func TestSanitizeOpenAIResponsesAccessProgramsMap(t *testing.T) {
	payload := map[string]any{
		"model":           "gpt-5.6-sol",
		"access_programs": map[string]any{"cyber": "daybreak_blue"},
	}

	require.True(t, sanitizeOpenAIResponsesAccessProgramsMap(payload))
	_, exists := payload["access_programs"]
	require.False(t, exists)
	require.False(t, sanitizeOpenAIResponsesAccessProgramsMap(payload))
}

func TestSanitizeOpenAIResponsesAccessProgramsIgnoresInvalidJSON(t *testing.T) {
	body := []byte(`{"access_programs":`)
	got, changed, err := sanitizeOpenAIResponsesAccessPrograms(body)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, body, got)
}

package handler

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const handlerDelegationEnvelope = `<codex_delegation><source_thread_id>thread-1</source_thread_id><input>inspect</input></codex_delegation>`

func TestNormalizeCodexDelegationBootstrap(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"function_call_output","id":"fco_01a05cff-3864-7e31-bc57-65098a0035a9","namespace":"codex_app","name":"create_thread","output":` + handlerMustJSON(t, handlerDelegationEnvelope) + `}]}`)

	got, changed := normalizeCodexDelegationBootstrap(body)

	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(got, "input.0.type").String())
	require.Equal(t, handlerDelegationEnvelope, gjson.GetBytes(got, "input.0.content.0.text").String())
	require.False(t, gjson.GetBytes(got, "input.0.call_id").Exists())
	_, automationChanged := normalizeCodexAutomationBootstrap(body)
	require.False(t, automationChanged)
}

func TestNormalizeCodexAutomationBootstrap(t *testing.T) {
	output := "Automation: Project review\n" +
		"Automation ID: project-review\n" +
		"Automation memory: $CODEX_HOME/automations/project-review/memory.md\n" +
		"Last run: never\n\nReview the project."
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"function_call_output","id":"fco_automation","namespace":"codex_app","name":"automation_update","output":` + handlerMustJSON(t, output) + `}]}`)

	got, changed := normalizeCodexAutomationBootstrap(body)

	require.True(t, changed)
	require.Equal(t, "message", gjson.GetBytes(got, "input.0.type").String())
	require.Equal(t, output, gjson.GetBytes(got, "input.0.content.0.text").String())
	_, delegationChanged := normalizeCodexDelegationBootstrap(body)
	require.False(t, delegationChanged)
}

func TestNormalizeCodexBootstrapRejectsOrdinaryMissingCallID(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"function_call_output","id":"fco_ordinary","namespace":"codex_app","name":"exec_command","output":"ok"}]}`)
	got, delegationChanged := normalizeCodexDelegationBootstrap(body)
	require.False(t, delegationChanged)
	require.Equal(t, body, got)
	got, automationChanged := normalizeCodexAutomationBootstrap(body)
	require.False(t, automationChanged)
	require.Equal(t, body, got)
}

func handlerMustJSON(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}

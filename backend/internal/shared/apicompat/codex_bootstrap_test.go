package apicompat

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const codexDelegationBootstrapEnvelope = `<codex_delegation><source_thread_id>thread-1</source_thread_id><input>inspect</input></codex_delegation>`

func TestNormalizeCodexCallOutputBootstrap_DelegationWithFCOItemID(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"function_call_output","id":"fco_01a05cff-3864-7e31-bc57-65098a0035a9","namespace":"codex_app","name":"create_thread","output":` + string(mustMarshalJSON(t, codexDelegationBootstrapEnvelope)) + `}]}`)

	got, kind, changed := NormalizeCodexCallOutputBootstrap(body)

	require.True(t, changed)
	require.Equal(t, CodexBootstrapDelegation, kind)
	require.Equal(t, "message", gjson.GetBytes(got, "input.0.type").String())
	require.Equal(t, "user", gjson.GetBytes(got, "input.0.role").String())
	require.Equal(t, codexDelegationBootstrapEnvelope, gjson.GetBytes(got, "input.0.content.0.text").String())
	require.False(t, gjson.GetBytes(got, "input.0.id").Exists())
	require.False(t, gjson.GetBytes(got, "input.0.call_id").Exists())
	again, againKind, againChanged := NormalizeCodexCallOutputBootstrap(got)
	require.False(t, againChanged)
	require.Empty(t, againKind)
	require.Equal(t, got, again)
}

func TestNormalizeCodexCallOutputBootstrap_Automation(t *testing.T) {
	output := "Automation: Project review\n" +
		"Automation ID: project-review\n" +
		"Automation memory: $CODEX_HOME/automations/project-review/memory.md\n" +
		"Last run: 2026-09-01T02:06:34.536Z (1788228394536)\n\n" +
		"Review the project."
	body := []byte(`{"model":"gpt-5.5","input":[{"type":"function_call_output","id":"fco_automation","namespace":"codex_app","name":"automation_update","output":` + string(mustMarshalJSON(t, output)) + `}]}`)

	got, kind, changed := NormalizeCodexCallOutputBootstrap(body)

	require.True(t, changed)
	require.Equal(t, CodexBootstrapAutomation, kind)
	require.Equal(t, output, gjson.GetBytes(got, "input.0.content.0.text").String())
}

func TestNormalizeCodexCallOutputBootstrap_DelegationWithHistoricalContext(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","previous_response_id":"resp_1","input":[{"type":"function_call_output","id":"fco_wakeup","namespace":"codex_app","name":"send_message_to_thread","output":` + string(mustMarshalJSON(t, codexDelegationBootstrapEnvelope)) + `},{"type":"function_call","call_id":"call_1","name":"inspect","arguments":"{}"},{"type":"function_call_output","call_id":"call_1","output":"done"},{"type":"item_reference","id":"msg_1"}]}`)

	got, kind, changed := NormalizeCodexCallOutputBootstrap(body)

	require.True(t, changed)
	require.Equal(t, CodexBootstrapDelegation, kind)
	require.Equal(t, "resp_1", gjson.GetBytes(got, "previous_response_id").String())
	require.Equal(t, "message", gjson.GetBytes(got, "input.0.type").String())
	require.Equal(t, "call_1", gjson.GetBytes(got, "input.1.call_id").String())
	require.Equal(t, "call_1", gjson.GetBytes(got, "input.2.call_id").String())
	require.Equal(t, "msg_1", gjson.GetBytes(got, "input.3.id").String())
}

func TestNormalizeCodexCallOutputBootstrap_RejectsOrdinaryOrAmbiguousOutputs(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "ordinary output",
			body: `{"model":"gpt-5.5","input":[{"type":"function_call_output","id":"fco_1","namespace":"codex_app","name":"exec_command","output":"ok"}]}`,
		},
		{
			name: "real call id",
			body: `{"model":"gpt-5.5","input":[{"type":"function_call_output","id":"fco_1","call_id":"call_1","namespace":"codex_app","name":"create_thread","output":` + string(mustMarshalJSON(t, codexDelegationBootstrapEnvelope)) + `}]}`,
		},
		{
			name: "ambiguous call context",
			body: `{"model":"gpt-5.5","input":[{"type":"function_call"},{"type":"function_call_output","id":"fco_1","namespace":"codex_app","name":"create_thread","output":` + string(mustMarshalJSON(t, codexDelegationBootstrapEnvelope)) + `}]}`,
		},
		{
			name: "duplicate discriminator",
			body: `{"model":"gpt-5.5","input":[{"type":"message","type":"function_call_output","id":"fco_1","namespace":"codex_app","name":"create_thread","output":` + string(mustMarshalJSON(t, codexDelegationBootstrapEnvelope)) + `}]}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := []byte(tt.body)
			got, kind, changed := NormalizeCodexCallOutputBootstrap(body)
			require.False(t, changed)
			require.Empty(t, kind)
			require.Equal(t, body, got)
		})
	}
}

func TestNormalizeCodexCallOutputBootstrap_Heartbeat(t *testing.T) {
	for _, tc := range []struct {
		output string
		valid  bool
	}{
		{`<heartbeat><automation_id>review-pr</automation_id></heartbeat>`, true},
		{`<heartbeat><automation_id>review-pr</automation_id><automation_id>x</automation_id></heartbeat>`, false},
		{`<heartbeat><automation_id>../x</automation_id></heartbeat>`, false},
		{`<heartbeat attr="x"><automation_id>review-pr</automation_id></heartbeat>`, false},
		{`<heartbeat><!--x--><automation_id>review-pr</automation_id></heartbeat>`, false},
		{`<heartbeat><automation_id>review-pr</automation_id></heartbeat><heartbeat/>`, false},
	} {
		t.Run(tc.output, func(t *testing.T) {
			body := []byte(`{"input":[{"type":"function_call_output","id":"fco_heartbeat","namespace":"codex_app","name":"automation_update","output":` + string(mustMarshalJSON(t, tc.output)) + `}]}`)
			got, kind, changed := NormalizeCodexCallOutputBootstrap(body)
			require.Equal(t, tc.valid, changed)
			if tc.valid {
				require.Equal(t, CodexBootstrapAutomation, kind)
				require.Equal(t, tc.output, gjson.GetBytes(got, "input.0.content.0.text").String())
			} else {
				require.Equal(t, body, got)
			}
		})
	}
}

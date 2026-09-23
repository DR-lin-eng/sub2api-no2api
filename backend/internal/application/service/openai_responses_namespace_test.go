package service

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestShouldFlattenOpenAIResponsesNamespaces(t *testing.T) {
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	flattenOAuth := &Account{
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"openai_responses_flatten_namespaces": true},
	}
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	grokOAuth := &Account{Platform: PlatformGrok, Type: AccountTypeOAuth}

	tests := []struct {
		name               string
		account            *Account
		transport          OpenAIUpstreamTransport
		passthroughEnabled bool
		compactPath        bool
		want               bool
	}{
		{name: "oauth_http_preserves_by_default", account: oauth, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
		{name: "oauth_http_passthrough_preserves_by_default", account: oauth, transport: OpenAIUpstreamTransportHTTPSSE, passthroughEnabled: true, want: false},
		{name: "oauth_http_compatibility_switch", account: flattenOAuth, transport: OpenAIUpstreamTransportHTTPSSE, want: true},
		{name: "oauth_http_compact", account: oauth, transport: OpenAIUpstreamTransportHTTPSSE, compactPath: true, want: true},
		{name: "oauth_wsv2", account: oauth, transport: OpenAIUpstreamTransportResponsesWebsocketV2, want: false},
		{name: "oauth_wsv2_compatibility_switch", account: flattenOAuth, transport: OpenAIUpstreamTransportResponsesWebsocketV2, want: false},
		{name: "oauth_wsv2_passthrough_compatibility_switch", account: flattenOAuth, transport: OpenAIUpstreamTransportResponsesWebsocketV2, passthroughEnabled: true, want: true},
		{name: "apikey_http", account: apiKey, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
		{name: "grok_oauth_http", account: grokOAuth, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
		{name: "nil_account", account: nil, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldFlattenOpenAIResponsesNamespaces(tt.account, tt.transport, tt.passthroughEnabled, tt.compactPath))
		})
	}
}

func TestShouldStripOpenAIResponsesInputNamespaces(t *testing.T) {
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	setupToken := &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken}
	grokOAuth := &Account{Platform: PlatformGrok, Type: AccountTypeOAuth}

	tests := []struct {
		name               string
		account            *Account
		transport          OpenAIUpstreamTransport
		passthroughEnabled bool
		want               bool
	}{
		{name: "oauth_http", account: oauth, transport: OpenAIUpstreamTransportHTTPSSE, want: true},
		{name: "apikey_http", account: apiKey, transport: OpenAIUpstreamTransportHTTPSSE, want: true},
		{name: "oauth_wsv2", account: oauth, transport: OpenAIUpstreamTransportResponsesWebsocketV2, want: false},
		{name: "apikey_wsv2", account: apiKey, transport: OpenAIUpstreamTransportResponsesWebsocketV2, want: false},
		{name: "oauth_wsv2_passthrough", account: oauth, transport: OpenAIUpstreamTransportResponsesWebsocketV2, passthroughEnabled: true, want: true},
		{name: "apikey_wsv2_passthrough", account: apiKey, transport: OpenAIUpstreamTransportResponsesWebsocketV2, passthroughEnabled: true, want: true},
		{name: "setup_token_http", account: setupToken, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
		{name: "grok_oauth_http", account: grokOAuth, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
		{name: "nil_account", account: nil, transport: OpenAIUpstreamTransportHTTPSSE, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, shouldStripOpenAIResponsesInputNamespaces(tt.account, tt.transport, tt.passthroughEnabled))
		})
	}
}

func TestShouldKeepOpenAIResponsesToolCallNamespacesForLiteDeclarations(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	body := []byte(`{"input":[{"type":"additional_tools","tools":[{"type":"namespace","name":"collaboration"}]},{"type":"function_call","namespace":"collaboration","name":"spawn_agent"}]}`)
	require.True(t, shouldKeepOpenAIResponsesToolCallNamespaces(
		account, OpenAIUpstreamTransportHTTPSSE, false, false, body,
	))
	require.False(t, shouldKeepOpenAIResponsesToolCallNamespaces(
		account, OpenAIUpstreamTransportHTTPSSE, false, true, body,
	))
	require.False(t, shouldKeepOpenAIResponsesToolCallNamespaces(
		account, OpenAIUpstreamTransportHTTPSSE, false, false,
		[]byte(`{"tools":[{"type":"function","namespace":"residual"}]}`),
	))
}

func TestStripOpenAIResponsesInputNamespaces(t *testing.T) {
	body := []byte(`{
		"meta":9007199254740993,
		"scientific":1.25e+42,
		"escaped":"line\\n\\u003ctag\\u003e",
		"tools":[{"type":"function","name":"keep","namespace":"tool-namespace"}],
		"input":[
			{"type":"function_call","namespace":"n0","name":"one","content":{"namespace":"nested"},"large":9007199254740993},
			{"type":"message","namespace":"n1","content":[{"type":"input_text","text":"hello","namespace":"nested-content"}]},
			{"type":"custom_tool_call","namespace":"n2","input":"{}"},
			{"type":"function_call_output","namespace":"n3","output":"ok"},
			{"type":"item","namespace":"n4"},
			{"type":"item","namespace":"n5"},
			{"type":"item","namespace":"n6"},
			{"type":"item","namespace":"n7"}
		]
	}`)

	stripped, err := stripOpenAIResponsesInputNamespaces(body, false)
	require.NoError(t, err)
	for index := 0; index < 8; index++ {
		require.False(t, gjson.GetBytes(stripped, "input."+strconv.Itoa(index)+".namespace").Exists())
	}
	require.Equal(t, "nested", gjson.GetBytes(stripped, "input.0.content.namespace").String())
	require.Equal(t, "nested-content", gjson.GetBytes(stripped, "input.1.content.0.namespace").String())
	require.Equal(t, "tool-namespace", gjson.GetBytes(stripped, "tools.0.namespace").String())
	require.Equal(t, gjson.GetBytes(body, "meta").Raw, gjson.GetBytes(stripped, "meta").Raw)
	require.Equal(t, gjson.GetBytes(body, "scientific").Raw, gjson.GetBytes(stripped, "scientific").Raw)
	require.Equal(t, gjson.GetBytes(body, "escaped").Raw, gjson.GetBytes(stripped, "escaped").Raw)
	require.Equal(t, gjson.GetBytes(body, "input.0.large").Raw, gjson.GetBytes(stripped, "input.0.large").Raw)
}

func TestStripOpenAIResponsesInputNamespacesPreservesToolCallsWhenRequested(t *testing.T) {
	body := []byte(`{"input":[
		{"type":"function_call","namespace":"collaboration","name":"spawn_agent"},
		{"type":"custom_tool_call","namespace":"tools","name":"run"},
		{"type":"message","namespace":"residual","content":[{"namespace":"nested"}]},
		{"type":"function_call_output","namespace":"residual","output":"ok"}
	]}`)

	stripped, err := stripOpenAIResponsesInputNamespaces(body, true)
	require.NoError(t, err)
	require.Equal(t, "collaboration", gjson.GetBytes(stripped, "input.0.namespace").String())
	require.Equal(t, "tools", gjson.GetBytes(stripped, "input.1.namespace").String())
	require.False(t, gjson.GetBytes(stripped, "input.2.namespace").Exists())
	require.Equal(t, "nested", gjson.GetBytes(stripped, "input.2.content.0.namespace").String())
	require.False(t, gjson.GetBytes(stripped, "input.3.namespace").Exists())
}

func TestStripOpenAIResponsesInputNamespacesLeavesOtherShapesByteExact(t *testing.T) {
	tests := [][]byte{
		[]byte(`{"input":"text","namespace":"top-level"}`),
		[]byte(`{"input":{"namespace":"single-object"}}`),
		[]byte(`{"input":[{"content":{"namespace":"nested-only"}}],"tools":[{"namespace":"keep"}]}`),
	}
	for _, body := range tests {
		stripped, err := stripOpenAIResponsesInputNamespaces(body, false)
		require.NoError(t, err)
		require.Equal(t, body, stripped)
	}
}

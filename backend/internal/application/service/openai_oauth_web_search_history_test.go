package service

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const openAIWebSearchHistoryFixture = `{"model":"gpt-6-astra","input":[{"type":"message","role":"user","content":"look it up"},{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"q"}},{"type":"message","role":"assistant","content":"found"},{"type":"compaction_trigger"}],"tools":[],"tool_choice":"auto"}`

func TestEnsureOpenAIOAuthWebSearchToolForHistory_StandardAndLite(t *testing.T) {
	standard, changed := ensureWebSearchHistoryMap(t, openAIWebSearchHistoryFixture, false)
	require.True(t, changed)
	standardTools, ok := standard["tools"].([]any)
	require.True(t, ok)
	require.Len(t, standardTools, 1)
	require.Equal(t, "web_search", webSearchValue[map[string]any](t, standardTools[0])["type"])
	require.Equal(t, "none", standard["tool_choice"])

	lite, changed := ensureWebSearchHistoryMap(t, openAIWebSearchHistoryFixture, true)
	require.True(t, changed)
	require.Empty(t, lite["tools"])
	liteInput, ok := lite["input"].([]any)
	require.True(t, ok)
	require.Len(t, liteInput, 5)
	require.Equal(t, "additional_tools", webSearchValue[map[string]any](t, liteInput[3])["type"])
	require.Equal(t, "compaction_trigger", webSearchValue[map[string]any](t, liteInput[4])["type"])
	require.Equal(t, "web_search", webSearchValue[map[string]any](t, webSearchValue[[]any](t, webSearchValue[map[string]any](t, liteInput[3])["tools"])[0])["type"])
	require.Equal(t, "none", lite["tool_choice"])
}

func TestEnsureOpenAIOAuthWebSearchToolForHistory_RawMatchesMap(t *testing.T) {
	for _, lite := range []bool{false, true} {
		var want map[string]any
		require.NoError(t, json.Unmarshal([]byte(openAIWebSearchHistoryFixture), &want))
		require.True(t, ensureOpenAIOAuthWebSearchToolForHistory(want, lite))

		got, changed, err := ensureOpenAIOAuthWebSearchToolForHistoryBody([]byte(openAIWebSearchHistoryFixture), lite)
		require.NoError(t, err)
		require.True(t, changed)
		wantJSON, err := json.Marshal(want)
		require.NoError(t, err)
		require.JSONEq(t, string(wantJSON), string(got))
	}
}

func TestEnsureOpenAIOAuthWebSearchToolForHistory_NoOpAndChoiceBoundaries(t *testing.T) {
	for _, body := range []string{
		`{"input":[{"type":"web_search_call"}],"tools":[{"type":"web_search_preview"}]}`,
		`{"input":[{"type":"additional_tools","tools":[{"type":"web_search"}]},{"type":"web_search_call"}]}`,
		`{"input":[{"type":"message","content":"web_search_call"}],"tools":[]}`,
	} {
		var req map[string]any
		require.NoError(t, json.Unmarshal([]byte(body), &req))
		require.False(t, ensureOpenAIOAuthWebSearchToolForHistory(req, false))
		out, changed, err := ensureOpenAIOAuthWebSearchToolForHistoryBody([]byte(body), false)
		require.NoError(t, err)
		require.False(t, changed)
		require.JSONEq(t, body, string(out))
	}

	var required map[string]any
	require.NoError(t, json.Unmarshal([]byte(`{"input":[{"type":"web_search_call"}],"tools":[],"tool_choice":"required"}`), &required))
	require.True(t, ensureOpenAIOAuthWebSearchToolForHistory(required, false))
	require.Equal(t, "required", required["tool_choice"])
}

func TestEnsureOpenAIOAuthWebSearchToolForHistory_ToolMapIsolation(t *testing.T) {
	first := map[string]any{"input": []any{map[string]any{"type": "web_search_call"}}}
	second := map[string]any{"input": []any{map[string]any{"type": "web_search_call"}}}
	require.True(t, ensureOpenAIOAuthWebSearchToolForHistory(first, false))
	require.True(t, ensureOpenAIOAuthWebSearchToolForHistory(second, false))
	webSearchValue[map[string]any](t, webSearchValue[[]any](t, first["tools"])[0])["external_web_access"] = true
	require.Equal(t, false, webSearchValue[map[string]any](t, webSearchValue[[]any](t, second["tools"])[0])["external_web_access"])
	require.Equal(t, false, openAIWebSearchHistoryTool["external_web_access"])
}

func TestApplyCodexOAuthTransform_DeclaresWebSearchHistoryTool(t *testing.T) {
	var req map[string]any
	require.NoError(t, json.Unmarshal([]byte(openAIWebSearchHistoryFixture), &req))
	result := applyCodexOAuthTransformWithOptions(req, codexOAuthTransformOptions{
		IsCodexCLI:    true,
		ResponsesLite: true,
	})
	require.True(t, result.Modified)
	require.Equal(t, "none", req["tool_choice"])
	input, ok := req["input"].([]any)
	require.True(t, ok)
	require.Equal(t, "additional_tools", webSearchValue[map[string]any](t, input[len(input)-2])["type"])
	require.Equal(t, "compaction_trigger", webSearchValue[map[string]any](t, input[len(input)-1])["type"])
}

func ensureWebSearchHistoryMap(t *testing.T, body string, lite bool) (map[string]any, bool) {
	t.Helper()
	var req map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &req))
	changed := ensureOpenAIOAuthWebSearchToolForHistory(req, lite)
	return req, changed
}

func TestEnsureOpenAIOAuthWebSearchToolForHistory_RawToolPath(t *testing.T) {
	body := []byte(`{"input":[{"type":"web_search_call"}],"tools":[{"type":"function","name":"echo"}]}`)
	out, changed, err := ensureOpenAIOAuthWebSearchToolForHistoryBody(body, false)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "function", gjson.GetBytes(out, "tools.0.type").String())
	require.Equal(t, "web_search", gjson.GetBytes(out, "tools.1.type").String())
	require.NotEqual(t, string(body), string(out))
}

func webSearchValue[T any](t *testing.T, raw any) T {
	t.Helper()
	value, ok := raw.(T)
	require.True(t, ok)
	return value
}

func TestOAuthWebSearchHistoryGuardCompactAPIKeyAndOtherPlatform(t *testing.T) {
	for _, tc := range []struct {
		account *Account
		compact bool
	}{
		{nil, false}, {&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}, false},
		{&Account{Platform: PlatformGrok, Type: AccountTypeOAuth}, false},
		{&Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}, true},
	} {
		original := []byte(openAIWebSearchHistoryFixture)
		out, changed, err := normalizeOpenAIOAuthWebSearchHistoryForAccount(original, tc.account, tc.compact, true)
		require.NoError(t, err)
		require.False(t, changed)
		require.Equal(t, original, out)
	}
	req := webSearchValue[map[string]any](t, map[string]any{"input": []any{map[string]any{"type": "web_search_call"}}, "tools": []any{}})
	applyCodexOAuthTransformWithOptions(req, codexOAuthTransformOptions{IsCompact: true, ResponsesLite: true})
	require.Empty(t, req["tools"])
}

func TestOAuthWebSearchHistoryRawPreservesNonTargetBytes(t *testing.T) {
	for _, history := range []string{
		`[ { "type" : "web_search_call", "id":"ws_1" }, {"role":"user","content":"é\\n"}, { "type" : "compaction_trigger" } ]`,
		`[ { "type" : "web_search_call", "id":"ws_1" } ]`,
	} {
		body := []byte(`{ "number":900719925474099312345, "ext":1.20e-2, "input":` + history + `, "tools":[], "tool_choice":"none" }`)
		original := bytes.Clone(body)
		out, changed, err := ensureOpenAIOAuthWebSearchToolForHistoryBody(body, true)
		require.NoError(t, err)
		require.True(t, changed)
		require.True(t, gjson.ValidBytes(out))
		require.Equal(t, original, body)
		require.Contains(t, string(out), `"number":900719925474099312345, "ext":1.20e-2`)
		require.Contains(t, string(out), `{ "type" : "web_search_call", "id":"ws_1" }`)
		if strings.Contains(history, "compaction_trigger") {
			items := gjson.GetBytes(out, "input").Array()
			require.Equal(t, "compaction_trigger", items[len(items)-1].Get("type").String())
		}
	}
}

func TestEnsureOpenAIOAuthWebSearchToolForHistory(t *testing.T) {
	const (
		user    = `{"type":"message","role":"user","content":[{"type":"input_text","text":"q"}]}`
		search  = `{"type":"web_search_call","id":"ws_1","status":"completed","action":{"type":"search","query":"q"}}`
		trigger = `{"type":"compaction_trigger"}`
		history = `[` + user + `,` + search + `]`
	)
	tests := []struct {
		name       string
		body       string
		lite       bool
		changed    bool
		toolsPath  string
		toolCount  int
		toolChoice string
		itemTypes  []string
	}{
		{"tools absent", `{"input":` + history + `}`, false, true, "tools", 1, "none", nil},
		{"tools null", `{"input":` + history + `,"tools":null,"tool_choice":null}`, false, true, "tools", 1, "none", nil},
		{"explicit none kept", `{"input":` + history + `,"tools":[],"tool_choice":"none"}`, false, true, "tools", 1, "none", nil},
		{"required not rewritten", `{"input":` + history + `,"tools":[],"tool_choice":"required"}`, false, true, "tools", 1, "required", nil},
		{"caller tools keep choice", `{"input":` + history + `,"tools":[{"type":"function","name":"echo"}],"tool_choice":"auto"}`, false, true, "tools", 2, "auto", nil},
		{"web_search already declared", `{"input":` + history + `,"tools":[{"type":"web_search"}]}`, false, false, "", 0, "", nil},
		{"web_search_preview declared", `{"input":` + history + `,"tools":[{"type":"web_search_preview"}]}`, false, false, "", 0, "", nil},
		{"declared via additional_tools", `{"input":[{"type":"additional_tools","tools":[{"type":"web_search"}]},` + search + `]}`, false, false, "", 0, "", nil},
		{"no web_search_call item", `{"input":[{"type":"message","role":"user","content":"web_search_call"}],"tools":[]}`, false, false, "", 0, "", nil},
		{"string input", `{"input":"web_search_call"}`, false, false, "", 0, "", nil},
		{"lite inserts before compaction trigger", `{"input":[` + user + `,` + search + `,` + trigger + `]}`, true, true, "input.2.tools", 1, "none",
			[]string{"message", "web_search_call", "additional_tools", "compaction_trigger"}},
		{"lite appends without trigger", `{"input":` + history + `,"tool_choice":"auto"}`, true, true, "input.2.tools", 1, "none",
			[]string{"message", "web_search_call", "additional_tools"}},
		{"lite extends caller additional_tools", `{"input":[{"type":"additional_tools","role":"developer","tools":[{"type":"function","name":"echo"}]},` + search + `,` + trigger + `],"tool_choice":"auto"}`, true, true, "input.0.tools", 2, "auto",
			[]string{"additional_tools", "web_search_call", "compaction_trigger"}},
		{"lite fills empty additional_tools", `{"input":[{"type":"additional_tools","role":"developer"},` + search + `]}`, true, true, "input.0.tools", 1, "none",
			[]string{"additional_tools", "web_search_call"}},
		{"lite keeps top-level tools untouched", `{"input":` + history + `,"tools":[{"type":"function","name":"echo"}]}`, true, true, "input.2.tools", 1, "",
			[]string{"message", "web_search_call", "additional_tools"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, changed, err := ensureOpenAIOAuthWebSearchToolForHistoryBody([]byte(tt.body), tt.lite)
			require.NoError(t, err)
			require.Equal(t, tt.changed, changed)
			var req map[string]any
			require.NoError(t, json.Unmarshal([]byte(tt.body), &req))
			require.Equal(t, tt.changed, ensureOpenAIOAuthWebSearchToolForHistory(req, tt.lite))
			if !tt.changed {
				require.Equal(t, tt.body, string(out))
				return
			}
			require.True(t, gjson.ValidBytes(out))
			tools := gjson.GetBytes(out, tt.toolsPath).Array()
			require.Len(t, tools, tt.toolCount)
			require.Equal(t, "web_search", tools[len(tools)-1].Get("type").String())
			require.Equal(t, tt.toolChoice, gjson.GetBytes(out, "tool_choice").String())
			if tt.lite {
				require.False(t, gjsonToolsContainWebSearch(gjson.GetBytes(out, "tools")), "Lite rejects top-level hosted tools")
				var types []string
				for _, item := range gjson.GetBytes(out, "input").Array() {
					types = append(types, item.Get("type").String())
				}
				require.Equal(t, tt.itemTypes, types)
			}

			// The map variant used by the Codex transform must agree.
			mapOut, err := json.Marshal(req)
			require.NoError(t, err)
			require.JSONEq(t, string(out), string(mapOut))
		})
	}
}

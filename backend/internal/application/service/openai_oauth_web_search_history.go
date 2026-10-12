package service

import (
	"bytes"
	"fmt"
	"strings"
	"unsafe"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// The ChatGPT internal Codex endpoint rejects a request whose input replays a
// hosted web_search_call item unless the request also declares the web_search
// tool; the stream then fails with "response protection is unavailable".
// Codex's local context compaction sends the full history with tools:[], so
// every compaction after a web search fails (#7927).
//
// Declare a cached-only web_search tool for such requests. Responses Lite
// rejects hosted tools at the top level, so Lite requests carry it in an
// input additional_tools item instead. When the caller declared no tools at
// all, also pin tool_choice to "none" so the injected tool cannot be invoked
// and the request keeps its no-tools semantics.

const (
	openAIWebSearchCallItemType      = "web_search_call"
	openAIAdditionalToolsItemType    = "additional_tools"
	openAICompactionTriggerItemType  = "compaction_trigger"
	openAIAdditionalToolsDefaultRole = "developer"
)

var openAIWebSearchHistoryTool = map[string]any{
	"type":                "web_search",
	"external_web_access": false,
}

func isOpenAIWebSearchToolType(toolType string) bool {
	return strings.HasPrefix(strings.TrimSpace(toolType), "web_search")
}

func openAIToolsContainWebSearch(rawTools any) bool {
	tools, ok := rawTools.([]any)
	if !ok {
		return false
	}
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if ok && isOpenAIWebSearchToolType(firstNonEmptyString(tool["type"])) {
			return true
		}
	}
	return false
}

// shouldPinOpenAIWebSearchHistoryToolChoice reports whether tool_choice may be
// forced to "none": only when the caller offered no tools, so the choice
// cannot meaningfully be anything else.
func shouldPinOpenAIWebSearchHistoryToolChoice(choice any) bool {
	switch typed := choice.(type) {
	case nil:
		return true
	case string:
		normalized := strings.ToLower(strings.TrimSpace(typed))
		return normalized == "" || normalized == "auto" || normalized == "none"
	default:
		return false
	}
}

// ensureOpenAIOAuthWebSearchToolForHistory is the map variant used by the
// Codex OAuth transform.
func ensureOpenAIOAuthWebSearchToolForHistory(reqBody map[string]any, responsesLite bool) bool {
	if reqBody == nil {
		return false
	}
	input, ok := reqBody["input"].([]any)
	if !ok || openAIToolsContainWebSearch(reqBody["tools"]) {
		return false
	}
	hasWebSearchCall := false
	callerDeclaredTools := false
	additionalToolsIndex := -1
	lastType := ""
	for i, rawItem := range input {
		lastType = ""
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		lastType = strings.TrimSpace(firstNonEmptyString(item["type"]))
		switch lastType {
		case openAIWebSearchCallItemType:
			hasWebSearchCall = true
		case openAIAdditionalToolsItemType:
			if openAIToolsContainWebSearch(item["tools"]) {
				return false
			}
			if tools, _ := item["tools"].([]any); len(tools) > 0 {
				callerDeclaredTools = true
			}
			if additionalToolsIndex < 0 {
				additionalToolsIndex = i
			}
		}
	}
	if !hasWebSearchCall || openAIToolsContainWebSearch(reqBody["tools"]) {
		return false
	}
	tools, _ := reqBody["tools"].([]any)
	callerDeclaredTools = callerDeclaredTools || len(tools) > 0

	switch {
	case !responsesLite:
		reqBody["tools"] = append(tools, cloneOpenAIWebSearchHistoryTool())
	case additionalToolsIndex >= 0:
		// additionalToolsIndex is only recorded for map items.
		item, _ := input[additionalToolsIndex].(map[string]any)
		existing, _ := item["tools"].([]any)
		item["tools"] = append(existing, cloneOpenAIWebSearchHistoryTool())
	default:
		at := len(input)
		if lastType == openAICompactionTriggerItemType {
			at--
		}
		additional := map[string]any{
			"type":  openAIAdditionalToolsItemType,
			"role":  openAIAdditionalToolsDefaultRole,
			"tools": []any{cloneOpenAIWebSearchHistoryTool()},
		}
		next := make([]any, 0, len(input)+1)
		next = append(next, input[:at]...)
		next = append(next, additional)
		next = append(next, input[at:]...)
		reqBody["input"] = next
	}
	if !callerDeclaredTools {
		if choice, exists := reqBody["tool_choice"]; !exists || shouldPinOpenAIWebSearchHistoryToolChoice(choice) {
			reqBody["tool_choice"] = "none"
		}
	}
	return true
}

// ensureOpenAIOAuthWebSearchToolForHistoryBody is the raw-body variant used by
// the passthrough and WebSocket paths; it avoids decoding the whole body.
func ensureOpenAIOAuthWebSearchToolForHistoryBody(body []byte, responsesLite bool) ([]byte, bool, error) {
	if len(body) == 0 || !bytes.Contains(body, []byte(openAIWebSearchCallItemType)) {
		return body, false, nil
	}
	// This view is read-only, scoped to this call, and never retained. Large
	// histories with an existing declaration must not copy the input array.
	view := unsafe.String(unsafe.SliceData(body), len(body))
	tools := gjson.Get(view, "tools")
	if gjsonToolsContainWebSearch(tools) {
		return body, false, nil
	}
	input := gjson.Get(view, "input")
	if !input.IsArray() {
		return body, false, nil
	}
	hasWebSearchCall, alreadyDeclared := false, false
	callerDeclaredTools := gjsonArrayHasItems(tools)
	additionalToolsIndex := -1
	additionalToolsArray := false
	var last gjson.Result
	input.ForEach(func(index, item gjson.Result) bool {
		last = item
		switch strings.TrimSpace(item.Get("type").String()) {
		case openAIWebSearchCallItemType:
			hasWebSearchCall = true
		case openAIAdditionalToolsItemType:
			itemTools := item.Get("tools")
			if gjsonToolsContainWebSearch(itemTools) {
				alreadyDeclared = true
				return false
			}
			callerDeclaredTools = callerDeclaredTools || gjsonArrayHasItems(itemTools)
			if additionalToolsIndex < 0 {
				additionalToolsIndex = int(index.Int())
				additionalToolsArray = itemTools.IsArray()
			}
		}
		return true
	})
	if !hasWebSearchCall || alreadyDeclared {
		return body, false, nil
	}
	var next []byte
	var err error
	switch {
	case !responsesLite && gjsonArrayHasItems(tools):
		next, err = sjson.SetBytes(body, "tools.-1", cloneOpenAIWebSearchHistoryTool())
	case !responsesLite:
		next, err = sjson.SetBytes(body, "tools", []any{cloneOpenAIWebSearchHistoryTool()})
	case additionalToolsIndex >= 0 && additionalToolsArray:
		next, err = sjson.SetBytes(body, fmt.Sprintf("input.%d.tools.-1", additionalToolsIndex), cloneOpenAIWebSearchHistoryTool())
	case additionalToolsIndex >= 0:
		next, err = sjson.SetBytes(body, fmt.Sprintf("input.%d.tools", additionalToolsIndex), []any{cloneOpenAIWebSearchHistoryTool()})
	default:
		// Insert one item without rebuilding or reserializing the history.
		// ForEach offsets are absolute in the request body. Preserve whitespace,
		// number spelling, escaped strings, and a final compaction trigger.
		additional := []byte(`{"type":"additional_tools","role":"developer","tools":[{"type":"web_search","external_web_access":false}]}`)
		at := input.Index + len(input.Raw) - 1
		if strings.TrimSpace(last.Get("type").String()) == openAICompactionTriggerItemType {
			at = last.Index
			additional = append(additional, ',')
		} else {
			additional = append([]byte{','}, additional...)
		}
		next = make([]byte, 0, len(body)+len(additional))
		next = append(next, body[:at]...)
		next = append(next, additional...)
		next = append(next, body[at:]...)
	}
	if err != nil {
		return body, false, fmt.Errorf("declare web_search tool for web_search_call history: %w", err)
	}
	if !callerDeclaredTools {
		choice := gjson.GetBytes(next, "tool_choice")
		if !choice.Exists() || choice.Type == gjson.Null || (choice.Type == gjson.String && shouldPinOpenAIWebSearchHistoryToolChoice(choice.String())) {
			next, err = sjson.SetBytes(next, "tool_choice", "none")
			if err != nil {
				return body, false, fmt.Errorf("pin tool_choice for web_search_call history: %w", err)
			}
		}
	}
	return next, true, nil
}

func gjsonArrayHasItems(value gjson.Result) bool {
	found := false
	if value.IsArray() {
		value.ForEach(func(_, _ gjson.Result) bool { found = true; return false })
	}
	return found
}

func gjsonToolsContainWebSearch(tools gjson.Result) bool {
	if !tools.IsArray() {
		return false
	}
	found := false
	tools.ForEach(func(_, tool gjson.Result) bool {
		if isOpenAIWebSearchToolType(tool.Get("type").String()) {
			found = true
			return false
		}
		return true
	})
	return found
}

// normalizeOpenAIOAuthWebSearchHistoryForAccount is the common raw HTTP/WS
// guard. API-key, other-platform, and unary compact payloads stay byte-exact.
func normalizeOpenAIOAuthWebSearchHistoryForAccount(body []byte, account *Account, compact, responsesLite bool) ([]byte, bool, error) {
	if account == nil || !account.IsOpenAIOAuth() || compact {
		return body, false, nil
	}
	return ensureOpenAIOAuthWebSearchToolForHistoryBody(body, responsesLite)
}

func cloneOpenAIWebSearchHistoryTool() map[string]any {
	tool := make(map[string]any, len(openAIWebSearchHistoryTool))
	for key, value := range openAIWebSearchHistoryTool {
		tool[key] = value
	}
	return tool
}

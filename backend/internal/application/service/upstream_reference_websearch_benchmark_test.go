//go:build benchmark

package service

import (
	"bytes"
	"fmt"
	"strings"

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
	upstreamReferenceOpenAIWebSearchCallItemType      = "web_search_call"
	upstreamReferenceOpenAIAdditionalToolsItemType    = "additional_tools"
	upstreamReferenceOpenAICompactionTriggerItemType  = "compaction_trigger"
	upstreamReferenceOpenAIAdditionalToolsDefaultRole = "developer"
)

var upstreamReferenceOpenAIWebSearchHistoryTool = map[string]any{
	"type":                "web_search",
	"external_web_access": false,
}

func upstreamReferenceIsOpenAIWebSearchToolType(toolType string) bool {
	return strings.HasPrefix(strings.TrimSpace(toolType), "web_search")
}

func upstreamReferenceOpenAIToolsContainWebSearch(rawTools any) bool {
	tools, ok := rawTools.([]any)
	if !ok {
		return false
	}
	for _, rawTool := range tools {
		tool, ok := rawTool.(map[string]any)
		if ok && upstreamReferenceIsOpenAIWebSearchToolType(firstNonEmptyString(tool["type"])) {
			return true
		}
	}
	return false
}

// upstreamReferenceShouldPinOpenAIWebSearchHistoryToolChoice reports whether tool_choice may be
// forced to "none": only when the caller offered no tools, so the choice
// cannot meaningfully be anything else.
func upstreamReferenceShouldPinOpenAIWebSearchHistoryToolChoice(choice any) bool {
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

// upstreamReferenceOpenAIAdditionalToolsInsertIndex keeps a trailing compaction trigger last,
// as required by the remote compaction v2 wire format.
func upstreamReferenceOpenAIAdditionalToolsInsertIndex(itemTypes []string) int {
	if n := len(itemTypes); n > 0 && itemTypes[n-1] == upstreamReferenceOpenAICompactionTriggerItemType {
		return n - 1
	}
	return len(itemTypes)
}

// upstreamReferenceEnsureOpenAIOAuthWebSearchToolForHistory is the map variant used by the
// Codex OAuth transform.
func upstreamReferenceEnsureOpenAIOAuthWebSearchToolForHistory(reqBody map[string]any, responsesLite bool) bool {
	if reqBody == nil {
		return false
	}
	input, ok := reqBody["input"].([]any)
	if !ok {
		return false
	}
	hasWebSearchCall := false
	callerDeclaredTools := false
	additionalToolsIndex := -1
	itemTypes := make([]string, len(input))
	for i, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		itemTypes[i] = strings.TrimSpace(firstNonEmptyString(item["type"]))
		switch itemTypes[i] {
		case upstreamReferenceOpenAIWebSearchCallItemType:
			hasWebSearchCall = true
		case upstreamReferenceOpenAIAdditionalToolsItemType:
			if upstreamReferenceOpenAIToolsContainWebSearch(item["tools"]) {
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
	if !hasWebSearchCall || upstreamReferenceOpenAIToolsContainWebSearch(reqBody["tools"]) {
		return false
	}
	tools, _ := reqBody["tools"].([]any)
	callerDeclaredTools = callerDeclaredTools || len(tools) > 0

	switch {
	case !responsesLite:
		reqBody["tools"] = append(tools, upstreamReferenceCloneOpenAIWebSearchHistoryTool())
	case additionalToolsIndex >= 0:
		// additionalToolsIndex is only recorded for map items.
		item, _ := input[additionalToolsIndex].(map[string]any)
		existing, _ := item["tools"].([]any)
		item["tools"] = append(existing, upstreamReferenceCloneOpenAIWebSearchHistoryTool())
	default:
		at := upstreamReferenceOpenAIAdditionalToolsInsertIndex(itemTypes)
		additional := map[string]any{
			"type":  upstreamReferenceOpenAIAdditionalToolsItemType,
			"role":  upstreamReferenceOpenAIAdditionalToolsDefaultRole,
			"tools": []any{upstreamReferenceCloneOpenAIWebSearchHistoryTool()},
		}
		next := make([]any, 0, len(input)+1)
		next = append(next, input[:at]...)
		next = append(next, additional)
		next = append(next, input[at:]...)
		reqBody["input"] = next
	}
	if !callerDeclaredTools {
		if choice, exists := reqBody["tool_choice"]; !exists || upstreamReferenceShouldPinOpenAIWebSearchHistoryToolChoice(choice) {
			reqBody["tool_choice"] = "none"
		}
	}
	return true
}

// upstreamReferenceEnsureOpenAIOAuthWebSearchToolForHistoryBody is the raw-body variant used by
// the passthrough and WebSocket paths; it avoids decoding the whole body.
func upstreamReferenceEnsureOpenAIOAuthWebSearchToolForHistoryBody(body []byte, responsesLite bool) ([]byte, bool, error) {
	if len(body) == 0 || !bytes.Contains(body, []byte(upstreamReferenceOpenAIWebSearchCallItemType)) {
		return body, false, nil
	}
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return body, false, nil
	}
	items := input.Array()
	hasWebSearchCall := false
	callerDeclaredTools := false
	additionalToolsIndex := -1
	itemTypes := make([]string, len(items))
	for i, item := range items {
		itemTypes[i] = strings.TrimSpace(item.Get("type").String())
		switch itemTypes[i] {
		case upstreamReferenceOpenAIWebSearchCallItemType:
			hasWebSearchCall = true
		case upstreamReferenceOpenAIAdditionalToolsItemType:
			itemTools := item.Get("tools")
			if upstreamReferenceGjsonToolsContainWebSearch(itemTools) {
				return body, false, nil
			}
			if itemTools.IsArray() && len(itemTools.Array()) > 0 {
				callerDeclaredTools = true
			}
			if additionalToolsIndex < 0 {
				additionalToolsIndex = i
			}
		}
	}
	if !hasWebSearchCall {
		return body, false, nil
	}
	tools := gjson.GetBytes(body, "tools")
	if upstreamReferenceGjsonToolsContainWebSearch(tools) {
		return body, false, nil
	}
	topLevelTools := tools.IsArray() && len(tools.Array()) > 0
	callerDeclaredTools = callerDeclaredTools || topLevelTools

	var (
		next []byte
		err  error
	)
	switch {
	case !responsesLite && topLevelTools:
		next, err = sjson.SetBytes(body, "tools.-1", upstreamReferenceCloneOpenAIWebSearchHistoryTool())
	case !responsesLite:
		next, err = sjson.SetBytes(body, "tools", []any{upstreamReferenceCloneOpenAIWebSearchHistoryTool()})
	case additionalToolsIndex >= 0 && items[additionalToolsIndex].Get("tools").IsArray():
		next, err = sjson.SetBytes(body, fmt.Sprintf("input.%d.tools.-1", additionalToolsIndex), upstreamReferenceCloneOpenAIWebSearchHistoryTool())
	case additionalToolsIndex >= 0:
		next, err = sjson.SetBytes(body, fmt.Sprintf("input.%d.tools", additionalToolsIndex), []any{upstreamReferenceCloneOpenAIWebSearchHistoryTool()})
	default:
		next, err = upstreamReferenceInsertOpenAIAdditionalToolsItemRaw(body, items, upstreamReferenceOpenAIAdditionalToolsInsertIndex(itemTypes))
	}
	if err != nil {
		return body, false, fmt.Errorf("declare web_search tool for web_search_call history: %w", err)
	}
	if !callerDeclaredTools {
		choice := gjson.GetBytes(next, "tool_choice")
		if !choice.Exists() || choice.Type == gjson.Null || (choice.Type == gjson.String && upstreamReferenceShouldPinOpenAIWebSearchHistoryToolChoice(choice.String())) {
			next, err = sjson.SetBytes(next, "tool_choice", "none")
			if err != nil {
				return body, false, fmt.Errorf("pin tool_choice for web_search_call history: %w", err)
			}
		}
	}
	return next, true, nil
}

func upstreamReferenceInsertOpenAIAdditionalToolsItemRaw(body []byte, items []gjson.Result, at int) ([]byte, error) {
	additional, err := marshalOpenAIUpstreamJSON(map[string]any{
		"type":  upstreamReferenceOpenAIAdditionalToolsItemType,
		"role":  upstreamReferenceOpenAIAdditionalToolsDefaultRole,
		"tools": []any{upstreamReferenceCloneOpenAIWebSearchHistoryTool()},
	})
	if err != nil {
		return nil, err
	}
	rawItems := make([][]byte, 0, len(items)+1)
	for i, item := range items {
		if i == at {
			rawItems = append(rawItems, additional)
		}
		rawItems = append(rawItems, []byte(item.Raw))
	}
	if at >= len(items) {
		rawItems = append(rawItems, additional)
	}
	raw := append([]byte{'['}, bytes.Join(rawItems, []byte{','})...)
	raw = append(raw, ']')
	return sjson.SetRawBytes(body, "input", raw)
}

func upstreamReferenceGjsonToolsContainWebSearch(tools gjson.Result) bool {
	if !tools.IsArray() {
		return false
	}
	for _, tool := range tools.Array() {
		if upstreamReferenceIsOpenAIWebSearchToolType(tool.Get("type").String()) {
			return true
		}
	}
	return false
}

func upstreamReferenceCloneOpenAIWebSearchHistoryTool() map[string]any {
	tool := make(map[string]any, len(upstreamReferenceOpenAIWebSearchHistoryTool))
	for key, value := range upstreamReferenceOpenAIWebSearchHistoryTool {
		tool[key] = value
	}
	return tool
}

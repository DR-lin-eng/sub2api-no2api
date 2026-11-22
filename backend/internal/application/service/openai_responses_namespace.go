package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const openAIResponsesNamespaceNamesContextKey = "openai_responses_namespace_names"

// shouldFlattenOpenAIResponsesNamespaces determines whether Codex namespace
// tools need the legacy flattened representation. Native OpenAI OAuth
// Responses preserves namespaces by default; compact requests and explicitly
// configured compatibility upstreams retain the established flattening path.
func shouldFlattenOpenAIResponsesNamespaces(
	account *Account,
	transport OpenAIUpstreamTransport,
	passthroughEnabled bool,
	compactPath bool,
) bool {
	if account == nil || !account.IsOpenAIOAuth() {
		return false
	}
	if !compactPath && !account.IsOpenAIResponsesFlattenNamespacesEnabled() {
		return false
	}
	if transport == OpenAIUpstreamTransportResponsesWebsocketV2 && !passthroughEnabled {
		return false
	}
	return true
}

func shouldKeepOpenAIResponsesToolCallNamespaces(
	account *Account,
	transport OpenAIUpstreamTransport,
	passthroughEnabled bool,
	compactPath bool,
	c *gin.Context,
	body []byte,
) bool {
	if account == nil || compactPath {
		return false
	}
	if account.IsOpenAIApiKey() {
		return hasOpenAIResponsesNamespaceToolDeclaration(c, body)
	}
	if !account.IsOpenAIOAuth() {
		return false
	}
	return !shouldFlattenOpenAIResponsesNamespaces(account, transport, passthroughEnabled, compactPath)
}

func hasOpenAIResponsesNamespaceToolDeclaration(c *gin.Context, body []byte) bool {
	hasNamespaceTool := func(tools gjson.Result) bool {
		if !tools.IsArray() {
			return false
		}
		found := false
		tools.ForEach(func(_, tool gjson.Result) bool {
			if strings.EqualFold(strings.TrimSpace(tool.Get("type").String()), "namespace") {
				found = true
				return false
			}
			return true
		})
		return found
	}

	if hasNamespaceTool(openAIRequestBodyFieldView(c, body, "tools")) {
		return true
	}
	if !openAIRequestBodyInputItemTypeMatches(c, body, func(itemType string) bool {
		return strings.EqualFold(strings.TrimSpace(itemType), "additional_tools")
	}) {
		return false
	}

	// Responses Lite carries namespace declarations in input.additional_tools.
	// The view is read-only and stays inside this call. It avoids copying the
	// full input array before deciding whether any namespace needs deletion.
	input := openAIRequestBodyFieldView(c, body, "input")
	if !input.IsArray() {
		return false
	}
	found := false
	input.ForEach(func(_, item gjson.Result) bool {
		if !strings.EqualFold(strings.TrimSpace(item.Get("type").String()), "additional_tools") {
			return true
		}
		if hasNamespaceTool(item.Get("tools")) {
			found = true
			return false
		}
		return true
	})
	return found
}

var openAIResponsesToolCallItemTypes = map[string]bool{
	"function_call":    true,
	"tool_call":        true,
	"custom_tool_call": true,
	"mcp_tool_call":    true,
}

func isOpenAIResponsesToolCallItemType(itemType string) bool {
	return openAIResponsesToolCallItemTypes[strings.ToLower(strings.TrimSpace(itemType))]
}

// shouldStripOpenAIResponsesInputNamespaces removes residual input item
// namespaces for OpenAI OAuth and API Key HTTP forwarding. Native WSv2 keeps
// namespaces because that protocol supports them and does not restore payloads.
func shouldStripOpenAIResponsesInputNamespaces(account *Account, transport OpenAIUpstreamTransport, passthroughEnabled bool) bool {
	if account == nil || (!account.IsOpenAIOAuth() && !account.IsOpenAIApiKey()) {
		return false
	}
	if transport == OpenAIUpstreamTransportResponsesWebsocketV2 && !passthroughEnabled {
		return false
	}
	return true
}

func flattenOpenAIResponsesNamespaces(c *gin.Context, body []byte) ([]byte, error) {
	if !bytes.Contains(body, []byte(`"namespace"`)) {
		return body, nil
	}
	var requestBody map[string]any
	if err := json.Unmarshal(body, &requestBody); err != nil {
		return body, fmt.Errorf("decode OpenAI namespace body: %w", err)
	}
	names, changed, err := apicompat.FlattenResponsesNamespacesExcept(requestBody, map[string]bool{"image_gen": true})
	if err != nil {
		return body, err
	}
	if !changed {
		return body, nil
	}
	rebuilt, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return body, fmt.Errorf("encode OpenAI namespace body: %w", err)
	}
	setOpenAIResponsesNamespaceNames(c, names)
	return rebuilt, nil
}

// stripOpenAIResponsesInputNamespaces removes namespace only from direct input
// array items. Namespace declarations and nested namespace fields are left
// untouched. Rebuilding the input array once keeps this linear for long
// histories and avoids decoding JSON numbers through float64.
func stripOpenAIResponsesInputNamespaces(body []byte, keepToolCallNamespaces bool) ([]byte, error) {
	if !bytes.Contains(body, []byte(`"namespace"`)) {
		return body, nil
	}
	input := parseRawJSONView(body).Get("input")
	if !input.IsArray() {
		return body, nil
	}
	// inputStart/inputEnd 是 input 数组在 body 里的字节区间。
	inputStart := input.Index
	inputEnd := inputStart + len(input.Raw)

	// 输出缓冲只在首次真正改写时分配，容量按整个 body 预留，prefix + 改写后的数组 +
	// suffix 一次写完，不再额外走一遍 sjson.SetRawBytes（那会再拷一份完整 body）。
	var out []byte
	// Offsets below are relative to input.Raw, not the full request body.
	copyFrom := 0
	var stripErr error
	input.ForEach(func(_, item gjson.Result) bool {
		if !item.IsObject() || !item.Get("namespace").Exists() ||
			(keepToolCallNamespaces && isOpenAIResponsesToolCallItemType(item.Get("type").String())) {
			return true
		}
		itemBody, err := sjson.Delete(item.Raw, "namespace")
		if err != nil {
			stripErr = err
			return false
		}
		if out == nil {
			out = make([]byte, 0, len(body))
			out = append(out, body[:inputStart]...)
		}
		itemStart := item.Index - input.Index
		out = append(out, input.Raw[copyFrom:itemStart]...)
		out = append(out, itemBody...)
		copyFrom = itemStart + len(item.Raw)
		return true
	})
	if stripErr != nil {
		return body, fmt.Errorf("delete OpenAI input namespace: %w", stripErr)
	}
	if out == nil {
		return body, nil
	}
	out = append(out, input.Raw[copyFrom:]...)
	out = append(out, body[inputEnd:]...)
	return out, nil
}

func clearOpenAIResponsesNamespaceNames(c *gin.Context) {
	if c == nil {
		return
	}
	if _, exists := c.Get(openAIResponsesNamespaceNamesContextKey); exists {
		c.Set(openAIResponsesNamespaceNamesContextKey, map[string]apicompat.ResponsesNamespaceName(nil))
	}
}

func setOpenAIResponsesNamespaceNames(c *gin.Context, names map[string]apicompat.ResponsesNamespaceName) {
	if c != nil && len(names) > 0 {
		c.Set(openAIResponsesNamespaceNamesContextKey, names)
	}
}

func openAIResponsesNamespaceNames(c *gin.Context) map[string]apicompat.ResponsesNamespaceName {
	if c == nil {
		return nil
	}
	value, ok := c.Get(openAIResponsesNamespaceNamesContextKey)
	if !ok {
		return nil
	}
	names, _ := value.(map[string]apicompat.ResponsesNamespaceName)
	return names
}

func restoreOpenAIResponsesNamespacePayload(c *gin.Context, payload []byte) ([]byte, error) {
	names := openAIResponsesNamespaceNames(c)
	if len(names) == 0 || !json.Valid(payload) {
		return payload, nil
	}
	restored, changed, err := apicompat.RestoreResponsesNamespaceCalls(payload, names)
	if err != nil {
		return payload, err
	}
	if changed {
		return restored, nil
	}
	return payload, nil
}

package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// isDeepSeekModelName identifies DeepSeek semantics after account/channel model
// mapping. This is intentionally narrow so other OpenAI-compatible providers do
// not inherit DeepSeek's reasoning-history rules.
func isDeepSeekModelName(model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	return strings.HasPrefix(model, "deepseek-") || model == "deepseek"
}

func isDeepSeekAPIHost(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return false
	}
	return strings.EqualFold(parsed.Hostname(), "api.deepseek.com")
}

// isDeepSeekSemanticsChatUpstream covers native DeepSeek accounts and OpenAI
// aggregate accounts whose selected upstream model or base URL is DeepSeek.
func isDeepSeekSemanticsChatUpstream(account *Account, upstreamModel string) bool {
	if account == nil {
		return false
	}
	return account.Platform == PlatformDeepseek ||
		isDeepSeekAPIHost(account.GetOpenAIBaseURL()) ||
		isDeepSeekModelName(upstreamModel)
}

// DeepSeek's native Responses endpoint acknowledges input[].additional_tools
// but silently ignores the declarations. Route that specific Lite shape through
// the existing Responses -> Chat bridge, which promotes them to Chat tools.
func shouldForwardDeepSeekResponsesLiteViaChatCompletions(account *Account, body []byte) bool {
	if !deepSeekNativeResponsesCandidate(account, body) {
		return false
	}
	for _, item := range gjson.GetBytes(body, "input").Array() {
		if strings.TrimSpace(item.Get("type").String()) == "additional_tools" && len(item.Get("tools").Array()) > 0 {
			return true
		}
	}
	return false
}

// A native compaction trigger has no DeepSeek Responses equivalent. The Chat
// bridge summarizes the turn and wraps it in a Responses compaction item.
func shouldForwardDeepSeekResponsesCompactViaChatCompletions(account *Account, body []byte) bool {
	return deepSeekNativeResponsesCandidate(account, body) && HasCompactionTriggerInInput(body)
}

func deepSeekNativeResponsesCandidate(account *Account, body []byte) bool {
	if account == nil || account.Type != AccountTypeAPIKey || shouldForwardOpenAIResponsesViaRawChatCompletions(account) {
		return false
	}
	requestedModel := gjson.GetBytes(body, "model").String()
	return isDeepSeekSemanticsChatUpstream(account, account.GetMappedModel(requestedModel))
}

func shouldForwardDeepSeekCompatViaChatCompletions(c *gin.Context, account *Account, body []byte) bool {
	return shouldForwardDeepSeekResponsesLiteViaChatCompletions(account, body) ||
		(isOpenAINativeCompactionV2(c) && shouldForwardDeepSeekResponsesCompactViaChatCompletions(account, body))
}

// deepSeekChatReasoningPlaceholderText is the smallest non-empty value accepted
// by DeepSeek thinking mode when a historical reasoning item has no plaintext.
const deepSeekChatReasoningPlaceholderText = " "

// ensureDeepSeekChatReasoningPlaceholders fills missing assistant reasoning
// fields before a Responses -> Chat Completions request is sent. Existing
// reasoning text is never overwritten and non-DeepSeek requests remain byte
// identical.
func stripDeepSeekUnsupportedChatResponseFormat(account *Account, body []byte) []byte {
	if !isDeepSeekSemanticsChatUpstream(account, gjson.GetBytes(body, "model").String()) {
		return body
	}
	if strings.TrimSpace(gjson.GetBytes(body, "response_format.type").String()) != "json_schema" {
		return body
	}
	patched, err := sjson.DeleteBytes(body, "response_format")
	if err != nil {
		return body
	}
	return patched
}

func ensureDeepSeekChatReasoningPlaceholders(account *Account, body []byte) []byte {
	if !isDeepSeekSemanticsChatUpstream(account, gjson.GetBytes(body, "model").String()) {
		return body
	}
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
	}
	updated := body
	changed := false
	for i, message := range messages.Array() {
		if strings.TrimSpace(message.Get("role").String()) != "assistant" || message.Get("reasoning_content").String() != "" {
			continue
		}
		next, err := sjson.SetBytes(updated, "messages."+strconv.Itoa(i)+".reasoning_content", deepSeekChatReasoningPlaceholderText)
		if err != nil {
			return body
		}
		updated = next
		changed = true
	}
	if !changed {
		return body
	}
	return updated
}

// normalizeDeepSeekResponsesRequestBody adapts the stateless native DeepSeek
// Responses endpoint and its image serde. DeepSeek accepts the standard
// Responses shape but requires store=false, no previous_response_id, and an
// image part carrying both image_url and url strings.
func normalizeDeepSeekResponsesRequestBody(account *Account, body []byte) []byte {
	if account == nil {
		return body
	}
	applyStateless := account.UsesNativeCNResponses()
	applyImages := account.Platform == PlatformDeepseek || isDeepSeekAPIHost(account.GetOpenAIBaseURL())
	if !applyStateless && !applyImages {
		return body
	}

	normalized := body
	if applyStateless {
		patched, err := sjson.SetBytes(normalized, "store", false)
		if err != nil {
			return body
		}
		normalized = patched
		if stripped, err := sjson.DeleteBytes(normalized, "previous_response_id"); err == nil {
			normalized = stripped
		}
	}

	requestBody, err := decodeOpenAIJSONMap(normalized)
	if err != nil {
		return normalized
	}
	input, exists := requestBody["input"]
	if !exists {
		return normalized
	}
	changed := false
	if lifted, did := apicompat.LiftResponsesToolOutputMedia(input); did {
		requestBody["input"] = lifted
		input = lifted
		changed = true
	}
	if applyImages {
		if aliased, did := aliasDeepSeekResponsesInputImages(input); did {
			requestBody["input"] = aliased
			changed = true
		}
	}
	if !changed {
		return normalized
	}
	rebuilt, err := marshalOpenAIUpstreamJSON(requestBody)
	if err != nil {
		return normalized
	}
	return rebuilt
}

func aliasDeepSeekResponsesInputImages(input any) (any, bool) {
	items, ok := asDeepSeekResponsesSlice(input)
	if !ok {
		return input, false
	}
	changed := false
	for i, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if aliasDeepSeekResponsesInputItem(item) {
			items[i] = item
			changed = true
		}
	}
	return items, changed
}

func aliasDeepSeekResponsesInputItem(item map[string]any) bool {
	changed := aliasDeepSeekResponsesImagePart(item)
	for _, key := range []string{"content", "output"} {
		if content, exists := item[key]; exists {
			if rewritten, did := aliasDeepSeekResponsesContent(content); did {
				item[key] = rewritten
				changed = true
			}
		}
	}
	return changed
}

func aliasDeepSeekResponsesContent(content any) (any, bool) {
	parts, ok := asDeepSeekResponsesSlice(content)
	if !ok {
		return content, false
	}
	changed := false
	for i, raw := range parts {
		part, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if aliasDeepSeekResponsesImagePart(part) {
			parts[i] = part
			changed = true
		}
	}
	return parts, changed
}

func aliasDeepSeekResponsesImagePart(part map[string]any) bool {
	partType := strings.TrimSpace(stringValue(part["type"]))
	switch partType {
	case "input_image", "image_url", "image":
	default:
		return false
	}
	imageURL := extractDeepSeekResponsesImageURL(part)
	if imageURL == "" {
		return false
	}
	changed := false
	if partType != "input_image" {
		part["type"] = "input_image"
		changed = true
	}
	if current, ok := part["image_url"].(string); !ok || strings.TrimSpace(current) != imageURL {
		part["image_url"] = imageURL
		changed = true
	}
	if current, ok := part["url"].(string); !ok || strings.TrimSpace(current) != imageURL {
		part["url"] = imageURL
		changed = true
	}
	return changed
}

func extractDeepSeekResponsesImageURL(part map[string]any) string {
	for _, key := range []string{"url", "image_url", "image"} {
		if imageURL := deepSeekResponsesURLValue(part[key]); imageURL != "" {
			return imageURL
		}
	}
	source, ok := part["source"].(map[string]any)
	if !ok {
		return ""
	}
	if imageURL := deepSeekResponsesURLValue(source["url"]); imageURL != "" {
		return imageURL
	}
	data := strings.TrimSpace(stringValue(source["data"]))
	if data == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(data), "data:") {
		return data
	}
	mediaType := strings.TrimSpace(stringValue(source["media_type"]))
	if mediaType == "" {
		mediaType = "image/png"
	}
	return "data:" + mediaType + ";base64," + data
}

func deepSeekResponsesURLValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		if url := strings.TrimSpace(stringValue(typed["url"])); url != "" {
			return url
		}
		return strings.TrimSpace(stringValue(typed["image_url"]))
	default:
		return ""
	}
}

func asDeepSeekResponsesSlice(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	default:
		return nil, false
	}
}

func decodeOpenAIJSONUseNumber(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple JSON values")
		}
		return err
	}
	return nil
}

func decodeOpenAIJSONMap(body []byte) (map[string]any, error) {
	var value map[string]any
	if err := decodeOpenAIJSONUseNumber(body, &value); err != nil {
		return nil, fmt.Errorf("decode JSON object: %w", err)
	}
	if value == nil {
		return nil, fmt.Errorf("JSON body is not an object")
	}
	return value, nil
}

// buildDeepSeekCompactChatBody rewrites a native remote-compaction request into
// an ordinary, non-streaming summary turn for the DeepSeek Chat endpoint.
func buildDeepSeekCompactChatBody(body []byte) ([]byte, error) {
	payload, err := decodeOpenAIJSONMap(body)
	if err != nil {
		return nil, fmt.Errorf("decode deepseek compact body: %w", err)
	}
	input, _ := payload["input"].([]any)
	filtered := make([]any, 0, len(input)+1)
	for _, raw := range input {
		if item, ok := raw.(map[string]any); ok && strings.TrimSpace(stringValue(item["type"])) == "compaction_trigger" {
			continue
		}
		filtered = append(filtered, raw)
	}
	filtered = append(filtered, map[string]any{
		"type": "message", "role": "user",
		"content": []any{map[string]any{"type": "input_text", "text": grokCompactSummaryPrompt}},
	})
	payload["input"] = filtered
	payload["stream"] = false
	return marshalOpenAIUpstreamJSON(payload)
}

func compactSummaryTextFromResponses(output []apicompat.ResponsesOutput) string {
	var parts []string
	for _, item := range output {
		if item.Type != "message" {
			continue
		}
		for _, part := range item.Content {
			if text := strings.TrimSpace(part.Text); text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func buildDeepSeekCompactResponse(resp *apicompat.ResponsesResponse, summary string) *apicompat.ResponsesResponse {
	out := *resp
	out.Status = "completed"
	out.Output = []apicompat.ResponsesOutput{{
		Type: "compaction", ID: "cmp_" + strings.ReplaceAll(uuid.NewString(), "-", ""), Status: "completed",
		EncryptedContent: strings.TrimSpace(summary),
		Summary:          []apicompat.ResponsesSummary{{Type: "summary_text", Text: strings.TrimSpace(summary)}},
	}}
	return &out
}

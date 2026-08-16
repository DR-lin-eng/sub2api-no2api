package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const (
	compatCompactSummaryTag     = "conversation_summary"
	compatCompactEnvelopePrefix = "sub2api-compat-compact-v1:"
)

// isCompatCompactionRequest identifies a Codex remote compaction request that
// must receive a compaction output item instead of an ordinary chat response.
func isCompatCompactionRequest(body []byte) bool {
	hasTrigger, _ := inspectCompatCompactionInput(body)
	return hasTrigger
}

// inspectCompatCompactionInput keeps ordinary fallback requests on the cheap
// path: the full map rewrite is only needed for a trigger or replay item.
func inspectCompatCompactionInput(body []byte) (hasTrigger, hasCompactionItem bool) {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() {
		return false, false
	}
	input.ForEach(func(_, item gjson.Result) bool {
		itemType := strings.TrimSpace(item.Get("type").String())
		switch {
		case itemType == "compaction_trigger":
			hasTrigger = true
			hasCompactionItem = true
			return false
		case isOpenAICompactionType(itemType):
			hasCompactionItem = true
		}
		return true
	})
	return hasTrigger, hasCompactionItem
}

// rewriteCompatCompactRequestBody translates Responses-only compaction items
// before the request enters the Chat Completions compatibility bridge. Prior
// compaction items are restored on every subsequent request, not only when the
// client asks for another compaction.
func rewriteCompatCompactRequestBody(body []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode compact-compatible request: %w", err)
	}
	items, ok := payload["input"].([]any)
	if !ok {
		return body, nil
	}

	converted := make([]any, 0, len(items)+1)
	changed := false
	hasTrigger := false
	for _, raw := range items {
		item, ok := raw.(map[string]any)
		if !ok {
			converted = append(converted, raw)
			continue
		}

		itemType := strings.TrimSpace(stringValue(item["type"]))
		switch {
		case itemType == "compaction_trigger":
			changed = true
			hasTrigger = true
			converted = append(converted, compatCompactUserMessage(grokCompactSummaryPrompt))
		case isOpenAICompactionType(itemType):
			changed = true
			summary, err := compatCompactSummaryFromItem(item)
			if err != nil {
				return nil, err
			}
			converted = append(converted, compatCompactUserMessage(
				"<"+compatCompactSummaryTag+">\n"+summary+"\n</"+compatCompactSummaryTag+">",
			))
		default:
			converted = append(converted, raw)
		}
	}
	if !changed {
		return body, nil
	}

	payload["input"] = converted
	if hasTrigger {
		// A compaction turn is one buffered summary operation. The response is
		// reshaped into a single compaction item after the upstream call.
		payload["stream"] = false
		if tools, ok := payload["tools"].([]any); ok && len(tools) > 0 {
			payload["tool_choice"] = "none"
		}
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode compact-compatible request: %w", err)
	}
	return encoded, nil
}

func compatCompactUserMessage(text string) map[string]any {
	return map[string]any{
		"type": "message",
		"role": "user",
		"content": []any{map[string]any{
			"type": "input_text",
			"text": text,
		}},
	}
}

func encodeCompatCompactSummary(summary string) string {
	return compatCompactEnvelopePrefix + base64.RawURLEncoding.EncodeToString([]byte(summary))
}

func decodeCompatCompactSummary(value string) (string, bool, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, compatCompactEnvelopePrefix) {
		return "", false, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(value, compatCompactEnvelopePrefix))
	if err != nil {
		return "", true, fmt.Errorf("decode prior compact summary: %w", err)
	}
	summary := strings.TrimSpace(string(decoded))
	if summary == "" {
		return "", true, fmt.Errorf("prior compact summary is empty")
	}
	return summary, true, nil
}

func compatCompactSummaryFromItem(item map[string]any) (string, error) {
	encrypted := strings.TrimSpace(stringValue(item["encrypted_content"]))
	if summary, recognized, err := decodeCompatCompactSummary(encrypted); recognized {
		if err != nil {
			return "", err
		}
		return summary, nil
	}
	if summary := compactSummaryText(item["summary"]); summary != "" {
		return summary, nil
	}
	if encrypted != "" {
		return "", fmt.Errorf("cannot replay a compaction item produced by another upstream through Chat Completions fallback")
	}
	return "", fmt.Errorf("compaction item carries no replayable summary")
}

// buildCompatCompactResponse reshapes one buffered Chat Completions response
// into the single compaction item required by Codex remote compaction v2. The
// compatibility envelope is opaque to Codex but lets this gateway restore the
// summary after Codex serializes the item back without its optional summary.
func buildCompatCompactResponse(resp *apicompat.ChatCompletionsResponse, model string) (*apicompat.ResponsesResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("compact response is nil")
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("compact response has no choices")
	}
	choice := resp.Choices[0]
	switch strings.TrimSpace(choice.FinishReason) {
	case "length", "content_filter", "tool_calls", "function_call":
		return nil, fmt.Errorf("compact response did not finish with a complete summary: %s", choice.FinishReason)
	}

	summary := strings.TrimSpace(chatMessagePlainText(choice.Message.Content))
	if summary == "" {
		summary = strings.TrimSpace(choice.Message.ReasoningContent)
	}
	if summary == "" {
		return nil, fmt.Errorf("compact response carries no summary text")
	}

	id := strings.TrimSpace(resp.ID)
	if id == "" {
		id = "resp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	createdAt := resp.Created
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}
	if strings.TrimSpace(model) == "" {
		model = resp.Model
	}
	out := &apicompat.ResponsesResponse{
		ID:        id,
		Object:    "response",
		CreatedAt: createdAt,
		Model:     model,
		Status:    "completed",
		Output: []apicompat.ResponsesOutput{{
			Type:             "compaction",
			ID:               "cmp_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
			Status:           "completed",
			EncryptedContent: encodeCompatCompactSummary(summary),
			Summary: []apicompat.ResponsesSummary{{
				Type: "summary_text",
				Text: summary,
			}},
		}},
	}
	if resp.Usage != nil {
		out.Usage = apicompat.ChatUsageToResponsesUsage(resp.Usage)
	}
	return out, nil
}

func chatMessagePlainText(content json.RawMessage) string {
	if len(content) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(content, &text); err == nil {
		return text
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(content, &parts); err != nil {
		return ""
	}
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part.Text); trimmed != "" {
			texts = append(texts, trimmed)
		}
	}
	return strings.Join(texts, "\n")
}

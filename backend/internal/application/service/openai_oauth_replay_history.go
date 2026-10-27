package service

import (
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// normalizeOpenAIOAuthReplayHistoryForAccount prepares replayed output items
// for the ChatGPT Codex input schema. HTTP, compact and every native WS turn
// share the same policy; public API-key and non-OpenAI accounts are untouched.
func normalizeOpenAIOAuthReplayHistoryForAccount(body []byte, account *Account, compact, lite bool) ([]byte, bool, error) {
	if account == nil || !account.IsOpenAIOAuth() {
		return body, false, nil
	}
	normalized, contentChanged, err := stripOpenAIOAuthReasoningContent(body)
	if err != nil {
		return body, false, err
	}
	normalized, toolsChanged, err := normalizeOpenAIOAuthWebSearchHistoryForAccount(normalized, account, compact, lite)
	if err != nil {
		return body, false, err
	}
	return normalized, contentChanged || toolsChanged, nil
}

// Reasoning content is response-only on Codex OAuth: a non-empty array is
// rejected as array_above_max_length (maximum zero). Keep the item, summary,
// encrypted_content and IDs, and leave message/tool content byte-exact.
func stripOpenAIOAuthReasoningContent(body []byte) ([]byte, bool, error) {
	input := parseRawJSONView(body).Get("input")
	needsStrip := func(item gjson.Result) bool {
		if !item.IsObject() || item.Get("type").String() != "reasoning" {
			return false
		}
		content := item.Get("content")
		return content.IsArray() && content.Get("#").Int() > 0
	}
	if !input.IsArray() {
		if !needsStrip(input) {
			return body, false, nil
		}
		normalized, err := sjson.Delete(input.Raw, "content")
		if err != nil {
			return body, false, fmt.Errorf("strip OAuth reasoning content: %w", err)
		}
		return replaceOpenAIRawValue(body, input, normalized), true, nil
	}
	changed := false
	input.ForEach(func(_, item gjson.Result) bool {
		changed = needsStrip(item)
		return !changed
	})
	if !changed {
		return body, false, nil
	}
	// Rebuild input once without decoding other items through float64. Large
	// integer extensions, image data and tool outputs retain their raw bytes.
	items := make([]string, 0, 16)
	var stripErr error
	input.ForEach(func(_, item gjson.Result) bool {
		raw := item.Raw
		if needsStrip(item) {
			raw, stripErr = sjson.Delete(raw, "content")
			if stripErr != nil {
				return false
			}
		}
		items = append(items, raw)
		return true
	})
	if stripErr != nil {
		return body, false, fmt.Errorf("strip OAuth reasoning content: %w", stripErr)
	}
	return replaceOpenAIRawInput(body, input, items), true, nil
}

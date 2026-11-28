package service

import (
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const geminiStudioChatEndpoint = "https://generativelanguage.googleapis.com/v1beta/openai/chat/completions"

// A translated history can have no signature to replay. For the direct Gemini
// Chat endpoint, fill the first call of an unsigned assistant batch and preserve
// every batch that already carries an upstream signature.
func prepareGeminiStudioChatHistory(targetURL string, body []byte) []byte {
	if targetURL != geminiStudioChatEndpoint {
		return body
	}

	type replacement struct {
		start, end int
		value      string
	}
	var replacements []replacement
	size := len(body)
	parseRawJSONView(body).Get("messages").ForEach(func(_, message gjson.Result) bool {
		if message.Get("role").Str != "assistant" {
			return true
		}
		calls := message.Get("tool_calls")
		if !calls.IsArray() {
			return true
		}
		var first gjson.Result
		signed := false
		calls.ForEach(func(_, call gjson.Result) bool {
			if strings.TrimSpace(call.Get("extra_content.google.thought_signature").Str) != "" {
				signed = true
				return false
			}
			if !first.Exists() && call.Get("type").Str == "function" && call.Get("function.name").Str != "" {
				first = call
			}
			return true
		})
		if signed || !first.Exists() {
			return true
		}
		start, end := first.Index, first.Index+len(first.Raw)
		if start <= 0 || end > len(body) || (len(replacements) > 0 && start < replacements[len(replacements)-1].end) {
			return true
		}
		updated, err := sjson.Set(first.Raw, "extra_content.google.thought_signature", geminiDummyThoughtSignature)
		if err == nil {
			replacements = append(replacements, replacement{start: start, end: end, value: updated})
			size += len(updated) - len(first.Raw)
		}
		return true
	})
	if len(replacements) == 0 {
		return body
	}
	out := make([]byte, 0, size)
	offset := 0
	for _, replacement := range replacements {
		out = append(out, body[offset:replacement.start]...)
		out = append(out, replacement.value...)
		offset = replacement.end
	}
	return append(out, body[offset:]...)
}

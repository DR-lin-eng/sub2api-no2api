package service

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// stripEmptyChatToolCallIdentityFromSSELine removes only empty id/name fields
// from follow-up Chat Completions tool-call deltas. Some compatible providers
// repeat these fields as empty strings; clients merge present fields and would
// otherwise overwrite the valid identity from the first delta.
func stripEmptyChatToolCallIdentityFromSSELine(line string) string {
	payload, ok := extractOpenAISSEDataLine(line)
	if !ok || strings.TrimSpace(payload) == "" || strings.TrimSpace(payload) == "[DONE]" {
		return line
	}
	rewritten, changed := stripEmptyChatToolCallIdentity([]byte(payload))
	if !changed {
		return line
	}
	prefixLen := len(line) - len(payload)
	if prefixLen < 0 {
		return line
	}
	return line[:prefixLen] + string(rewritten)
}

func stripEmptyChatToolCallIdentity(payload []byte) ([]byte, bool) {
	if len(payload) == 0 || !bytes.Contains(payload, []byte("tool_calls")) || !gjson.ValidBytes(payload) {
		return payload, false
	}
	choices := gjson.GetBytes(payload, "choices")
	if !choices.Exists() || !choices.IsArray() {
		return payload, false
	}
	updated := payload
	changed := false
	for choiceIndex, choice := range choices.Array() {
		toolCalls := choice.Get("delta.tool_calls")
		if !toolCalls.Exists() || !toolCalls.IsArray() {
			continue
		}
		for toolIndex, toolCall := range toolCalls.Array() {
			if id := toolCall.Get("id"); id.Exists() && id.Type == gjson.String && id.Str == "" {
				next, err := sjson.DeleteBytes(updated, "choices."+strconv.Itoa(choiceIndex)+".delta.tool_calls."+strconv.Itoa(toolIndex)+".id")
				if err != nil {
					return payload, false
				}
				updated = next
				changed = true
			}
			if name := toolCall.Get("function.name"); name.Exists() && name.Type == gjson.String && name.Str == "" {
				next, err := sjson.DeleteBytes(updated, "choices."+strconv.Itoa(choiceIndex)+".delta.tool_calls."+strconv.Itoa(toolIndex)+".function.name")
				if err != nil {
					return payload, false
				}
				updated = next
				changed = true
			}
		}
	}
	return updated, changed
}

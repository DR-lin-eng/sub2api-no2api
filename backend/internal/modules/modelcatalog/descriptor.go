package modelcatalog

import (
	"encoding/json"
	"strings"
)

// Normalize removes unknown/unbounded provider payloads while translating
// provider spellings into Codex-compatible fields. Credentials and quota data
// are never part of a model capability descriptor.
func Normalize(raw Descriptor, id string) Descriptor {
	// Translate nested provider metadata without changing the caller's map.
	raw = Fill(raw, nil)
	for field, translations := range map[string]map[string]string{
		"limit":                   {"context": "context_window", "output": "max_output_tokens"},
		"modalities":              {"input": "input_modalities", "output": "output_modalities"},
		"codex_tool_capabilities": {"supports_parallel_tool_calls": "supports_parallel_tool_calls", "supports_search_tool": "supports_search_tool", "shell_type": "shell_type", "apply_patch_tool_type": "apply_patch_tool_type"},
	} {
		var nested Descriptor
		if json.Unmarshal(raw[field], &nested) == nil {
			for from, to := range translations {
				if _, exists := raw[to]; !exists {
					if value, ok := nested[from]; ok {
						raw[to] = value
					}
				}
			}
		}
	}
	out := Descriptor{}
	aliases := map[string][]string{
		"display_name": {"displayName", "title"}, "description": {},
		"context_window":             {"contextWindow", "inputTokenLimit", "maxTokens", "max_context_window", "max_input_tokens"},
		"max_context_window":         {"maxContextWindow"},
		"max_output_tokens":          {"maxOutputTokens", "outputTokenLimit"},
		"supports_reasoning":         {"reasoning", "supportsReasoning", "supportsThinking"},
		"supported_reasoning_levels": {"supportedReasoningLevels"},
		"default_reasoning_level":    {"defaultReasoningLevel"},
		"input_modalities":           {"inputModalities"}, "output_modalities": {"outputModalities"},
		"supports_parallel_tool_calls": {}, "supports_image_detail_original": {},
		"supports_verbosity": {}, "default_verbosity": {}, "apply_patch_tool_type": {},
		"shell_type": {}, "supports_search_tool": {}, "supports_computer_use": {},
		"supports_image_generation": {}, "supports_web_search": {},
	}
	for key, names := range aliases {
		value, present := raw[key]
		if !present {
			for _, name := range names {
				if v, ok := raw[name]; ok {
					value = v
					present = true
					break
				}
			}
		}
		if !present || len(value) > 8192 {
			continue
		}
		if validField(key, value) {
			out[key] = append(json.RawMessage(nil), value...)
		}
	}
	if _, exists := out["input_modalities"]; !exists {
		if rawImages, ok := raw["supportsImages"]; ok {
			var images bool
			if json.Unmarshal(rawImages, &images) == nil {
				out["input_modalities"] = json.RawMessage(`["text"]`)
				if images {
					out["input_modalities"] = json.RawMessage(`["text","image"]`)
				}
			}
		}
	}
	out["id"], _ = json.Marshal(id)
	return out
}

func validField(key string, raw json.RawMessage) bool {
	switch key {
	case "context_window", "max_context_window", "max_output_tokens":
		var n int64
		return json.Unmarshal(raw, &n) == nil && n > 0 && n <= 1<<31
	case "input_modalities", "output_modalities", "supported_reasoning_levels":
		var values []json.RawMessage
		if json.Unmarshal(raw, &values) != nil || values == nil || len(values) > 16 {
			return false
		}
		for _, v := range values {
			var s string
			// Codex supplies reasoning entries as objects with effort and description.
			if json.Unmarshal(v, &s) != nil && key == "supported_reasoning_levels" {
				var entry map[string]json.RawMessage
				if json.Unmarshal(v, &entry) != nil {
					return false
				}
				_ = json.Unmarshal(entry["effort"], &s)
			}
			if len(s) == 0 || len(s) > 64 {
				return false
			}
		}
		return true
	}
	if strings.HasPrefix(key, "supports_") {
		var b bool
		return json.Unmarshal(raw, &b) == nil && string(raw) != "null"
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return false
	}
	limit := 256
	if key == "description" {
		limit = 4096
	}
	return len(s) <= limit && !strings.ContainsRune(s, 0)
}

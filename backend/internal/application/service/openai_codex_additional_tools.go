package service

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Only compact UUID-shaped declaration IDs; other provider IDs are opaque.
func normalizeCodexAdditionalToolsID(id string) string {
	if len(id) != 39 || !strings.HasPrefix(id, "at_") ||
		id[11] != '-' || id[16] != '-' || id[21] != '-' || id[26] != '-' {
		return id
	}
	compactUUID := strings.ReplaceAll(id[3:], "-", "")
	if len(compactUUID) != 32 {
		return id
	}
	if _, err := hex.DecodeString(compactUUID); err != nil {
		return id
	}
	return "at_" + compactUUID
}

func codexAdditionalToolsIDMappings(input []any) map[string]string {
	mappings := make(map[string]string)
	for _, rawItem := range input {
		item, ok := rawItem.(map[string]any)
		if !ok || item["type"] != "additional_tools" {
			continue
		}
		if id, ok := item["id"].(string); ok {
			if normalizedID := normalizeCodexAdditionalToolsID(id); normalizedID != id {
				mappings[id] = normalizedID
			}
		}
	}
	return mappings
}

// The passthrough/WebSocket compatibility path must not decode and re-encode
// whole requests. Patch only declaration IDs and references to those declarations.
func normalizeCodexAdditionalToolsIDsBody(body []byte) ([]byte, bool, error) {
	if !bytes.Contains(body, []byte("additional_tools")) && !bytes.Contains(body, []byte(`\u`)) {
		return body, false, nil
	}
	input := parseRawJSONView(body).Get("input")
	if !input.IsArray() {
		return body, false, nil
	}
	mappings := make(map[string]string)
	input.ForEach(func(_, item gjson.Result) bool {
		if id := item.Get("id"); item.Get("type").String() == "additional_tools" && id.Type == gjson.String {
			if normalizedID := normalizeCodexAdditionalToolsID(id.Str); normalizedID != id.Str {
				mappings[id.Str] = normalizedID
			}
		}
		return true
	})
	if len(mappings) == 0 {
		return body, false, nil
	}
	items := make([]string, 0)
	var patchErr error
	input.ForEach(func(_, item gjson.Result) bool {
		raw := item.Raw
		if typ := item.Get("type").String(); typ == "additional_tools" || typ == "item_reference" {
			if id := item.Get("id"); id.Type == gjson.String {
				if normalizedID, mapped := mappings[id.Str]; mapped {
					raw, patchErr = sjson.Set(raw, "id", normalizedID)
					if patchErr != nil {
						return false
					}
				}
			}
		}
		items = append(items, raw)
		return true
	})
	if patchErr != nil {
		return body, false, fmt.Errorf("normalize additional_tools ID: %w", patchErr)
	}
	return replaceOpenAIRawInput(body, input, items), true, nil
}

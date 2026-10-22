package service

import (
	"encoding/json"
	"strings"
)

func isCloudflareBotBlockResponse(body []byte) bool {
	normalized := strings.ToLower(strings.TrimSpace(string(body)))
	var payload any
	if json.Unmarshal(body, &payload) == nil {
		return containsCloudflareBotBlockCode(payload)
	}
	return strings.Contains(normalized, "error code: 1010")
}

func containsCloudflareBotBlockCode(value any) bool {
	switch node := value.(type) {
	case map[string]any:
		cloudflareError := false
		for key, child := range node {
			if strings.EqualFold(key, "cloudflare_error") {
				if is, ok := child.(bool); ok && is {
					cloudflareError = true
				}
			}
		}
		for key, child := range node {
			if strings.EqualFold(key, "error_name") {
				if name, ok := child.(string); ok && strings.EqualFold(strings.TrimSpace(name), "browser_signature_banned") {
					return true
				}
			}
			if cloudflareError && (strings.EqualFold(key, "code") || strings.EqualFold(key, "error_code")) {
				if isCloudflareBotBlockCode(child) {
					return true
				}
			}
			if containsCloudflareBotBlockCode(child) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if containsCloudflareBotBlockCode(child) {
				return true
			}
		}
	}
	return false
}

func isCloudflareBotBlockCode(value any) bool {
	switch code := value.(type) {
	case float64:
		return code == 1010
	case json.Number:
		return code == "1010"
	case string:
		return strings.TrimSpace(code) == "1010"
	default:
		return false
	}
}

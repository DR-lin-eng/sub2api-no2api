package service

import (
	"fmt"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// openAIResponsesServerOwnedTopLevelFields are fields from the Codex
// app-server request envelope, not user-authored Responses content. Sub2API
// does not expose the corresponding server-side entitlement/capability
// authority, so these fields must not cross the relay boundary as client
// assertions. Matching is intentionally top-level-only; the same names inside
// input, tools, or tool arguments remain user data.
var openAIResponsesServerOwnedTopLevelFields = map[string]struct{}{
	"access_programs":       {},
	"accessPrograms":        {},
	"cyber_access_program":  {},
	"cyberAccessProgram":    {},
	"daybreak_enabled":      {},
	"daybreakEnabled":       {},
	"disabled_plugin_ids":   {},
	"disabledPluginIds":     {},
	"service_name":          {},
	"serviceName":           {},
	"service_tier_for_turn": {},
	"serviceTierForTurn":    {},
	"turn_trigger":          {},
	"turnTrigger":           {},
	"approval_policy":       {},
	"approvalPolicy":        {},
	"approvals_reviewer":    {},
	"approvalsReviewer":     {},
	"sandbox_policy":        {},
	"sandboxPolicy":         {},
}

func isOpenAIResponsesServerOwnedTopLevelField(key string) bool {
	if _, exists := openAIResponsesServerOwnedTopLevelFields[key]; exists {
		return true
	}
	// A future casing variant must not become an accidental capability channel,
	// while ordinary user payloads remain protected by the top-level boundary.
	normalized := strings.ToLower(strings.TrimSpace(key))
	switch normalized {
	case "access_programs", "accessprograms", "cyber_access_program", "cyberaccessprogram",
		"daybreak_enabled", "daybreakenabled", "disabled_plugin_ids", "disabledpluginids",
		"service_name", "servicename", "service_tier_for_turn", "servicetierforturn",
		"turn_trigger", "turntrigger", "approval_policy", "approvalpolicy",
		"approvals_reviewer", "approvalsreviewer", "sandbox_policy", "sandboxpolicy":
		return true
	default:
		return false
	}
}

// sanitizeOpenAIResponsesAccessProgramsMap keeps the existing call-site name;
// the actual boundary now covers the complete app-server envelope field set.
func sanitizeOpenAIResponsesAccessProgramsMap(body map[string]any) bool {
	return sanitizeOpenAIResponsesServerOwnedFieldsMap(body)
}

func sanitizeOpenAIResponsesServerOwnedFieldsMap(body map[string]any) bool {
	if body == nil {
		return false
	}
	changed := false
	for key := range body {
		if !isOpenAIResponsesServerOwnedTopLevelField(key) {
			continue
		}
		delete(body, key)
		changed = true
	}
	return changed
}

// sanitizeOpenAIResponsesAccessPrograms keeps the existing call-site name;
// the actual boundary now covers the complete app-server envelope field set.
func sanitizeOpenAIResponsesAccessPrograms(body []byte) ([]byte, bool, error) {
	return sanitizeOpenAIResponsesServerOwnedFields(body)
}

func sanitizeOpenAIResponsesServerOwnedFields(body []byte) ([]byte, bool, error) {
	if len(body) == 0 || !gjson.ValidBytes(body) {
		return body, false, nil
	}
	parsed := gjson.ParseBytes(body)
	if !parsed.IsObject() {
		return body, false, nil
	}

	// Enumerate the actual top-level keys instead of looking up only the
	// canonical spellings. JSON object keys are case-sensitive, while the
	// app-server boundary is intentionally case-insensitive so a casing variant
	// cannot become an accidental capability channel. ForEach is top-level only;
	// nested input/tool payloads remain user data.
	keys := make([]string, 0, len(openAIResponsesServerOwnedTopLevelFields))
	parsed.ForEach(func(key, _ gjson.Result) bool {
		if isOpenAIResponsesServerOwnedTopLevelField(key.String()) {
			keys = append(keys, key.String())
		}
		return true
	})

	sanitized := body
	changed := false
	for _, key := range keys {
		next, err := sjson.DeleteBytes(sanitized, key)
		if err != nil {
			return body, false, fmt.Errorf("strip untrusted Responses field %s: %w", key, err)
		}
		sanitized = next
		changed = true
	}
	return sanitized, changed, nil
}

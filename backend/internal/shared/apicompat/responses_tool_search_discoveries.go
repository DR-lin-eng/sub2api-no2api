package apicompat

import (
	"encoding/json"
	"fmt"
	"strings"
)

// promoteResponsesToolSearchDiscoveries makes completed client-side tool
// discoveries callable by function-only upstreams. Declarations are appended
// after static tools, preserving the client's declaration order. Malformed or
// in-progress discovery entries are ignored; conflicting schemas fail closed.
func promoteResponsesToolSearchDiscoveries(req map[string]any) (bool, error) {
	tools, ok := req["tools"].([]any)
	if !ok || len(tools) == 0 || !hasResponsesToolSearchDeclaration(tools) {
		return false, nil
	}
	input, ok := req["input"].([]any)
	if !ok || len(input) == 0 {
		return false, nil
	}

	known := make(map[string]string)
	for _, raw := range tools {
		registerDiscoveredToolIdentity(known, raw)
	}
	var promoted []any
	for _, raw := range input {
		item, ok := raw.(map[string]any)
		if !ok || strings.TrimSpace(stringValue(item["type"])) != "tool_search_output" {
			continue
		}
		if status, present := item["status"]; present {
			if value, ok := status.(string); !ok || !strings.EqualFold(strings.TrimSpace(value), "completed") {
				continue
			}
		}
		callID := strings.TrimSpace(stringValue(item["call_id"]))
		if callID == "" {
			return false, fmt.Errorf("tool_search_output requires a non-empty string call_id before promotion")
		}
		discoveries, ok := item["tools"].([]any)
		if !ok {
			continue
		}
		for _, rawDiscovery := range discoveries {
			discovery, ok := rawDiscovery.(map[string]any)
			if !ok {
				continue
			}
			typ := strings.TrimSpace(stringValue(discovery["type"]))
			switch typ {
			case "function", "custom":
				name := strings.TrimSpace(stringValue(discovery["name"]))
				if name == "" {
					continue
				}
				copy := copyClientTool(discovery)
				encoded, err := json.Marshal(copy)
				if err != nil {
					continue
				}
				if _, err := appendDiscoveredTool(known, typ+"\x00"+name, string(encoded), copy, &promoted); err != nil {
					return false, err
				}
			case "namespace":
				namespace := strings.TrimSpace(stringValue(discovery["name"]))
				children := namespaceChildren(discovery)
				if namespace == "" || len(children) == 0 {
					continue
				}
				newChildren := make([]any, 0, len(children))
				for _, rawChild := range children {
					child, ok := rawChild.(map[string]any)
					if !ok || strings.TrimSpace(stringValue(child["type"])) != "function" {
						continue
					}
					name := strings.TrimSpace(stringValue(child["name"]))
					if name == "" {
						continue
					}
					childCopy := copyClientTool(child)
					encoded, err := json.Marshal(childCopy)
					if err != nil {
						continue
					}
					key := "namespace\x00" + namespace + "\x00" + name
					if _, err := appendDiscoveredTool(known, key, string(encoded), childCopy, &newChildren); err != nil {
						return false, err
					}
				}
				if len(newChildren) > 0 {
					copy := copyClientTool(discovery)
					copy["tools"] = newChildren
					delete(copy, "children")
					promoted = append(promoted, copy)
				}
			}
		}
	}
	if len(promoted) == 0 {
		return false, nil
	}
	req["tools"] = append(tools, promoted...)
	return true, nil
}

func hasResponsesToolSearchDeclaration(tools []any) bool {
	for _, raw := range tools {
		if tool, ok := raw.(map[string]any); ok && strings.TrimSpace(stringValue(tool["type"])) == "tool_search" {
			return true
		}
	}
	return false
}

func registerDiscoveredToolIdentity(known map[string]string, raw any) {
	tool, ok := raw.(map[string]any)
	if !ok {
		return
	}
	typ := strings.TrimSpace(stringValue(tool["type"]))
	switch typ {
	case "function", "custom":
		name := strings.TrimSpace(stringValue(tool["name"]))
		if encoded, err := json.Marshal(tool); err == nil && name != "" {
			known[typ+"\x00"+name] = string(encoded)
		}
	case "namespace":
		namespace := strings.TrimSpace(stringValue(tool["name"]))
		children := namespaceChildren(tool)
		if len(children) == 0 {
			return
		}
		for _, rawChild := range children {
			child, ok := rawChild.(map[string]any)
			if !ok {
				continue
			}
			name := strings.TrimSpace(stringValue(child["name"]))
			if encoded, err := json.Marshal(child); err == nil && namespace != "" && name != "" {
				known["namespace\x00"+namespace+"\x00"+name] = string(encoded)
			}
		}
	}
}

// appendDiscoveredTool returns true when a new declaration was appended and
// rejects a same-name declaration with a different schema.
func appendDiscoveredTool(known map[string]string, key, encoded string, value any, out *[]any) (bool, error) {
	if previous, exists := known[key]; exists {
		if previous != encoded {
			return false, fmt.Errorf("discovered tool %q conflicts with an existing declaration", key)
		}
		return false, nil
	}
	known[key] = encoded
	*out = append(*out, value)
	return true, nil
}

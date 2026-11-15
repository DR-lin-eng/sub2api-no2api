package apicompat

import (
	"encoding/json"
)

const anthropicToolUnionMaxBranches = 512
const anthropicToolUnionMaxDepth = 16

// Anthropic rejects unions at the tool schema root. Collect branch properties
// before encoding them: repeatedly nesting and marshaling the accumulated
// property makes a wide union quadratic. Nested property unions remain legal.
// Limits apply to the whole tool, including recursively nested root unions.
func flattenAnthropicRootUnions(schema map[string]json.RawMessage) bool {
	remaining := anthropicToolUnionMaxBranches
	return flattenAnthropicToolUnion(schema, 0, &remaining)
}

func flattenAnthropicToolUnion(schema map[string]json.RawMessage, depth int, remaining *int) bool {
	hasUnion := false
	for _, key := range [...]string{"oneOf", "anyOf", "allOf"} {
		if _, exists := schema[key]; exists {
			hasUnion = true
		}
	}
	if !hasUnion {
		return true
	}
	if depth >= anthropicToolUnionMaxDepth {
		return false
	}
	properties := make(map[string]json.RawMessage)
	if raw, ok := schema["properties"]; ok && json.Unmarshal(raw, &properties) != nil {
		return false
	}
	if properties == nil {
		properties = make(map[string]json.RawMessage)
	}
	var required []string
	if raw, ok := schema["required"]; ok && json.Unmarshal(raw, &required) != nil {
		return false
	}
	requiredSet := make(map[string]bool, len(required))
	for _, name := range required {
		requiredSet[name] = true
	}
	for _, keyword := range [...]string{"oneOf", "anyOf", "allOf"} {
		raw, present := schema[keyword]
		if !present {
			continue
		}
		var branches []json.RawMessage
		if json.Unmarshal(raw, &branches) != nil || len(branches) == 0 || len(branches) > *remaining {
			return false
		}
		*remaining -= len(branches)
		alternatives := make(map[string][]json.RawMessage)
		var sharedRequired []string
		for index, rawBranch := range branches {
			var branch map[string]json.RawMessage
			var branchRequired []string
			if json.Unmarshal(rawBranch, &branch) == nil && branch != nil {
				if !flattenAnthropicToolUnion(branch, depth+1, remaining) {
					return false
				}
				var typ string
				object := branch["type"] == nil || (json.Unmarshal(branch["type"], &typ) == nil && typ == "object")
				if object {
					var fields map[string]json.RawMessage
					if fieldRaw := branch["properties"]; fieldRaw != nil && json.Unmarshal(fieldRaw, &fields) != nil {
						return false
					}
					for name, value := range fields {
						alternatives[name] = append(alternatives[name], value)
					}
					if requiredRaw := branch["required"]; requiredRaw != nil && json.Unmarshal(requiredRaw, &branchRequired) != nil {
						return false
					}
				}
			}
			if keyword == "allOf" {
				for _, name := range branchRequired {
					if !requiredSet[name] {
						required = append(required, name)
						requiredSet[name] = true
					}
				}
			} else if index == 0 {
				sharedRequired = branchRequired
			} else {
				members := make(map[string]bool, len(branchRequired))
				for _, name := range branchRequired {
					members[name] = true
				}
				kept := sharedRequired[:0]
				for _, name := range sharedRequired {
					if members[name] {
						kept = append(kept, name)
					}
				}
				sharedRequired = kept
			}
		}
		for _, name := range sharedRequired {
			if !requiredSet[name] {
				required = append(required, name)
				requiredSet[name] = true
			}
		}
		propertyKeyword := "anyOf"
		if keyword == "allOf" {
			propertyKeyword = "allOf"
		}
		for name, values := range alternatives {
			value := values[0]
			if len(values) > 1 {
				value, _ = json.Marshal(map[string][]json.RawMessage{propertyKeyword: values})
			}
			if root, exists := properties[name]; exists {
				value, _ = json.Marshal(map[string][]json.RawMessage{"allOf": {root, value}})
			}
			properties[name] = value
		}
		delete(schema, keyword)
	}
	schema["properties"], _ = json.Marshal(properties)
	if len(required) > 0 {
		schema["required"], _ = json.Marshal(required)
	}
	return true
}

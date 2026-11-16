package service

import (
	"bytes"
	"sort"

	"github.com/tidwall/gjson"
)

// Retain the existing null/union repair owners. Regex compatibility is specific
// to OpenAI; other platforms keep their declared constraints.
func sanitizeOpenAIResponsesToolSchemasForPlatform(body []byte, platform string) ([]byte, bool, error) {
	next, changed, err := sanitizeOpenAIResponsesToolParameterTypes(body)
	if err != nil || platform != PlatformOpenAI {
		return next, changed, err
	}
	next, patternsChanged, err := sanitizeOpenAIResponsesToolSchemaPatterns(next)
	return next, changed || patternsChanged, err
}

// Collect byte spans only along schema edges, then copy the request once.
// Prompts, defaults, examples and arbitrary objects named "pattern" are data.
func sanitizeOpenAIResponsesToolSchemaPatterns(body []byte) ([]byte, bool, error) {
	if !bytes.Contains(body, []byte(`"pattern`)) && !bytes.Contains(body, []byte(`\u`)) {
		return body, false, nil
	}
	if !gjson.ValidBytes(body) {
		return body, false, nil
	}
	// Borrow the immutable request only during this scan; edits retain offsets
	// and constant replacements, never strings referencing the request buffer.
	view := requestJSONReadOnlyString(body)
	var edits []openAIResponsesToolSchemaNullType
	var tools func(gjson.Result, int)
	tools = func(list gjson.Result, depth int) {
		if depth > openAIResponsesToolSchemaMaxDepth || !list.IsArray() {
			return
		}
		list.ForEach(func(_, tool gjson.Result) bool {
			for _, path := range []string{"parameters", "function.parameters", "input_schema"} {
				schema := tool.Get(path)
				if len(schema.Raw) <= 1<<20 {
					collectOpenAIRegexSchemaEdits(body, schema, 0, &edits)
				}
			}
			tools(tool.Get("tools"), depth+1)
			return len(edits) <= 4096
		})
	}
	tools(gjson.Get(view, "tools"), 0)
	if input := gjson.Get(view, "input"); input.IsArray() {
		input.ForEach(func(_, item gjson.Result) bool {
			tools(item.Get("tools"), 0)
			return len(edits) <= 4096
		})
	}
	if len(edits) == 0 || len(edits) > 4096 {
		return body, false, nil
	}
	sort.Slice(edits, func(i, j int) bool { return edits[i].offset < edits[j].offset })
	result := make([]byte, 0, len(body))
	cursor := 0
	for _, edit := range edits {
		if edit.offset < cursor {
			continue
		}
		result = append(result, body[cursor:edit.offset]...)
		result = append(result, edit.replacement...)
		cursor = edit.offset + edit.length
	}
	result = append(result, body[cursor:]...)
	return result, true, nil
}

func collectOpenAIRegexSchemaEdits(body []byte, schema gjson.Result, depth int, edits *[]openAIResponsesToolSchemaNullType) {
	if depth >= 64 || !schema.IsObject() || len(*edits) > 4096 {
		return
	}
	lostPattern := false
	if patterns := schema.Get("patternProperties"); patterns.IsObject() {
		type member struct{ key, value gjson.Result }
		var members []member
		patterns.ForEach(func(key, value gjson.Result) bool {
			members = append(members, member{key, value})
			return true
		})
		for i := 0; i < len(members); {
			if !hasUnsupportedOpenAIRegexPattern(members[i].key.String()) {
				collectOpenAIRegexSchemaEdits(body, members[i].value, depth+1, edits)
				i++
				continue
			}
			lostPattern = true
			first := i
			for i < len(members) && hasUnsupportedOpenAIRegexPattern(members[i].key.String()) {
				i++
			}
			start := members[first].key.Index
			end := members[i-1].value.Index + len(members[i-1].value.Raw)
			if i < len(members) {
				end = members[i].key.Index
			} else if first > 0 {
				start = members[first-1].value.Index + len(members[first-1].value.Raw)
			}
			*edits = append(*edits, openAIResponsesToolSchemaNullType{offset: start, length: end - start})
		}
	}
	schema.ForEach(func(key, value gjson.Result) bool {
		switch key.String() {
		case "pattern":
			if value.Type == gjson.String && hasUnsupportedOpenAIRegexPattern(value.String()) {
				// An always-matching pattern has the same compatibility meaning as
				// removing this constraint, without rewriting surrounding members.
				appendOpenAIResponsesToolSchemaNullType(body, value, `".*"`, edits)
			}
		case "patternProperties":
			// Already processed, including deletion of incompatible keys.
		case "additionalProperties":
			if lostPattern && value.Raw != "true" {
				appendOpenAIResponsesToolSchemaNullType(body, value, "true", edits)
			} else {
				collectOpenAIRegexSchemaEdits(body, value, depth+1, edits)
			}
		case "properties", "$defs", "definitions", "dependentSchemas", "dependencies":
			if value.IsObject() {
				value.ForEach(func(_, child gjson.Result) bool {
					collectOpenAIRegexSchemaEdits(body, child, depth+1, edits)
					return len(*edits) <= 4096
				})
			}
		case "allOf", "anyOf", "oneOf", "prefixItems", "items":
			if value.IsArray() {
				value.ForEach(func(_, child gjson.Result) bool {
					collectOpenAIRegexSchemaEdits(body, child, depth+1, edits)
					return len(*edits) <= 4096
				})
			} else {
				collectOpenAIRegexSchemaEdits(body, value, depth+1, edits)
			}
		case "additionalItems", "contains", "not", "if", "then", "else", "propertyNames", "unevaluatedProperties", "unevaluatedItems", "contentSchema":
			collectOpenAIRegexSchemaEdits(body, value, depth+1, edits)
		}
		return len(*edits) <= 4096
	})
}

// OpenAI rejects lookaround in addition to the syntax Python re rejects.
// Do not use Go regexp: it would reject valid Python named groups and escapes.
func hasUnsupportedOpenAIRegexPattern(pattern string) bool {
	inClass := false
	for i := 0; i < len(pattern); i++ {
		switch pattern[i] {
		case '\\':
			if i+1 >= len(pattern) {
				return true
			}
			escaped := pattern[i+1]
			if (escaped >= 'a' && escaped <= 'z' || escaped >= 'A' && escaped <= 'Z') && !bytes.ContainsRune([]byte("abfnrtvdDsSwWAZBNuUx"), rune(escaped)) {
				return true
			}
			i++
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '(':
			if !inClass && i+1 < len(pattern) && pattern[i+1] == '?' {
				if i+2 >= len(pattern) || bytes.ContainsRune([]byte("=!<'"), rune(pattern[i+2])) {
					return true
				}
			}
		}
	}
	return false
}

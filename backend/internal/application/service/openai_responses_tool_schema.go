package service

import (
	"bytes"
	"sort"

	"github.com/tidwall/gjson"
)

const (
	// Tool definitions in historical turns may contain another tools array.
	// Bound traversal so malformed requests cannot cause unbounded recursion.
	openAIResponsesToolSchemaMaxDepth = 4
	// JSON Schema type only accepts a string or string array. An explicit null
	// is invalid, and object matches the upstream contract for these tools.
	openAIResponsesToolSchemaFallbackType = `"object"`
	openAIResponsesToolSchemaNullLiteral  = "null"
)

type openAIResponsesToolSchemaNullType struct {
	offset      int
	length      int
	replacement string
}

// sanitizeOpenAIResponsesToolParameterTypes fixes explicit null values in
// tools[].parameters.type without narrowing schemas where type is absent.
// Matches are collected first and rewritten in one pass so a large Responses
// request is copied once rather than once per offending tool.
func sanitizeOpenAIResponsesToolParameterTypes(body []byte) ([]byte, bool, error) {
	if len(body) == 0 {
		return body, false, nil
	}

	hits := make([]openAIResponsesToolSchemaNullType, 0, 2)
	collectOpenAIResponsesToolSchemaNullTypes(body, gjson.GetBytes(body, "tools"), 0, &hits)
	if input := gjson.GetBytes(body, "input"); input.IsArray() {
		input.ForEach(func(_, item gjson.Result) bool {
			if item.IsObject() {
				collectOpenAIResponsesToolSchemaNullTypes(body, item.Get("tools"), 0, &hits)
			}
			return true
		})
	}
	if len(hits) == 0 {
		return body, false, nil
	}

	sort.Slice(hits, func(i, j int) bool { return hits[i].offset < hits[j].offset })
	sanitized := make([]byte, 0, len(body)+len(hits)*len(openAIResponsesToolSchemaFallbackType))
	cursor := 0
	for _, hit := range hits {
		if hit.offset < cursor {
			continue
		}
		sanitized = append(sanitized, body[cursor:hit.offset]...)
		sanitized = append(sanitized, hit.replacement...)
		cursor = hit.offset + hit.length
	}
	sanitized = append(sanitized, body[cursor:]...)
	return sanitized, true, nil
}

func collectOpenAIResponsesToolSchemaNullTypes(
	body []byte,
	tools gjson.Result,
	depth int,
	hits *[]openAIResponsesToolSchemaNullType,
) {
	if depth > openAIResponsesToolSchemaMaxDepth || !tools.IsArray() {
		return
	}
	tools.ForEach(func(_, tool gjson.Result) bool {
		if !tool.IsObject() {
			return true
		}
		// Responses tools use parameters directly; Chat Completions-shaped tools
		// may also appear here after compatibility normalization.
		for _, suffix := range []string{"parameters", "function.parameters", "input_schema"} {
			params := tool.Get(suffix)
			if !params.IsObject() {
				continue
			}
			if typ := params.Get("type"); typ.Type == gjson.Null && typ.Raw == openAIResponsesToolSchemaNullLiteral {
				appendOpenAIResponsesToolSchemaNullType(body, typ, openAIResponsesToolSchemaFallbackType, hits)
			}
			collectOpenAIResponsesNullRequired(body, params, 0, hits)
		}
		collectOpenAIResponsesToolSchemaNullTypes(body, tool.Get("tools"), depth+1, hits)
		return true
	})
}

func appendOpenAIResponsesToolSchemaNullType(
	body []byte,
	typ gjson.Result,
	replacement string,
	hits *[]openAIResponsesToolSchemaNullType,
) {
	end := typ.Index + len(typ.Raw)
	if typ.Index <= 0 || end > len(body) {
		return
	}
	if !bytes.Equal(body[typ.Index:end], []byte(typ.Raw)) {
		return
	}
	*hits = append(*hits, openAIResponsesToolSchemaNullType{offset: typ.Index, length: len(typ.Raw), replacement: replacement})
}

// Only follow JSON Schema edges: default/const/examples and properties named
// "required" are user data. Replacing invalid required:null with [] preserves
// the optional-field meaning without deleting commas or re-encoding a body.
func collectOpenAIResponsesNullRequired(body []byte, schema gjson.Result, depth int, hits *[]openAIResponsesToolSchemaNullType) {
	if depth >= 64 || !schema.IsObject() {
		return
	}
	schema.ForEach(func(key, value gjson.Result) bool {
		switch key.String() {
		case "required":
			if value.Type == gjson.Null && value.Raw == "null" {
				appendOpenAIResponsesToolSchemaNullType(body, value, "[]", hits)
			}
		case "properties", "patternProperties", "$defs", "definitions", "dependentSchemas":
			if value.IsObject() {
				value.ForEach(func(_, child gjson.Result) bool {
					collectOpenAIResponsesNullRequired(body, child, depth+1, hits)
					return true
				})
			}
		case "allOf", "anyOf", "oneOf", "prefixItems", "items":
			if value.IsArray() {
				value.ForEach(func(_, child gjson.Result) bool {
					collectOpenAIResponsesNullRequired(body, child, depth+1, hits)
					return true
				})
			} else {
				collectOpenAIResponsesNullRequired(body, value, depth+1, hits)
			}
		case "additionalProperties", "additionalItems", "contains", "not", "if", "then", "else", "propertyNames", "unevaluatedProperties", "unevaluatedItems", "contentSchema":
			collectOpenAIResponsesNullRequired(body, value, depth+1, hits)
		}
		return true
	})
}

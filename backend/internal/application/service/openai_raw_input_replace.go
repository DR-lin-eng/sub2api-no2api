package service

import "github.com/tidwall/gjson"

// replaceOpenAIRawValue replaces one JSON value by copying the surrounding
// request bytes once.  The value's Index/Raw pair comes from parseRawJSONView,
// which keeps large data URLs and opaque extension fields out of an eager
// whole-body decode.
func replaceOpenAIRawValue(body []byte, value gjson.Result, replacement string) []byte {
	if value.Index < 0 || value.Index+len(value.Raw) > len(body) {
		return body
	}
	result := make([]byte, 0, len(body)-len(value.Raw)+len(replacement))
	result = append(result, body[:value.Index]...)
	result = append(result, replacement...)
	return append(result, body[value.Index+len(value.Raw):]...)
}

// replaceOpenAIRawInput rebuilds an input array while retaining unchanged
// elements as raw strings.  This is intentionally a single final allocation;
// it avoids the second complete-body allocation caused by sjson.SetRawBytes
// for large Responses image requests.
func replaceOpenAIRawInput(body []byte, input gjson.Result, items []string) []byte {
	size := len(body) - len(input.Raw) + 2
	for index, item := range items {
		size += len(item)
		if index > 0 {
			size++
		}
	}
	if input.Index < 0 || input.Index+len(input.Raw) > len(body) {
		return body
	}
	result := make([]byte, 0, size)
	result = append(result, body[:input.Index]...)
	result = append(result, '[')
	for index, item := range items {
		if index > 0 {
			result = append(result, ',')
		}
		result = append(result, item...)
	}
	result = append(result, ']')
	return append(result, body[input.Index+len(input.Raw):]...)
}

// hasDuplicateJSONObjectKeys reports duplicate keys in one object.  GJSON
// selects the first value while encoding/json (the decoder fallback) keeps
// the last; detecting this corner case preserves historical normalization.
func hasDuplicateJSONObjectKeys(object gjson.Result) bool {
	if !object.IsObject() {
		return false
	}
	seen := make(map[string]struct{})
	duplicate := false
	object.ForEach(func(key, _ gjson.Result) bool {
		if _, exists := seen[key.Str]; exists {
			duplicate = true
			return false
		}
		seen[key.Str] = struct{}{}
		return true
	})
	return duplicate
}

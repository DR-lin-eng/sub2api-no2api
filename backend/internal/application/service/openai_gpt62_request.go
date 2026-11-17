package service

import (
	"encoding/json"

	"github.com/tidwall/gjson"
)

// Use the existing patch owner to preserve the conversation prefix, encrypted
// reasoning and unrelated include values on native and compatibility paths.
func patchGPT62ResponsesParameters(body []byte, set func(string, any), remove func(string)) {
	for _, field := range []string{"temperature", "top_p", "top_logprobs"} {
		if gjson.GetBytes(body, field).Exists() {
			remove(field)
		}
	}
	include := gjson.GetBytes(body, "include")
	if !include.IsArray() {
		return
	}
	hasLogprobs := false
	include.ForEach(func(_, value gjson.Result) bool {
		hasLogprobs = value.Type == gjson.String && value.String() == "message.output_text.logprobs"
		return !hasLogprobs
	})
	if !hasLogprobs {
		return
	}
	values := include.Array()
	filtered := make([]json.RawMessage, 0, len(values))
	for _, value := range values {
		if value.Type != gjson.String || value.String() != "message.output_text.logprobs" {
			filtered = append(filtered, json.RawMessage(value.Raw))
		}
	}
	set("include", filtered)
}

package handler

import "github.com/tidwall/gjson"

const invalidStreamFieldTypeMessage = "invalid stream field type"

func parseOpenAICompatibleStream(body []byte) (bool, bool) {
	streamResult := gjson.GetBytes(body, "stream")
	if streamResult.Exists() && streamResult.Type != gjson.True && streamResult.Type != gjson.False {
		return false, false
	}
	return streamResult.Bool(), true
}

func validateChatCompletionsOutputTokenLimits(body []byte) (string, bool) {
	// Match the conversion path: a non-null max_completion_tokens overrides
	// max_tokens, including when its value is zero. Validate before account
	// selection so an invalid request cannot reach any upstream protocol.
	field := "max_completion_tokens"
	value := gjson.GetBytes(body, field)
	if !value.Exists() || value.Type == gjson.Null {
		field = "max_tokens"
		value = gjson.GetBytes(body, field)
	}
	if value.Type == gjson.Number && value.Num < 0 {
		return field + " must be greater than or equal to 0", false
	}
	return "", true
}

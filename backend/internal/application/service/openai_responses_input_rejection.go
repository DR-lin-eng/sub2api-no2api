package service

import (
	"encoding/json"
	"github.com/tidwall/gjson"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

var responsesInputStringValidationError = regexp.MustCompile(`^['"]loc['"]\s*:\s*\(\s*['"]body['"]\s*,\s*['"]input['"]\s*,\s*['"]str['"]\s*\)$`)
var responsesStringValidationMessage = regexp.MustCompile(`^['"]msg['"]\s*:\s*['"]Input should be a valid string['"]$`)
var responsesStringValidationType = regexp.MustCompile(`^['"]type['"]\s*:\s*['"]string_type['"]$`)
var responsesValidationErrors = regexp.MustCompile(`(?s)^([1-9][0-9]*) validation errors?(?: for [^\r\n:]+)?:\s*(\[.*\])$`)
var responsesInvalidInputTypeMessage = regexp.MustCompile(`(?i)^(?:Invalid type for ['"]input['"]:\s*)?Expected a string(?: for input)?, but got an (?:array|object) instead\.?$`)

func isResponsesInputStringValidationMessage(message string) bool {
	match := responsesValidationErrors.FindStringSubmatch(strings.TrimSpace(message))
	if match == nil {
		return false
	}
	entries, ok := splitResponsesValidationFields(match[2][1 : len(match[2])-1])
	count, err := strconv.Atoi(match[1])
	if !ok || err != nil || count != len(entries) {
		return false
	}
	matched := false
	for _, entry := range entries {
		if !strings.HasPrefix(entry, "{") || !strings.HasSuffix(entry, "}") {
			return false
		}
		fields, ok := splitResponsesValidationFields(entry[1 : len(entry)-1])
		if !ok {
			return false
		}
		location, message, stringType := false, false, false
		keys := make(map[string]bool, len(fields))
		for _, field := range fields {
			key, value, found := strings.Cut(field, ":")
			key = strings.TrimSpace(key)
			if !found || len(key) < 3 || (key[0] != '\'' && key[0] != '"') || key[len(key)-1] != key[0] || strings.TrimSpace(value) == "" || keys[key[1:len(key)-1]] {
				return false
			}
			keys[key[1:len(key)-1]] = true
			location = location || responsesInputStringValidationError.MatchString(field)
			message = message || responsesStringValidationMessage.MatchString(field)
			stringType = stringType || responsesStringValidationType.MatchString(field)
		}
		matched = matched || (location && message && stringType)
	}
	return matched
}

func splitResponsesValidationFields(value string) ([]string, bool) {
	var fields []string
	var brackets []byte
	var quote byte
	escaped := false
	start := 0
	for index := 0; index < len(value); index++ {
		character := value[index]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
		case '{', '[', '(':
			brackets = append(brackets, character)
		case '}', ']', ')':
			if len(brackets) == 0 {
				return nil, false
			}
			opening := brackets[len(brackets)-1]
			if (character == '}' && opening != '{') || (character == ']' && opening != '[') || (character == ')' && opening != '(') {
				return nil, false
			}
			brackets = brackets[:len(brackets)-1]
		case ',':
			if len(brackets) == 0 {
				field := strings.TrimSpace(value[start:index])
				if field == "" {
					return nil, false
				}
				fields = append(fields, field)
				start = index + 1
			}
		}
	}
	if quote != 0 || len(brackets) != 0 {
		return nil, false
	}
	if field := strings.TrimSpace(value[start:]); field != "" {
		fields = append(fields, field)
	}
	return fields, len(fields) > 0
}

func isConvertedResponsesInputStringRejection(status int, responseBody []byte) bool {
	if status != http.StatusBadRequest || !json.Valid(responseBody) {
		return false
	}
	envelope := gjson.ParseBytes(responseBody)
	for _, object := range []gjson.Result{envelope, envelope.Get("error")} {
		if !object.IsObject() {
			return false
		}
		keys := make(map[string]bool)
		unambiguous := true
		object.ForEach(func(key, value gjson.Result) bool {
			name := key.String()
			if keys[name] {
				unambiguous = false
				return false
			}
			keys[name] = true
			switch name {
			case "usage", "output", "output_text", "choices", "tool_calls", "response", "status", "incomplete_details":
				unambiguous = false
			case "code":
				code := strings.ToLower(strings.TrimSpace(value.String()))
				unambiguous = code == "" || code == "invalid_type" || code == "invalid_request_error"
			case "param":
				param := strings.ToLower(strings.TrimSpace(value.String()))
				unambiguous = param == "" || param == "input"
			case "type":
				errorType := strings.ToLower(strings.TrimSpace(value.String()))
				unambiguous = errorType == "" || errorType == "invalid_type" || errorType == "invalid_request_error"
			}
			return unambiguous
		})
		if !unambiguous {
			return false
		}
	}
	message := gjson.GetBytes(responseBody, "error.message").String()
	if isResponsesInputStringValidationMessage(message) {
		return true
	}
	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(responseBody, "error.code").String()))
	typ := strings.ToLower(strings.TrimSpace(gjson.GetBytes(responseBody, "error.type").String()))
	if code != "invalid_type" && typ != "invalid_type" {
		return false
	}
	param := strings.ToLower(strings.TrimSpace(gjson.GetBytes(responseBody, "error.param").String()))
	if param != "input" {
		return false
	}
	message = strings.ToLower(strings.TrimSpace(message))
	return responsesInvalidInputTypeMessage.MatchString(message)
}

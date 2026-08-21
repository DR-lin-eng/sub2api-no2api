package service

import (
	"encoding/json"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// An inline image is already visible to Grok. Keeping Codex's local view_image
// tool in the same turn can make Grok announce a tool call without executing it,
// so remove only that redundant automatic choice.
func stripRedundantGrokViewImageTool(body []byte) ([]byte, error) {
	input := gjson.GetBytes(body, "input")
	if !input.IsArray() || len(input.Array()) == 0 {
		return body, nil
	}
	items := input.Array()
	current := items[len(items)-1]
	if strings.TrimSpace(current.Get("role").String()) != "user" || !openAIJSONValueMayContainImageInput(current) {
		return body, nil
	}

	toolChoice := gjson.GetBytes(body, "tool_choice")
	if toolChoice.IsObject() && strings.TrimSpace(toolChoice.Get("type").String()) == "function" {
		choiceName := strings.TrimSpace(toolChoice.Get("name").String())
		if choiceName == "" {
			choiceName = strings.TrimSpace(toolChoice.Get("function.name").String())
		}
		if choiceName == "view_image" {
			return body, nil
		}
	}

	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return body, nil
	}
	filtered := make([]json.RawMessage, 0, len(tools.Array()))
	changed := false
	for _, tool := range tools.Array() {
		if strings.TrimSpace(tool.Get("type").String()) == "function" && strings.TrimSpace(tool.Get("name").String()) == "view_image" {
			changed = true
			continue
		}
		filtered = append(filtered, json.RawMessage(tool.Raw))
	}
	if !changed || (len(filtered) == 0 && strings.TrimSpace(toolChoice.String()) == "required") {
		return body, nil
	}
	if len(filtered) == 0 {
		out, err := sjson.DeleteBytes(body, "tools")
		if err != nil {
			return nil, err
		}
		return sjson.DeleteBytes(out, "parallel_tool_calls")
	}
	encoded, err := json.Marshal(filtered)
	if err != nil {
		return nil, err
	}
	return sjson.SetRawBytes(body, "tools", encoded)
}

func stripRedundantGrokChatViewImageTool(body []byte) ([]byte, error) {
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() || len(messages.Array()) == 0 {
		return body, nil
	}
	items := messages.Array()
	current := items[len(items)-1]
	if strings.TrimSpace(current.Get("role").String()) != "user" || !openAIJSONValueMayContainImageInput(current) {
		return body, nil
	}

	toolChoice := gjson.GetBytes(body, "tool_choice")
	if toolChoice.IsObject() && strings.TrimSpace(toolChoice.Get("type").String()) == "function" {
		choiceName := strings.TrimSpace(toolChoice.Get("function.name").String())
		if choiceName == "" {
			choiceName = strings.TrimSpace(toolChoice.Get("name").String())
		}
		if choiceName == "view_image" {
			return body, nil
		}
	}

	tools := gjson.GetBytes(body, "tools")
	if !tools.IsArray() {
		return body, nil
	}
	filtered := make([]json.RawMessage, 0, len(tools.Array()))
	changed := false
	for _, tool := range tools.Array() {
		toolName := strings.TrimSpace(tool.Get("function.name").String())
		if toolName == "" {
			toolName = strings.TrimSpace(tool.Get("name").String())
		}
		if strings.TrimSpace(tool.Get("type").String()) == "function" && toolName == "view_image" {
			changed = true
			continue
		}
		filtered = append(filtered, json.RawMessage(tool.Raw))
	}
	if !changed || (len(filtered) == 0 && strings.TrimSpace(toolChoice.String()) == "required") {
		return body, nil
	}
	if len(filtered) > 0 {
		encoded, err := json.Marshal(filtered)
		if err != nil {
			return nil, err
		}
		return sjson.SetRawBytes(body, "tools", encoded)
	}
	out, err := sjson.DeleteBytes(body, "tools")
	if err != nil {
		return nil, err
	}
	out, err = sjson.DeleteBytes(out, "parallel_tool_calls")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(toolChoice.String()) == "auto" {
		out, err = sjson.DeleteBytes(out, "tool_choice")
	}
	return out, err
}

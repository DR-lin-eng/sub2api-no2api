package service

import (
	"encoding/json"
	"fmt"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/apicompat"
	"strings"
)

func parseChatRequestForBridge(body []byte, platform string, req *apicompat.ChatCompletionsRequest) error {
	if platform != PlatformKimi {
		return json.Unmarshal(body, req)
	}
	var wire struct {
		*apicompat.ChatCompletionsRequest
		Messages []struct {
			apicompat.ChatMessage
			Tools json.RawMessage `json:"tools"`
		} `json:"messages"`
	}
	wire.ChatCompletionsRequest = req
	if err := json.Unmarshal(body, &wire); err != nil {
		return err
	}
	req.Messages = make([]apicompat.ChatMessage, 0, len(wire.Messages))
	for i, msg := range wire.Messages {
		if msg.Role != "system" || msg.Tools == nil {
			req.Messages = append(req.Messages, msg.ChatMessage)
			continue
		}
		if msg.Content != nil {
			return fmt.Errorf("messages[%d]: Kimi system tools cannot be used with content", i)
		}
		var tools []apicompat.ChatTool
		if err := json.Unmarshal(msg.Tools, &tools); err != nil {
			return fmt.Errorf("messages[%d].tools: %w", i, err)
		}
		for j, tool := range tools {
			if tool.Type != "function" || tool.Function == nil || strings.TrimSpace(tool.Function.Name) == "" {
				return fmt.Errorf("messages[%d].tools[%d]: expected a named function", i, j)
			}
		}
		req.Tools = append(req.Tools, tools...)
	}
	return nil
}

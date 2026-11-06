package service

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/shared/openai"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// SIWC direct grants use a public API identity. Session IDs have already
// passed through the gateway's existing isolation before this boundary.
func applyOpenAISIWCHeaders(headers http.Header) {
	if headers == nil {
		return
	}
	sessionID := headers.Get("session_id")
	if sessionID == "" {
		sessionID = headers.Get("session-id")
	}
	for key := range headers {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "x-codex-") || strings.HasPrefix(lower, "x-oai-") || strings.HasPrefix(lower, "x-openai-attach-") {
			delete(headers, key)
		}
	}
	for _, name := range []string{"originator", "version", "chatgpt-account-id", "x-openai-fedramp", "conversation_id", "session-id", "thread-id", "x-openai-expected-account-id", "x-openai-subagent"} {
		deleteOpenAIHeaderEqualFold(headers, name)
	}
	headers.Set("User-Agent", fmt.Sprintf("%s (%s; %s)", openai.SIWCAgentNameHint, runtime.GOOS, runtime.GOARCH))
	if sessionID != "" {
		headers.Set("session_id", sessionID)
		headers.Set("X-Client-Request-Id", sessionID)
	}
}

// Both streaming and collected non-streaming SIWC requests use upstream SSE.
// Native WebSockets and compact have separate envelope fields.
func normalizeOpenAISIWCBody(body []byte, compact, websocket bool) ([]byte, error) {
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return nil, fmt.Errorf("invalid SIWC request body")
	}
	normalized := body
	var err error
	for _, key := range []string{"prompt_cache_retention", "prompt_cache_options", "max_completion_tokens", "temperature"} {
		normalized, err = sjson.DeleteBytes(normalized, key)
		if err != nil {
			return nil, err
		}
	}
	if instructions := gjson.GetBytes(normalized, "instructions"); instructions.Type == gjson.String && strings.TrimSpace(instructions.String()) == "" {
		normalized, err = sjson.DeleteBytes(normalized, "instructions")
		if err != nil {
			return nil, err
		}
	}
	input := gjson.GetBytes(normalized, "input")
	if input.IsArray() {
		items := make([]string, 0)
		changed, effort := false, ""
		input.ForEach(func(_, item gjson.Result) bool {
			if item.Get("type").String() == "configuration_update" {
				changed = true
				if value := item.Get("reasoning.effort"); value.Type == gjson.String && strings.TrimSpace(value.String()) != "" {
					effort = value.String()
				}
			} else {
				items = append(items, item.Raw)
			}
			return true
		})
		if changed {
			normalized = replaceOpenAIRawInput(normalized, input, items)
			if effort != "" {
				normalized, err = sjson.SetBytes(normalized, "reasoning.effort", effort)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	if compact || websocket {
		normalized, err = sjson.DeleteBytes(normalized, "stream")
	} else {
		normalized, err = sjson.SetBytes(normalized, "stream", true)
	}
	if err != nil {
		return nil, err
	}
	if compact {
		return sjson.DeleteBytes(normalized, "store")
	}
	if websocket {
		normalized, err = sjson.DeleteBytes(normalized, "background")
		if err != nil {
			return nil, err
		}
	}
	return sjson.SetBytes(normalized, "store", false)
}

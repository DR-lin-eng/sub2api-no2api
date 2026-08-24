package service

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
)

const (
	defaultContextHintKeepRecentToolResults    = 5
	defaultContextManagementKeepRecentThinking = 1
	contextHintToolResultPlaceholder           = "[tool result cleared by context hint]"
	contextManagementThinkingPlaceholder       = "[thinking block cleared by context management]"
)

// openAIAnthropicContextControlStats records local work done for Anthropic
// context controls when a request is headed to an OpenAI-compatible upstream.
// The OpenAI protocols have no equivalent top-level context_hint or
// context_management field, so silently dropping those fields would make a
// Claude Code session grow until the next hard context error.  We apply the
// safe, content-local subset before converting the request.
type openAIAnthropicContextControlStats struct {
	ContextHintSeen       bool
	ContextManagementSeen bool
	ToolResultsCleared    int
	ThinkingBlocksCleared int
}

func (s openAIAnthropicContextControlStats) changed() bool {
	return s.ToolResultsCleared > 0 || s.ThinkingBlocksCleared > 0
}

func (s openAIAnthropicContextControlStats) seen() bool {
	return s.ContextHintSeen || s.ContextManagementSeen
}

type anthropicContextEdit struct {
	Type string          `json:"type"`
	Keep json.RawMessage `json:"keep"`
}

type anthropicContextManagement struct {
	Edits []anthropicContextEdit `json:"edits"`
}

// applyOpenAIAnthropicContextControls consumes only the context-editing
// strategies that can be represented safely in a replayed Messages history.
// Native Anthropic paths keep the original raw fields and continue to use the
// upstream implementation.  This helper is called only by OpenAI Messages
// compatibility paths.
func applyOpenAIAnthropicContextControls(req *apicompat.AnthropicRequest) openAIAnthropicContextControlStats {
	if req == nil {
		return openAIAnthropicContextControlStats{}
	}
	stats := openAIAnthropicContextControlStats{}
	stats.ContextHintSeen = len(req.ContextHint) > 0
	stats.ContextManagementSeen = len(req.ContextManagement) > 0
	toolKeepRecent := -1
	targetTokens := 0
	if enabled, hintTargetTokens := parseAnthropicContextHint(req.ContextHint); enabled {
		toolKeepRecent = defaultContextHintKeepRecentToolResults
		targetTokens = hintTargetTokens
	}

	var management anthropicContextManagement
	if len(req.ContextManagement) > 0 && json.Unmarshal(req.ContextManagement, &management) == nil {
		for _, edit := range management.Edits {
			switch strings.TrimSpace(edit.Type) {
			case "clear_tool_uses_20250919":
				keepRecent := contextKeepRecent(edit.Keep, defaultContextHintKeepRecentToolResults)
				if keepRecent >= 0 {
					if toolKeepRecent < 0 || keepRecent < toolKeepRecent {
						toolKeepRecent = keepRecent
					}
				}
			case "clear_thinking_20251015":
				keepRecent := contextKeepRecent(edit.Keep, defaultContextManagementKeepRecentThinking)
				if keepRecent >= 0 {
					stats.ThinkingBlocksCleared += clearOldAnthropicThinking(req, keepRecent)
				}
			}
		}
	}
	if toolKeepRecent >= 0 {
		stats.ToolResultsCleared = clearOldAnthropicToolResults(req, toolKeepRecent, targetTokens)
	}

	return stats
}

func parseAnthropicContextHint(raw json.RawMessage) (enabled bool, targetTokens int) {
	if len(raw) == 0 {
		return false, 0
	}
	var hint struct {
		Enabled           bool `json:"enabled"`
		TargetTokensSaved int  `json:"target_tokens_saved"`
	}
	if err := json.Unmarshal(raw, &hint); err != nil {
		return false, 0
	}
	return hint.Enabled, contextMaxInt(hint.TargetTokensSaved, 0)
}

// contextKeepRecent accepts both the compact string form ("all") and the
// Anthropic object form ({"type":"tool_uses","value":3}).  A negative
// return value means “keep all”.
func contextKeepRecent(raw json.RawMessage, defaultValue int) int {
	if len(raw) == 0 || string(raw) == "null" {
		return defaultValue
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		if strings.EqualFold(strings.TrimSpace(text), "all") {
			return -1
		}
		if n, err := strconv.Atoi(strings.TrimSpace(text)); err == nil {
			return contextMaxInt(n, 0)
		}
	}
	var number int
	if json.Unmarshal(raw, &number) == nil {
		return contextMaxInt(number, 0)
	}
	var object struct {
		Value int    `json:"value"`
		Type  string `json:"type"`
	}
	if json.Unmarshal(raw, &object) == nil {
		if strings.EqualFold(strings.TrimSpace(object.Type), "all") {
			return -1
		}
		if object.Value > 0 {
			return object.Value
		}
	}
	return defaultValue
}

type anthropicMessageBlocks struct {
	blocks []map[string]any
	valid  bool
}

type anthropicToolResultRef struct {
	messageIndex int
	blockIndex   int
	savedBytes   int
}

func decodeAnthropicMessageBlocks(req *apicompat.AnthropicRequest) []anthropicMessageBlocks {
	decoded := make([]anthropicMessageBlocks, len(req.Messages))
	for i, message := range req.Messages {
		var blocks []map[string]any
		if len(message.Content) == 0 || json.Unmarshal(message.Content, &blocks) != nil {
			continue
		}
		decoded[i] = anthropicMessageBlocks{blocks: blocks, valid: true}
	}
	return decoded
}

func clearOldAnthropicToolResults(req *apicompat.AnthropicRequest, keepRecent, targetTokens int) int {
	if req == nil || keepRecent < 0 {
		return 0
	}
	decoded := decodeAnthropicMessageBlocks(req)
	refs := make([]anthropicToolResultRef, 0)
	for messageIndex, message := range decoded {
		if !message.valid {
			continue
		}
		for blockIndex, block := range message.blocks {
			if strings.TrimSpace(stringValue(block["type"])) != "tool_result" {
				continue
			}
			contentBytes, _ := json.Marshal(block["content"])
			refs = append(refs, anthropicToolResultRef{
				messageIndex: messageIndex,
				blockIndex:   blockIndex,
				savedBytes:   contextMaxInt(len(contentBytes)-len(contextHintToolResultPlaceholder), 0),
			})
		}
	}
	if len(refs) <= keepRecent {
		return 0
	}

	eligible := refs[:len(refs)-keepRecent]
	needBytes := targetTokens * 4
	cleared := 0
	saved := 0
	changedMessages := make(map[int]struct{})
	for _, ref := range eligible {
		block := decoded[ref.messageIndex].blocks[ref.blockIndex]
		block["content"] = contextHintToolResultPlaceholder
		changedMessages[ref.messageIndex] = struct{}{}
		cleared++
		saved += ref.savedBytes
		if needBytes > 0 && saved >= needBytes {
			break
		}
	}
	for messageIndex := range changedMessages {
		if encoded, err := json.Marshal(decoded[messageIndex].blocks); err == nil {
			req.Messages[messageIndex].Content = encoded
		}
	}
	return cleared
}

func clearOldAnthropicThinking(req *apicompat.AnthropicRequest, keepRecent int) int {
	if req == nil || keepRecent < 0 {
		return 0
	}
	decoded := decodeAnthropicMessageBlocks(req)
	assistantIndices := make([]int, 0)
	for i, message := range req.Messages {
		if message.Role == "assistant" && decoded[i].valid {
			assistantIndices = append(assistantIndices, i)
		}
	}
	keepFrom := 0
	if len(assistantIndices) > keepRecent {
		keepFrom = len(assistantIndices) - keepRecent
	}
	keepMessages := make(map[int]struct{}, keepRecent)
	for _, index := range assistantIndices[keepFrom:] {
		keepMessages[index] = struct{}{}
	}

	cleared := 0
	for messageIndex, message := range decoded {
		if !message.valid {
			continue
		}
		if _, keep := keepMessages[messageIndex]; keep {
			continue
		}
		filtered := make([]map[string]any, 0, len(message.blocks))
		changed := false
		for _, block := range message.blocks {
			blockType := strings.TrimSpace(stringValue(block["type"]))
			if blockType == "thinking" || blockType == "redacted_thinking" {
				cleared++
				changed = true
				continue
			}
			filtered = append(filtered, block)
		}
		if !changed {
			continue
		}
		if len(filtered) == 0 {
			filtered = append(filtered, map[string]any{
				"type": "text",
				"text": contextManagementThinkingPlaceholder,
			})
		}
		if encoded, err := json.Marshal(filtered); err == nil {
			req.Messages[messageIndex].Content = encoded
		}
	}
	return cleared
}

func contextMaxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func contextControlDescription(stats openAIAnthropicContextControlStats) string {
	return fmt.Sprintf("context_hint_seen=%t context_management_seen=%t tool_results_cleared=%d thinking_blocks_cleared=%d",
		stats.ContextHintSeen, stats.ContextManagementSeen, stats.ToolResultsCleared, stats.ThinkingBlocksCleared)
}

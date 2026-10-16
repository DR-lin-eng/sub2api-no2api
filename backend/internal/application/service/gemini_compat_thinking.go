package service

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type geminiThinkingParam int

const (
	geminiThinkingParamNone geminiThinkingParam = iota
	geminiThinkingParamBudget
	geminiThinkingParamLevel
)

const (
	gemini25ProMaxThinkingBudget       = 32768
	gemini25ProMinThinkingBudget       = 128
	gemini25FlashMaxThinkingBudget     = 24576
	gemini25FlashLiteMinThinkingBudget = 512
)

func geminiThinkingParamForModel(model string) geminiThinkingParam {
	name := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(model)), "models/")
	rest, ok := strings.CutPrefix(name, "gemini-")
	if !ok {
		return geminiThinkingParamNone
	}
	for _, kind := range []string{"image", "tts", "embedding", "live", "audio"} {
		if strings.Contains(rest, kind) {
			return geminiThinkingParamNone
		}
	}
	version, _, _ := strings.Cut(rest, "-")
	majorText, minorText, _ := strings.Cut(version, ".")
	major, err := strconv.Atoi(majorText)
	if err != nil {
		return geminiThinkingParamNone
	}
	minor := 0
	if minorText != "" {
		if minor, err = strconv.Atoi(minorText); err != nil {
			return geminiThinkingParamNone
		}
	}
	switch {
	case major >= 3:
		return geminiThinkingParamLevel
	case major == 2 && minor >= 5:
		return geminiThinkingParamBudget
	default:
		return geminiThinkingParamNone
	}
}

func geminiModelNameCarriesThinkingTier(model string) bool {
	name := strings.ToLower(strings.TrimSpace(model))
	for _, suffix := range []string{"-minimal", "-low", "-medium", "-high", "-xhigh", "-max", "-tiered"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

type claudeThinkingIntent struct {
	thinkingType string
	tier         string
	budgetTokens int
}

func parseClaudeThinkingIntent(claudeBody []byte) claudeThinkingIntent {
	intent := claudeThinkingIntent{
		thinkingType: strings.ToLower(strings.TrimSpace(gjson.GetBytes(claudeBody, "thinking.type").String())),
	}
	if intent.thinkingType == "disabled" {
		intent.tier = "minimal"
		return intent
	}
	switch strings.ToLower(strings.TrimSpace(gjson.GetBytes(claudeBody, "output_config.effort").String())) {
	case "none", "minimal":
		intent.tier = "minimal"
	case "low":
		intent.tier = "low"
	case "medium":
		intent.tier = "medium"
	case "high", "xhigh", "max":
		intent.tier = "high"
	}
	if intent.thinkingType == "enabled" {
		intent.budgetTokens = int(gjson.GetBytes(claudeBody, "thinking.budget_tokens").Int())
	}
	return intent
}

func claudeRequestEnablesThinking(claudeBody []byte) bool {
	switch parseClaudeThinkingIntent(claudeBody).thinkingType {
	case "enabled", "adaptive":
		return true
	default:
		return false
	}
}

func geminiThinkingLevelFor(model string, intent claudeThinkingIntent) string {
	tier := intent.tier
	if tier == "" && intent.budgetTokens > 0 {
		switch {
		case intent.budgetTokens <= 1024:
			tier = "low"
		case intent.budgetTokens <= 8192:
			tier = "medium"
		default:
			tier = "high"
		}
	}
	name := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(model)), "models/")
	isFlash := strings.Contains(name, "flash")
	if tier == "minimal" && !isFlash {
		tier = "low"
	}
	if tier == "medium" && !isFlash && !strings.HasPrefix(name, "gemini-3.1-pro") {
		tier = "high"
	}
	return tier
}

func geminiThinkingBudgetFor(model string, intent claudeThinkingIntent) (budget int, ok bool) {
	name := strings.ToLower(model)
	isPro := strings.Contains(name, "pro")
	maxBudget, minPositive := gemini25FlashMaxThinkingBudget, 1
	switch {
	case isPro:
		maxBudget, minPositive = gemini25ProMaxThinkingBudget, gemini25ProMinThinkingBudget
	case strings.Contains(name, "flash-lite"):
		minPositive = gemini25FlashLiteMinThinkingBudget
	}
	switch intent.tier {
	case "minimal":
		if isPro {
			return gemini25ProMinThinkingBudget, true
		}
		return 0, true
	case "low":
		budget = 1024
	case "medium":
		budget = 8192
	case "high":
		budget = maxBudget
	default:
		if intent.budgetTokens <= 0 {
			return 0, false
		}
		budget = intent.budgetTokens
	}
	budget = min(max(budget, minPositive), maxBudget)
	return budget, true
}

// Build the small thinkingConfig once and patch the request once. Repeated
// sjson writes would copy an entire multi-megabyte conversation per field.
func applyGeminiCompatThinkingConfig(geminiReq, claudeBody []byte, upstreamModel string, includeThoughts bool) []byte {
	param := geminiThinkingParamForModel(upstreamModel)
	if param == geminiThinkingParamNone {
		return geminiReq
	}
	intent := parseClaudeThinkingIntent(claudeBody)
	thinking := make(map[string]any, 2)
	if existing := gjson.GetBytes(geminiReq, "generationConfig.thinkingConfig"); existing.IsObject() {
		if err := json.Unmarshal([]byte(existing.Raw), &thinking); err != nil {
			return geminiReq
		}
	}
	thinkingOff := false
	if !geminiModelNameCarriesThinkingTier(upstreamModel) {
		switch param {
		case geminiThinkingParamLevel:
			if level := geminiThinkingLevelFor(upstreamModel, intent); level != "" {
				thinking["thinkingLevel"] = level
			}
		case geminiThinkingParamBudget:
			if budget, ok := geminiThinkingBudgetFor(upstreamModel, intent); ok {
				thinking["thinkingBudget"] = budget
				thinkingOff = budget == 0
			}
		}
	}
	if includeThoughts && !thinkingOff {
		thinking["includeThoughts"] = true
	}
	if len(thinking) == 0 {
		return geminiReq
	}
	updated, err := sjson.SetBytes(geminiReq, "generationConfig.thinkingConfig", thinking)
	if err != nil {
		return geminiReq
	}
	return updated
}

func extractGeminiReasoningEffortFromBody(body []byte) *string {
	if level := strings.TrimSpace(gjson.GetBytes(body, "generationConfig.thinkingConfig.thinkingLevel").String()); level != "" {
		return &level
	}
	if budget := gjson.GetBytes(body, "generationConfig.thinkingConfig.thinkingBudget"); budget.Exists() {
		value := "high"
		switch {
		case budget.Int() <= 0:
			value = "none"
		case budget.Int() <= 1024:
			value = "low"
		case budget.Int() <= 8192:
			value = "medium"
		}
		return &value
	}
	return nil
}

func isGeminiThoughtPart(part map[string]any) bool {
	thought, _ := part["thought"].(bool)
	return thought
}

func dropClaudeHistoryThinkingBlocks(claudeBody []byte) []byte {
	var req map[string]any
	if err := json.Unmarshal(claudeBody, &req); err != nil {
		return claudeBody
	}
	messages, ok := req["messages"].([]any)
	if !ok {
		return claudeBody
	}
	changed := false
	kept := make([]any, 0, len(messages))
	for _, message := range messages {
		mm, ok := message.(map[string]any)
		if !ok {
			kept = append(kept, message)
			continue
		}
		blocks, ok := mm["content"].([]any)
		if !ok {
			kept = append(kept, message)
			continue
		}
		filtered := make([]any, 0, len(blocks))
		for _, block := range blocks {
			if bm, ok := block.(map[string]any); ok {
				if kind, _ := bm["type"].(string); kind == "thinking" || kind == "redacted_thinking" {
					continue
				}
			}
			filtered = append(filtered, block)
		}
		if len(filtered) == len(blocks) {
			kept = append(kept, message)
			continue
		}
		nonBlank := filtered[:0]
		for _, block := range filtered {
			if bm, ok := block.(map[string]any); ok {
				if kind, _ := bm["type"].(string); kind == "text" {
					if text, _ := bm["text"].(string); strings.TrimSpace(text) == "" {
						continue
					}
				}
			}
			nonBlank = append(nonBlank, block)
		}
		changed = true
		if len(nonBlank) == 0 {
			continue
		}
		mm["content"] = nonBlank
		kept = append(kept, mm)
	}
	if !changed {
		return claudeBody
	}
	req["messages"] = kept
	out, err := json.Marshal(req)
	if err != nil {
		return claudeBody
	}
	return out
}

func dropChatReasoningContentParts(content json.RawMessage) json.RawMessage {
	if !bytes.HasPrefix(bytes.TrimSpace(content), []byte("[")) {
		return content
	}
	var parts []json.RawMessage
	if err := json.Unmarshal(content, &parts); err != nil {
		return content
	}
	kept := make([]json.RawMessage, 0, len(parts))
	for _, part := range parts {
		kind := gjson.GetBytes(part, "type").String()
		if kind == "thinking" || kind == "reasoning" {
			continue
		}
		kept = append(kept, part)
	}
	if len(kept) == len(parts) {
		return content
	}
	out, err := json.Marshal(kept)
	if err != nil {
		return content
	}
	return out
}

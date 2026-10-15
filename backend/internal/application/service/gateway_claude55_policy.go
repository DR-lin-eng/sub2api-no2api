package service

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/Wei-Shaw/sub2api/internal/shared/claude"
	"github.com/tidwall/gjson"
)

func isClaude55SignedThinkingModel(model string) bool {
	return claude.IsOpus55(model) || claude.IsSonnet55(model) || claude.IsHaiku55(model)
}

func validateClaude55Request(body []byte, model string) error {
	if !isClaude55SignedThinkingModel(model) {
		return nil
	}
	fields := make(map[string]gjson.Result, 6)
	// The read-only view is local to validation. No result escapes or mutates
	// body; avoid copying MiB histories just to inspect six small fields.
	gjson.Parse(unsafe.String(unsafe.SliceData(body), len(body))).ForEach(func(key, value gjson.Result) bool {
		switch key.String() {
		case "thinking", "output_config", "tool_choice", "temperature", "top_p", "top_k":
			fields[key.String()] = value
		}
		return true
	})
	get := func(path string) gjson.Result {
		key, rest, nested := strings.Cut(path, ".")
		v := fields[key]
		if nested {
			return v.Get(rest)
		}
		return v
	}
	if effort := get("output_config.effort"); effort.Exists() {
		switch effort.String() {
		case "low", "medium", "high", "xhigh", "max":
		default:
			return fmt.Errorf("%s has unsupported output_config.effort", model)
		}
	}
	if mode := get("thinking.type"); mode.Exists() {
		switch mode.String() {
		case "adaptive", "enabled", "disabled", "between_tools":
		default:
			return fmt.Errorf("%s has unsupported thinking.type", model)
		}
	}
	isSonnet55 := claude.IsSonnet55(model)
	isHaiku55 := claude.IsHaiku55(model)
	thinkingType := get("thinking.type").String()
	switch thinkingType {
	case "disabled", "enabled":
		if isHaiku55 {
			// haiku-5-5：enabled 一律 400（用 adaptive）；disabled 仅 high 及以下
			// 可用，xhigh/max 上 400。见官方 migration guide。
			if thinkingType == "enabled" {
				return fmt.Errorf("claude-haiku-5-5 requires adaptive thinking; omit thinking or use thinking.type=adaptive and output_config.effort")
			}
			switch get("output_config.effort").String() {
			case "xhigh", "max":
				return fmt.Errorf("claude-haiku-5-5 thinking.type=disabled supports only low, medium or high effort")
			}
			break
		}
		if isSonnet55 {
			return fmt.Errorf("claude-sonnet-5-5 requires adaptive thinking or thinking.type=between_tools; omit thinking or use one of those modes")
		}
		return fmt.Errorf("claude-opus-5-5 requires adaptive thinking; omit thinking or use thinking.type=adaptive and output_config.effort")
	case "between_tools":
		if !isSonnet55 {
			if isHaiku55 {
				return fmt.Errorf("claude-haiku-5-5 requires adaptive thinking; thinking.type=between_tools is unsupported (use thinking.type=disabled at high effort or below)")
			}
			return fmt.Errorf("claude-opus-5-5 requires adaptive thinking; thinking.type=between_tools is unsupported")
		}
		effort := get("output_config.effort").String()
		if effort == "xhigh" || effort == "max" {
			return fmt.Errorf("claude-sonnet-5-5 thinking.type=between_tools supports only low, medium or high effort")
		}
		for _, field := range []string{"thinking.display", "thinking.budget_tokens", "thinking.block_binding"} {
			if get(field).Exists() {
				return fmt.Errorf("claude-sonnet-5-5 thinking.type=between_tools does not support %s", field)
			}
		}
	}
	modelName := "claude-opus-5-5"
	if isSonnet55 {
		modelName = "claude-sonnet-5-5"
	}
	if isHaiku55 {
		modelName = "claude-haiku-5-5"
	}
	// haiku-5-5 接受强制 tool_choice（响应以 tool call 开头、无 thinking 块），
	// 与 opus/sonnet-5-5 不同，不拦截。见官方 migration guide。
	if !isHaiku55 {
		if get("tool_choice").String() == "required" {
			return fmt.Errorf("%s does not support forced tool_choice; use auto or none", modelName)
		}
		switch get("tool_choice.type").String() {
		case "any", "tool", "function", "custom", "namespace":
			return fmt.Errorf("%s does not support forced tool_choice; use auto or none", modelName)
		}
	}
	if isSonnet55 || isHaiku55 {
		if temperature := get("temperature"); temperature.Exists() && (temperature.Type != gjson.Number || temperature.Float() != 1) {
			return fmt.Errorf("%s does not support non-default temperature", modelName)
		}
		// Haiku 5.5 只接受显式 top_p=0.99；保留 Sonnet 5.5 既有的区间策略。
		if topP := get("top_p"); topP.Exists() && (topP.Type != gjson.Number ||
			(isHaiku55 && topP.Float() != 0.99) || (isSonnet55 && (topP.Float() < 0.99 || topP.Float() > 1))) {
			return fmt.Errorf("%s does not support non-default top_p", modelName)
		}
		if get("top_k").Exists() {
			return fmt.Errorf("%s does not support top_k", modelName)
		}
		if isHaiku55 && get("temperature").Exists() && get("top_p").Exists() {
			return fmt.Errorf("%s does not allow temperature and top_p together; omit both", modelName)
		}
	}
	return nil
}

// Resolve before conversion: aliases do not identify the upstream thinking policy.
// Keep the original request model separately for response and billing contracts.
func resolveClaudeForwardModel(account *Account, model string) string {
	if account == nil {
		return model
	}
	if account.IsBedrock() {
		if mapped, ok := ResolveBedrockModelID(account, model); ok {
			return mapped
		}
	}
	mapped := model
	if account.Type == AccountTypeAPIKey || account.Type == AccountTypeServiceAccount {
		mapped = account.GetMappedModel(model)
	}
	if mapped == model && account.Platform == PlatformAnthropic && account.Type == AccountTypeServiceAccount {
		mapped = normalizeVertexAnthropicModelID(claude.NormalizeModelID(model))
	} else if mapped == model && account.Platform == PlatformAnthropic && account.Type != AccountTypeAPIKey {
		mapped = claude.NormalizeModelID(model)
	}
	return mapped
}

package service
import("fmt";"strings";"testing";"github.com/Wei-Shaw/sub2api/internal/shared/claude";"github.com/tidwall/gjson")
func upstreamValidateClaude55Request(body []byte, model string) error {
	if !isClaude55SignedThinkingModel(model) {
		return nil
	}
	isSonnet55 := claude.IsSonnet55(model)
	isHaiku55 := claude.IsHaiku55(model)
	thinkingType := gjson.GetBytes(body, "thinking.type").String()
	switch thinkingType {
	case "disabled", "enabled":
		if isHaiku55 {
			// haiku-5-5：enabled 一律 400（用 adaptive）；disabled 仅 high 及以下
			// 可用，xhigh/max 上 400。见官方 migration guide。
			if thinkingType == "enabled" {
				return fmt.Errorf("claude-haiku-5-5 requires adaptive thinking; omit thinking or use thinking.type=adaptive and output_config.effort")
			}
			switch gjson.GetBytes(body, "output_config.effort").String() {
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
		effort := gjson.GetBytes(body, "output_config.effort").String()
		if effort == "xhigh" || effort == "max" {
			return fmt.Errorf("claude-sonnet-5-5 thinking.type=between_tools supports only low, medium or high effort")
		}
		for _, field := range []string{"thinking.display", "thinking.budget_tokens", "thinking.block_binding"} {
			if gjson.GetBytes(body, field).Exists() {
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
		if gjson.GetBytes(body, "tool_choice").String() == "required" {
			return fmt.Errorf("%s does not support forced tool_choice; use auto or none", modelName)
		}
		switch gjson.GetBytes(body, "tool_choice.type").String() {
		case "any", "tool", "function", "custom", "namespace":
			return fmt.Errorf("%s does not support forced tool_choice; use auto or none", modelName)
		}
	}
	if isSonnet55 || isHaiku55 {
		if temperature := gjson.GetBytes(body, "temperature"); temperature.Exists() && (temperature.Type != gjson.Number || temperature.Float() != 1) {
			return fmt.Errorf("%s does not support non-default temperature", modelName)
		}
		// Haiku 5.5 只接受显式 top_p=0.99；保留 Sonnet 5.5 既有的区间策略。
		if topP := gjson.GetBytes(body, "top_p"); topP.Exists() && (topP.Type != gjson.Number ||
			(isHaiku55 && topP.Float() != 0.99) || (isSonnet55 && (topP.Float() < 0.99 || topP.Float() > 1))) {
			return fmt.Errorf("%s does not support non-default top_p", modelName)
		}
		if gjson.GetBytes(body, "top_k").Exists() {
			return fmt.Errorf("%s does not support top_k", modelName)
		}
		if isHaiku55 && gjson.GetBytes(body, "temperature").Exists() && gjson.GetBytes(body, "top_p").Exists() {
			return fmt.Errorf("%s does not allow temperature and top_p together; omit both", modelName)
		}
	}
	return nil
}
func BenchmarkOpenClaude55Validation(b *testing.B){body:=[]byte(`{"messages":[{"role":"user","content":"`+strings.Repeat("x",1<<20)+`"}],"thinking":{"type":"adaptive"},"output_config":{"effort":"high"}}`);for _,ref:=range []bool{true,false}{name:="native";if ref{name="upstream"};b.Run(name,func(b *testing.B){b.ReportAllocs();b.SetBytes(int64(len(body)));for i:=0;i<b.N;i++{var err error;if ref{err=upstreamValidateClaude55Request(body,"claude-sonnet-5-5")}else{err=validateClaude55Request(body,"claude-sonnet-5-5")};if err!=nil{b.Fatal(err)}}})}}

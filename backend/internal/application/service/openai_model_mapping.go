package service

import "strings"

// resolveOpenAIForwardModel 解析 OpenAI 兼容转发使用的模型。
// messagesDispatchMappedModel 是调用方已为 /v1/messages 解析的显式调度结果；
// 普通 OpenAI 请求必须传空，避免将分组配置作为通用模型兜底。
func resolveOpenAIForwardModel(account *Account, requestedModel, messagesDispatchMappedModel string) string {
	messagesDispatchMappedModel = strings.TrimSpace(messagesDispatchMappedModel)
	if account == nil {
		if messagesDispatchMappedModel != "" {
			return messagesDispatchMappedModel
		}
		return requestedModel
	}

	mappedModel, matched := account.ResolveMappedModel(requestedModel)
	if !matched && messagesDispatchMappedModel != "" {
		return messagesDispatchMappedModel
	}
	return mappedModel
}

// openAIOAuthForeignModelPrefixes 列出明确属于其他厂商家族的模型名前缀。
// Codex 上游不可能服务这些模型：转发阶段 normalizeOpenAIModelForUpstream
// 对未知模型原样透传，上游必然返回不可重试的 400。
var openAIOAuthForeignModelPrefixes = []string{
	"deepseek-",
	"glm-",
	"kimi-",
	"moonshot-",
	"qwen-",
	"qwen2-",
	"qwen3-",
	"qwen4-",
	"qwq-",
	"minimax-",
	"gemini-",
	"gemma-",
	"grok-",
	"doubao-",
	"hunyuan-",
	"llama-",
	"llama2-",
	"llama3-",
	"meta-llama",
	"mistral-",
	"mixtral-",
	"baichuan-",
	"ernie-",
	"step-",
	"seed-",
	"yi-",
}

// isOpenAIOAuthServableModel 判断「空 model_mapping 的 OpenAI OAuth 账号」能否
// 服务请求模型。只有 Codex 已知模型和 Claude Messages 的可归一化家族才放行；
// 真正未知的模型应在调度阶段交给 API Key/显式映射账号，避免先把原始别名发到
// Codex 后才得到不可重试的 400。渠道别名在调度前会以映射后的模型调用本函数。
func isOpenAIOAuthServableModel(requestedModel string) bool {
	model := strings.ToLower(lastOpenAIModelSegment(requestedModel))
	if model == "" {
		return true // 空模型交由上层必填校验处理
	}
	// Kimi Code 官方 bare model ID：无厂商前缀，prefix 黑名单挡不住。
	if model == "k3" || model == "k3-256k" {
		return false
	}
	for _, prefix := range openAIOAuthForeignModelPrefixes {
		if strings.HasPrefix(model, prefix) {
			return false
		}
	}
	if claudeMessagesDispatchFamily(model) != "" {
		return true
	}
	_, known := normalizeKnownCodexModel(model)
	return known
}

// resolveOpenAICompactForwardModel determines the compact-only upstream model
// for /responses/compact requests. It never affects normal /responses traffic.
// When no compact-specific mapping matches, the input model is returned as-is.
func resolveOpenAICompactForwardModel(account *Account, model string) string {
	trimmedModel := strings.TrimSpace(model)
	if trimmedModel == "" || account == nil {
		return trimmedModel
	}

	mappedModel, matched := account.ResolveCompactMappedModel(trimmedModel)
	if !matched {
		return trimmedModel
	}
	if trimmedMapped := strings.TrimSpace(mappedModel); trimmedMapped != "" {
		return trimmedMapped
	}
	return trimmedModel
}

func resolveOpenAICompactFallbackForwardModels(account *Account, requestedModel, mappedModel string) []string {
	if account == nil {
		return nil
	}
	primaryUpstreamModel := normalizeOpenAIModelForUpstream(account, mappedModel)
	rawCandidates := account.ResolveCompactFallbackModels(requestedModel, mappedModel)
	if len(rawCandidates) == 0 {
		return nil
	}

	result := make([]string, 0, len(rawCandidates))
	seen := make(map[string]bool, len(rawCandidates)+1)
	if primaryUpstreamModel != "" {
		seen[strings.ToLower(primaryUpstreamModel)] = true
	}
	for _, candidate := range rawCandidates {
		trimmed := strings.TrimSpace(candidate)
		if trimmed == "" {
			continue
		}
		upstreamModel := normalizeOpenAIModelForUpstream(account, trimmed)
		if upstreamModel == "" {
			continue
		}
		key := strings.ToLower(upstreamModel)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, upstreamModel)
	}
	return result
}

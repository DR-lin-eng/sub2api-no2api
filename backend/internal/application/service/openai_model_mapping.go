package service

import (
	"sort"
	"strings"
)

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

// HasExplicitModelMapping reports whether an account contains a non-empty
// administrator-provided model mapping.  Empty maps are intentionally treated
// as "no restriction", matching the account editor and historical routing
// semantics.
func (a *Account) HasExplicitModelMapping() bool {
	if a == nil || a.Credentials == nil {
		return false
	}
	switch raw := a.Credentials["model_mapping"].(type) {
	case map[string]any:
		for key, value := range raw {
			if strings.TrimSpace(key) == "" {
				continue
			}
			if target, ok := value.(string); ok && strings.TrimSpace(target) != "" {
				return true
			}
			if targets, ok := value.([]any); ok && len(targets) > 0 {
				return true
			}
		}
		return false
	case map[string]string:
		for key, value := range raw {
			if strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
				return true
			}
		}
		return false
	case map[string][]string:
		for key, values := range raw {
			if strings.TrimSpace(key) != "" && len(values) > 0 {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// OpenAIOAuthSupportedModels returns the last successful automatic model
// capability snapshot.  The second return value is false when no usable
// snapshot is present, so callers can retain the conservative static fallback
// during first startup or after an upstream failure.
func (a *Account) OpenAIOAuthSupportedModels() ([]string, bool) {
	if a == nil || !a.IsOpenAIOAuth() || a.Extra == nil {
		return nil, false
	}
	models := normalizeOAuthSupportedModelValues(a.Extra[OAuthSupportedModelsExtraKey])
	if len(models) == 0 {
		models = normalizeOAuthSupportedModelValues(a.Extra[OpenAIOAuthSupportedModelsExtraKey])
	}
	if len(models) == 0 {
		return nil, false
	}
	return models, true
}

// OAuthSupportedModels returns a capability snapshot for any OAuth-backed
// platform. OpenAI callers should use OpenAIOAuthSupportedModels when they
// need the legacy OpenAI-specific key fallback.
func (a *Account) OAuthSupportedModels() ([]string, bool) {
	if a == nil || !a.IsOAuth() || a.Extra == nil {
		return nil, false
	}
	models := normalizeOAuthSupportedModelValues(a.Extra[OAuthSupportedModelsExtraKey])
	if len(models) == 0 && a.IsOpenAIOAuth() {
		models = normalizeOAuthSupportedModelValues(a.Extra[OpenAIOAuthSupportedModelsExtraKey])
	}
	if len(models) == 0 {
		return nil, false
	}
	return models, true
}

func normalizeOAuthSupportedModelValues(raw any) []string {
	var values []string
	switch typed := raw.(type) {
	case []string:
		values = append(values, typed...)
	case []any:
		values = make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := item.(string); ok {
				values = append(values, value)
			}
		}
	case map[string]any:
		// Accept the object form for forward compatibility with snapshots
		// written by an intermediate release. Only keys with a truthy value are
		// considered model IDs; ordinary string values are accepted as well.
		for key, item := range typed {
			switch value := item.(type) {
			case bool:
				if value {
					values = append(values, key)
				}
			case string:
				if strings.TrimSpace(value) != "" {
					values = append(values, key)
				}
			}
		}
	case map[string]string:
		for key, value := range typed {
			if strings.TrimSpace(value) != "" {
				values = append(values, key)
			}
		}
	}

	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(lastOpenAIModelSegment(trimmed))
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, trimmed)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return strings.ToLower(result[i]) < strings.ToLower(result[j])
	})
	return result
}

// isOpenAIOAuthSyncedModel reports whether a request model is present in the
// account's live capability snapshot.  Known Claude Messages models remain
// eligible because OpenAI OAuth can serve that compatibility path even when
// the Codex manifest only enumerates Codex slugs.  All other model families
// must match the observed manifest (including image models), preventing a
// stale static catalog entry from reaching an upstream that has removed it.
func isOpenAIOAuthSyncedModel(requestedModel string, supportedModels []string) bool {
	model := strings.TrimSpace(requestedModel)
	if model == "" {
		return true
	}
	lowerModel := strings.ToLower(lastOpenAIModelSegment(model))
	if lowerModel == "" {
		return true
	}
	if lowerModel == "k3" || lowerModel == "k3-256k" {
		return false
	}
	for _, prefix := range openAIOAuthForeignModelPrefixes {
		if strings.HasPrefix(lowerModel, prefix) {
			return false
		}
	}
	if claudeMessagesDispatchFamily(lowerModel) != "" {
		return true
	}

	manifestSet := make(map[string]struct{}, len(supportedModels)*2)
	for _, supported := range supportedModels {
		key := strings.ToLower(lastOpenAIModelSegment(supported))
		if key == "" {
			continue
		}
		manifestSet[key] = struct{}{}
		if normalized, ok := normalizeKnownCodexModel(key); ok {
			manifestSet[strings.ToLower(normalized)] = struct{}{}
		}
	}
	if _, ok := manifestSet[lowerModel]; ok {
		return true
	}
	if normalized, ok := normalizeKnownCodexModel(lowerModel); ok {
		_, exists := manifestSet[strings.ToLower(normalized)]
		return exists
	}
	return false
}

func isOAuthSyncedModel(account *Account, requestedModel, routedModel string, supportedModels []string) bool {
	model := firstNonEmptyModel(routedModel, requestedModel)
	if account != nil && account.IsOpenAIOAuth() {
		return isOpenAIOAuthSyncedModel(model, supportedModels)
	}
	candidates := []string{model}
	if account != nil && !account.HasExplicitModelMapping() {
		if mapped := strings.TrimSpace(account.GetMappedModel(model)); mapped != "" && mapped != model {
			candidates = append(candidates, mapped)
		}
	}
	for _, candidate := range candidates {
		modelKey := strings.ToLower(lastOpenAIModelSegment(candidate))
		if modelKey == "" {
			return true
		}
		for _, supported := range supportedModels {
			supportedKey := strings.ToLower(lastOpenAIModelSegment(supported))
			if supportedKey == modelKey || matchWildcard(supportedKey, modelKey) {
				return true
			}
		}
	}
	return false
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

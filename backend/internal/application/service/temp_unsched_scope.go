package service

import (
	"github.com/tidwall/gjson"
	"net/http"
	"strings"
)

// tempUnschedRulesApply 决定本次错误是否交给临时不可调度规则处理。
// 401 默认留给认证错误处理；只有首条命中规则按模型级生效时，才由规则只冷却该模型。
func tempUnschedRulesApply(account *Account, statusCode int, responseBody []byte, modelKey string) bool {
	if statusCode != http.StatusUnauthorized {
		return true
	}
	if strings.TrimSpace(modelKey) == "" || unauthorizedRequiresCredentialHandling(account, responseBody) {
		return false
	}
	matches := matchTempUnschedulableRules(account, statusCode, responseBody)
	return len(matches) > 0 && tempUnschedHitUsesModelScope(account, matches[0].rule, modelKey, statusCode, responseBody)
}

// tempUnschedHitUsesModelScope 决定一次命中是否按模型级冷却：凭据级 401 始终按账号处理，
// 其余由规则作用范围决定。
func tempUnschedHitUsesModelScope(account *Account, rule TempUnschedulableRule, modelKey string, statusCode int, responseBody []byte) bool {
	if statusCode == http.StatusUnauthorized && unauthorizedRequiresCredentialHandling(account, responseBody) {
		return false
	}
	return tempUnschedRuleUsesModelScope(rule, modelKey, statusCode)
}

// unauthorizedRequiresCredentialHandling 识别凭据级 401：OAuth 账号与 Spark 影子
// （失效缓存、刷新窗口、缺 refresh_token 停用），以及 OpenAI 明确的凭据失效
// （token_invalidated / token_revoked、{"detail":"Unauthorized"}）。
func unauthorizedRequiresCredentialHandling(account *Account, responseBody []byte) bool {
	if account == nil {
		return false
	}
	if account.Type == AccountTypeOAuth || account.IsShadow() {
		return true
	}
	if account.Platform != PlatformOpenAI {
		return false
	}
	switch extractUpstreamErrorCode(responseBody) {
	case "token_invalidated", "token_revoked":
		return true
	}
	return gjson.GetBytes(responseBody, "detail").String() == "Unauthorized"
}

// tempUnschedRuleUsesModelScope 决定一次命中按模型级冷却还是账号级暂停。
// 显式 scope 完全由规则决定；scope 为空时模型已知且非 401 走模型级。
// 模型未知时无法隔离，一律账号级。
func tempUnschedRuleUsesModelScope(rule TempUnschedulableRule, modelKey string, statusCode int) bool {
	if strings.TrimSpace(modelKey) == "" {
		return false
	}
	switch rule.Scope {
	case TempUnschedScopeAccount:
		return false
	case TempUnschedScopeModel:
		return true
	default:
		return statusCode != http.StatusUnauthorized
	}
}

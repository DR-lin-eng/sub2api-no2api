package service

import "strings"

// Exempt the upstream subscription model, independent of client/billing aliases.
func openAISubscriptionLongContextExempt(account *Account, result *OpenAIForwardResult) bool {
	if account == nil || !account.IsOpenAI() || (account.Type != AccountTypeOAuth && account.Type != AccountTypeSetupToken) || result == nil {
		return false
	}
	model := strings.TrimSpace(result.UpstreamModel)
	if model == "" {
		model = result.Model
	}
	model = strings.ReplaceAll(strings.ToLower(strings.TrimSpace(model)), "_", "-")
	model = strings.TrimPrefix(model, "openai/")
	if model == "gpt-6.1-sol" {
		return true
	}
	suffix, ok := strings.CutPrefix(model, "gpt-6.1-sol-")
	if !ok {
		return false
	}
	switch suffix {
	case "none", "minimal", "low", "medium", "high", "xhigh", "max", "openai-compact":
		return true
	}
	return false
}

package service

import (
	"encoding/json"
	"strings"
)

// GPT-5.5 is retired from ChatGPT-backed Codex accounts. OpenAI API-key
// accounts remain eligible, so the decision is based on the upstream account,
// never on the key used by a client to call this gateway.
func isRetiredOpenAISubscriptionModel(account *Account, upstreamModel string) bool {
	if account == nil || !account.IsOpenAI() {
		return false
	}
	// Setup tokens are inference-only ChatGPT credentials. Shadows inherit the
	// parent's OAuth scope and therefore follow the same retirement boundary.
	if account.Type != AccountTypeOAuth && account.Type != AccountTypeSetupToken && !account.IsShadow() {
		return false
	}
	model := canonicalizeOpenAIModelAliasSpelling(upstreamModel)
	return model == "gpt-5.5" || strings.HasPrefix(model, "gpt-5.5-")
}

// openAIModelRetiredForAccounts reports retirement only when every eligible
// account that could serve the public model is retired. A valid API-key or
// explicit replacement account keeps the model visible in mixed groups.
func openAIModelRetiredForAccounts(accounts []Account, modelID string) bool {
	retired := false
	for i := range accounts {
		account := &accounts[i]
		if !account.IsOpenAI() {
			continue
		}
		target := resolveOpenAIAccountUpstreamModelForRequest(account, modelID, false)
		if isRetiredOpenAISubscriptionModel(account, target) {
			retired = true
			continue
		}
		if account.IsModelSupported(modelID) {
			return false
		}
	}
	return retired
}

// Reuse Forward's single mapping step, including channel and compact-only
// targets. The normal mapping/snapshot policy remains the final support gate.
func supportsOpenAIRequestModel(account *Account, requestedModel, routedModel string, requireCompact bool) bool {
	if account == nil {
		return false
	}
	if account.IsOpenAI() && (account.IsOAuth() || account.IsShadow()) {
		target := resolveOpenAIAccountUpstreamModelForRequest(account, firstNonEmptyModel(routedModel, requestedModel), requireCompact)
		if isRetiredOpenAISubscriptionModel(account, target) {
			return false
		}
	}
	return account.isModelMappingSupportedForRequest(requestedModel, routedModel)
}

// Project first, then filter public slugs through the same routing chain.
// Preserve unknown entry/envelope metadata and the shared upstream body.
func filterRetiredCodexModelsBody(body []byte, account *Account) ([]byte, error) {
	if account == nil || !account.IsOpenAI() || (!account.IsOAuth() && !account.IsShadow()) {
		return body, nil
	}
	envelope, entries, err := codexMappingCatalogEntries(body)
	if err != nil {
		return nil, err
	}
	filtered := make([]json.RawMessage, 0, len(entries))
	for _, raw := range entries {
		var entry struct {
			Slug string `json:"slug"`
		}
		if json.Unmarshal(raw, &entry) != nil || !openAIModelRetiredForAccounts([]Account{*account}, entry.Slug) {
			filtered = append(filtered, raw)
		}
	}
	if len(filtered) == len(entries) {
		return body, nil
	}
	envelope["models"], err = json.Marshal(filtered)
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope)
}

func filterRetiredOpenAIModelIDs(accounts []Account, models []string) []string {
	filtered := make([]string, 0, len(models))
	for _, model := range models {
		if !openAIModelRetiredForAccounts(accounts, model) {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

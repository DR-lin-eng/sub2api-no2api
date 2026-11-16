package service

// Only an explicit boolean opt-in changes passthrough account eligibility.
// Mapping targets never rewrite a passthrough request.
func (a *Account) IsOpenAIPassthroughModelFilterEnabled() bool {
	return a.IsOpenAIPassthroughEnabled() && a.Extra["openai_passthrough_model_filter"] == true
}

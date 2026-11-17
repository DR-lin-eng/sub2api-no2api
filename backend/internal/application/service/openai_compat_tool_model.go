package service

import "strings"

func responsesToolModelBase(model string) string {
	if !strings.Contains(model, "6.1") && !strings.Contains(model, "6.2") {
		return ""
	}
	switch base := normalizeKnownOpenAICodexModel(model); base {
	case "gpt-6.2-astra", "gpt-6.2-sol", "gpt-6.1-sol", "gpt-6.1-terra":
		return base
	}
	return ""
}
func requiresResponsesForToolCalls(model string) bool { return responsesToolModelBase(model) != "" }

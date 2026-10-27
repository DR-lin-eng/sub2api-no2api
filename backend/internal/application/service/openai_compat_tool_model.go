package service

import "strings"

func gpt61CompatBase(model string) string {
	if !strings.Contains(model, "6.1") {
		return ""
	}
	switch base := normalizeKnownOpenAICodexModel(model); base {
	case "gpt-6.1-sol", "gpt-6.1-terra":
		return base
	}
	return ""
}
func requiresResponsesForToolCalls(model string) bool { return gpt61CompatBase(model) != "" }

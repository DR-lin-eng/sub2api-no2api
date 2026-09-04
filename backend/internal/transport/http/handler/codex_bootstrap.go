package handler

import "github.com/Wei-Shaw/sub2api/internal/shared/apicompat"

// Keep the handler-local entry points small and stable for callers/tests while
// the protocol parser itself is shared with the application and WS paths.
func normalizeCodexDelegationBootstrap(body []byte) ([]byte, bool) {
	normalized, kind, changed := apicompat.NormalizeCodexCallOutputBootstrap(body)
	if !changed || kind != apicompat.CodexBootstrapDelegation {
		return body, false
	}
	return normalized, true
}

func normalizeCodexAutomationBootstrap(body []byte) ([]byte, bool) {
	normalized, kind, changed := apicompat.NormalizeCodexCallOutputBootstrap(body)
	if !changed || kind != apicompat.CodexBootstrapAutomation {
		return body, false
	}
	return normalized, true
}

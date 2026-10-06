package service

import (
	"github.com/Wei-Shaw/sub2api/internal/modules/typesafe"
	"strings"
)

func (a *Account) GetTypeSafeBaseURL() string {
	if a == nil || !a.IsTypeSafe() || a.Type != AccountTypeAPIKey {
		return ""
	}
	baseURL := strings.TrimRight(strings.TrimSpace(a.GetCredential("base_url")), "/")
	// The System One path already carries /v1; accept a base URL pasted with it.
	if len(baseURL) >= 3 && strings.EqualFold(baseURL[len(baseURL)-3:], "/v1") {
		baseURL = strings.TrimRight(baseURL[:len(baseURL)-3], "/")
	}
	if baseURL == "" {
		return typesafe.DefaultBaseURL
	}
	return baseURL
}

func (a *Account) GetTypeSafeAPIKey() string {
	if a == nil || !a.IsTypeSafe() || a.Type != AccountTypeAPIKey {
		return ""
	}
	return strings.TrimSpace(a.GetCredential("api_key"))
}

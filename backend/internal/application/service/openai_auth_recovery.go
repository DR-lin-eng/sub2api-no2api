package service

import (
	"errors"
	"net/http"
	"strings"
)

// shouldRefreshOpenAIOAuth401 identifies an authentication response that may
// be caused by an early access-token expiry. A generic 401/Unauthorized body
// does not prove permanent revocation; give the refresh token one bounded
// attempt before applying the existing account failure policy.
//
// Explicit revocation, disabled-account and API-key-style codes remain
// terminal. A missing refresh token also remains terminal because the account
// cannot recover without user interaction.
func shouldRefreshOpenAIOAuth401(account *Account, statusCode int, responseBody []byte) bool {
	if statusCode != http.StatusUnauthorized || account == nil ||
		!account.IsOpenAIOAuth() || account.IsOpenAIPersonalAccessToken() ||
		strings.TrimSpace(account.GetOpenAIRefreshToken()) == "" {
		return false
	}

	switch strings.ToLower(strings.TrimSpace(extractUpstreamErrorCode(responseBody))) {
	case "token_revoked", "token_invalidated", "invalid_api_key", "account_deactivated", "account_disabled", "organization_deactivated", "invalid_grant", "invalid_refresh_token", "refresh_token_reused", "refresh_token_invalidated":
		return false
	default:
		return true
	}
}

func isOpenAIWSDialAuthRecoveryCandidate(err error, account *Account) bool {
	var dialErr *openAIWSDialError
	if !errors.As(err, &dialErr) || dialErr == nil {
		return false
	}
	return shouldRefreshOpenAIOAuth401(account, dialErr.StatusCode, dialErr.ResponseBody)
}

func isOpenAIWSTokenAuthRecoveryCandidate(payload []byte, account *Account) bool {
	codeRaw, typeRaw, messageRaw := parseOpenAIWSErrorEventFields(payload)
	code := strings.ToLower(strings.TrimSpace(codeRaw))
	typeName := strings.ToLower(strings.TrimSpace(typeRaw))
	message := strings.ToLower(strings.TrimSpace(messageRaw))
	statusCode := openAIWSErrorHTTPStatusFromRaw(codeRaw, typeRaw)
	if code == "token_expired" {
		statusCode = http.StatusUnauthorized
	} else if statusCode != http.StatusUnauthorized &&
		!strings.Contains(typeName, "authentication") &&
		!strings.Contains(code, "unauthorized") &&
		!strings.Contains(message, "unauthorized") &&
		!strings.Contains(message, "authentication") {
		return false
	} else if statusCode != http.StatusUnauthorized {
		statusCode = http.StatusUnauthorized
	}
	return shouldRefreshOpenAIOAuth401(account, statusCode, payload)
}

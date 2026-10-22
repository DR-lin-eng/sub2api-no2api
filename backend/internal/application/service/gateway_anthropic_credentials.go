package service

import "net/http"

const anthropicMessagesForeignOAuthReason GatewayFailureReason = "anthropic_messages_foreign_oauth"

// OAuth credentials are valid only for their issuer's native protocol.
// API keys retain their existing multi-protocol compatibility contract.
func anthropicMessagesCredentialAllowed(account *Account) bool {
	return account == nil || !account.IsOAuth() || account.Platform == PlatformAnthropic
}
func checkAnthropicMessagesCredential(account *Account) error {
	if anthropicMessagesCredentialAllowed(account) {
		return nil
	}
	return &UpstreamFailoverError{StatusCode: http.StatusBadGateway, Reason: anthropicMessagesForeignOAuthReason}
}

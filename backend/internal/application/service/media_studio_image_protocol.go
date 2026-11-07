package service

import "context"

// API-key-only pools retain native controls. Mixed or unknown pools expose the
// App's common controls so selecting an OAuth account does not discard options.
func (s *MediaStudioService) imageProtocolForGroup(ctx context.Context, group MediaStudioGroupOption, protocol string) string {
	if protocol != "chatgpt_app" || group.Platform != PlatformOpenAI || s.gatewayService == nil || s.gatewayService.accountRepo == nil {
		return protocol
	}
	accounts, err := s.gatewayService.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, group.GroupID, PlatformOpenAI)
	if err != nil || len(accounts) == 0 {
		return protocol
	}
	for index := range accounts {
		account := &accounts[index]
		if account.IsShadow() {
			return "chatgpt_app"
		}
		if account.IsOpenAIOAuth() && !account.IsOpenAISIWC() && !account.IsOpenAIPersonalAccessToken() && !account.IsOpenAIAgentIdentity() {
			return "chatgpt_app"
		}
	}
	return "legacy"
}

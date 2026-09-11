package service

import "context"

type OpenAISessionIDAdmissionDecision struct {
	Allowed           bool
	Existing          bool
	RetryAfterSeconds int
}

// OpenAISessionIDAdmissionCache is implemented by the Redis gateway cache.
// It is deliberately narrow so scheduler code does not depend on Redis types.
type OpenAISessionIDAdmissionCache interface {
	AdmitOpenAISessionID(ctx context.Context, accountID int64, sessionHash string, maxPerMinute int) (OpenAISessionIDAdmissionDecision, error)
}

func openAISessionIDRateLimitApplies(account *Account) bool {
	return account != nil && account.IsOpenAIOAuth()
}

func (s *OpenAIGatewayService) openAISessionIDRateLimitSettings(ctx context.Context) (bool, int) {
	settings := s.openAIAdvancedSchedulerRuntimeSettings(ctx)
	return settings.sessionIDRateLimitEnabled, settings.sessionIDRateLimitPerMinute
}

func (s *OpenAIGatewayService) admitOpenAISessionID(ctx context.Context, accountID int64, sessionHash string, maxPerMinute int) (OpenAISessionIDAdmissionDecision, error) {
	if s == nil || s.sessionIDAdmissionCache == nil {
		return OpenAISessionIDAdmissionDecision{Allowed: true}, nil
	}
	return s.sessionIDAdmissionCache.AdmitOpenAISessionID(ctx, accountID, sessionHash, maxPerMinute)
}

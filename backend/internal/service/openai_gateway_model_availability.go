package service

import (
	"context"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

// DiagnoseModelAvailabilityForPlatform reports whether the requested model
// is configured to be served by any OpenAI-compatible account in the group
// for the given platform (e.g. PlatformOpenAI, PlatformGrok). The platform
// scopes the candidate pool so distinct OpenAI-compatible platforms do not
// cross-contaminate diagnosis results.
//
// Safe to call on the error path: returns {true,true} on any internal
// failure or when the inputs preclude meaningful diagnosis (empty model,
// nil service), so callers stay on the 503 fallback branch.
func (s *OpenAIGatewayService) DiagnoseModelAvailabilityForPlatform(
	ctx context.Context,
	groupID *int64,
	requestedModel string,
	platform string,
) ModelAvailabilityDiagnosis {
	if s == nil {
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}

	accounts, err := s.listAccountsForNoAccountDiagnosis(ctx, groupID, platform)
	if err != nil {
		// Conservative fallback so the caller keeps returning 503; we do not
		// want a transient lookup failure to flip into 404 model_not_found.
		return ModelAvailabilityDiagnosis{HasAccountsInPool: true, HasModelSupport: true}
	}

	diag := ModelAvailabilityDiagnosis{}
	matchingModelAccounts := 0
	rateLimitedMatchingAccounts := 0
	now := time.Now()
	for i := range accounts {
		diag.HasAccountsInPool = true
		// Mirrors the per-candidate filter used during account selection
		// (openai_account_scheduler.isAccountRequestCompatible): empty
		// model_mapping accepts everything; otherwise the explicit / wildcard
		// mapping must match.
		if accounts[i].IsModelSupported(requestedModel) {
			diag.HasModelSupport = true
			matchingModelAccounts++
			if accounts[i].RateLimitResetAt != nil && now.Before(*accounts[i].RateLimitResetAt) {
				rateLimitedMatchingAccounts++
				if diag.RateLimitResetAt == nil || accounts[i].RateLimitResetAt.Before(*diag.RateLimitResetAt) {
					resetAt := *accounts[i].RateLimitResetAt
					diag.RateLimitResetAt = &resetAt
				}
			}
		}
	}
	if matchingModelAccounts > 0 && matchingModelAccounts == rateLimitedMatchingAccounts {
		diag.AllModelSupportingAccountsRateLimited = true
	}
	return diag
}

func (s *OpenAIGatewayService) listAccountsForNoAccountDiagnosis(ctx context.Context, groupID *int64, platform string) ([]Account, error) {
	if s == nil || s.accountRepo == nil {
		return nil, nil
	}
	platform = normalizeOpenAICompatiblePlatform(platform)
	accounts, err := s.accountRepo.ListByPlatform(ctx, platform)
	if err != nil {
		return nil, err
	}
	if s.cfg != nil && s.cfg.RunMode == config.RunModeSimple {
		return accounts, nil
	}

	out := make([]Account, 0, len(accounts))
	for i := range accounts {
		if groupID != nil {
			if openAIAccountHasGroupID(&accounts[i], *groupID) {
				out = append(out, accounts[i])
			}
			continue
		}
		if len(accounts[i].GroupIDs) == 0 {
			out = append(out, accounts[i])
		}
	}
	return out, nil
}

func openAIAccountHasGroupID(account *Account, groupID int64) bool {
	if account == nil {
		return false
	}
	for _, id := range account.GroupIDs {
		if id == groupID {
			return true
		}
	}
	return false
}

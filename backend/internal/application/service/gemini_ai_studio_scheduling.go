package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// SelectAccountForAIStudioEndpoints selects an account that is likely to
// succeed against generativelanguage.googleapis.com (for example, models GETs).
func (s *GeminiMessagesCompatService) SelectAccountForAIStudioEndpoints(ctx context.Context, groupID *int64) (*Account, error) {
	return s.SelectAccountForAIStudioEndpointsWithExclusions(ctx, groupID, nil)
}

// SelectAccountForAIStudioEndpointsWithExclusions keeps metadata retries away
// from accounts whose scheduling state changed after a queued slot was granted.
func (s *GeminiMessagesCompatService) SelectAccountForAIStudioEndpointsWithExclusions(ctx context.Context, groupID *int64, excludedAccountIDs map[int64]struct{}) (*Account, error) {
	candidates, routed := apiKeyGroupRoutingCandidates(ctx, groupID)
	if !routed {
		return s.selectAccountForAIStudioEndpointsWithExclusionsInGroup(ctx, groupID, excludedAccountIDs)
	}
	var lastErr error = ErrNoAvailableAccounts
	for i := range candidates {
		candidateID := candidates[i].GroupID
		account, err := s.selectAccountForAIStudioEndpointsWithExclusionsInGroup(
			withAPIKeyGroupRoutingAttempt(ctx, candidates[i]),
			&candidateID,
			excludedAccountIDs,
		)
		if err == nil && account != nil {
			markAPIKeyGroupRoutingSelected(ctx, candidateID)
			return account, nil
		}
		if err != nil {
			lastErr = err
			if !errors.Is(err, ErrNoAvailableAccounts) {
				return nil, err
			}
		}
	}
	return nil, lastErr
}

func (s *GeminiMessagesCompatService) selectAccountForAIStudioEndpointsWithExclusionsInGroup(ctx context.Context, groupID *int64, excludedAccountIDs map[int64]struct{}) (*Account, error) {
	accounts, err := s.listSchedulableAccountsOnce(ctx, groupID, PlatformGemini, true)
	if err != nil {
		return nil, fmt.Errorf("query accounts failed: %w", err)
	}

	var selected *Account
	for i := range accounts {
		account := &accounts[i]
		if _, excluded := excludedAccountIDs[account.ID]; excluded {
			continue
		}
		if selected == nil || preferAIStudioMetadataAccount(account, selected) {
			selected = account
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("%w: no available Gemini accounts", ErrNoAvailableAccounts)
	}
	return s.hydrateSelectedAccount(ctx, selected)
}

func preferAIStudioMetadataAccount(candidate, selected *Account) bool {
	candidateRank := aiStudioMetadataAccountRank(candidate)
	selectedRank := aiStudioMetadataAccountRank(selected)
	if candidateRank != selectedRank {
		return candidateRank < selectedRank
	}
	if candidate.Priority != selected.Priority {
		return candidate.Priority < selected.Priority
	}
	switch {
	case candidate.LastUsedAt == nil && selected.LastUsedAt != nil:
		return true
	case candidate.LastUsedAt != nil && selected.LastUsedAt == nil:
		return false
	case candidate.LastUsedAt == nil && selected.LastUsedAt == nil:
		return candidate.Type == AccountTypeOAuth && selected.Type != AccountTypeOAuth
	default:
		return candidate.LastUsedAt.Before(*selected.LastUsedAt)
	}
}

func aiStudioMetadataAccountRank(account *Account) int {
	if account == nil {
		return 999
	}
	switch account.Type {
	case AccountTypeAPIKey:
		if strings.TrimSpace(account.GetCredential("api_key")) != "" {
			return 0
		}
		return 9
	case AccountTypeOAuth:
		if strings.TrimSpace(account.GetCredential("project_id")) == "" {
			return 1
		}
		if strings.TrimSpace(account.GetCredential("oauth_type")) == "ai_studio" {
			return 2
		}
		return 3
	case AccountTypeServiceAccount:
		return 999
	default:
		return 10
	}
}

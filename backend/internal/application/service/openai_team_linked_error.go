package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

const (
	openAITeamLinkedErrorDedupTTL      = 60 * time.Second
	openAITeamLinkedErrorFanoutTimeout = 30 * time.Second
	openAITeamLinkedErrorBlockReason   = "team_linked_error"
)

type openAIWorkspaceRevocationRepository interface {
	RevokeOpenAIWorkspaceAccounts(context.Context, string, string) ([]int64, error)
}

// maybeHandleOpenAITeamLinkedError turns a ChatGPT Team workspace deactivation
// into a bounded sibling-account circuit break. It is intentionally restricted
// to OpenAI OAuth accounts and the exact upstream error code, so ordinary 402s
// and API-key accounts retain their existing handling.
func (s *RateLimitService) maybeHandleOpenAITeamLinkedError(ctx context.Context, account *Account, statusCode int, responseBody []byte) {
	if s == nil || s.accountRepo == nil || !isOpenAIOAuthAccount(account) ||
		!isOpenAIWorkspaceDeactivatedError(statusCode, responseBody) {
		return
	}
	teamID := strings.TrimSpace(account.GetChatGPTAccountID())
	if teamID == "" || !s.loadOAuth401CleanupPolicy(ctx).revokeSameWorkspace {
		return
	}
	repo, ok := s.accountRepo.(openAIWorkspaceRevocationRepository)
	if !ok {
		slog.Error("openai_workspace_revocation_repository_unavailable", "account_id", account.ID)
		return
	}
	if !s.markOpenAITeamLinkedFired(teamID) {
		return
	}

	base := context.Background()
	if ctx != nil {
		base = context.WithoutCancel(ctx)
	}
	opCtx, cancel := context.WithTimeout(base, openAITeamLinkedErrorFanoutTimeout)
	defer cancel()

	errorMessage := fmt.Sprintf("Workspace deactivated (402): team-linked error triggered by account #%d", account.ID)
	accountIDs, err := repo.RevokeOpenAIWorkspaceAccounts(opCtx, teamID, errorMessage)
	if err != nil {
		s.openaiTeamLinkedMu.Lock()
		delete(s.openaiTeamLinkedRecent, teamID)
		s.openaiTeamLinkedMu.Unlock()
		slog.Warn("openai_workspace_revocation_failed", "trigger_account_id", account.ID, "error", err)
		return
	}
	// Only rows whose workspace still matched at the atomic mutation are blocked.
	// The repository also persists outbox events for every owner and shadow.
	for _, accountID := range accountIDs {
		s.notifyAccountSchedulingBlocked(&Account{ID: accountID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}, time.Time{}, openAITeamLinkedErrorBlockReason)
	}
	slog.Warn("openai_team_linked_error_fanout", "trigger_account_id", account.ID, "chatgpt_account_id", teamID, "affected", len(accountIDs))
}

func (s *RateLimitService) markOpenAITeamLinkedFired(teamID string) bool {
	now := time.Now()
	s.openaiTeamLinkedMu.Lock()
	defer s.openaiTeamLinkedMu.Unlock()
	if expiry, ok := s.openaiTeamLinkedRecent[teamID]; ok && expiry.After(now) {
		return false
	}
	if s.openaiTeamLinkedRecent == nil {
		s.openaiTeamLinkedRecent = make(map[string]time.Time)
	}
	for key, expiry := range s.openaiTeamLinkedRecent {
		if !expiry.After(now) {
			delete(s.openaiTeamLinkedRecent, key)
		}
	}
	s.openaiTeamLinkedRecent[teamID] = now.Add(openAITeamLinkedErrorDedupTTL)
	return true
}

package service

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

const oauth401PostDeleteCleanupTimeout = 2 * time.Second

type cachedOAuth401CleanupPolicy struct {
	enabled   bool
	expiresAt int64
}

type oauth401CleanupAttemptContextKey struct{}

type oauth401CleanupAttemptState struct {
	attempted atomic.Bool
}

type oauth401AccountDeletionRepository interface {
	DeleteOAuthAccountIfCredentialsUnchanged(context.Context, int64, map[string]any) ([]int64, bool, error)
}

func withOAuth401CleanupAttemptState(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, oauth401CleanupAttemptContextKey{}, &oauth401CleanupAttemptState{})
}

func markOAuth401CleanupAttempted(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	if state, ok := ctx.Value(oauth401CleanupAttemptContextKey{}).(*oauth401CleanupAttemptState); ok && state != nil {
		return state.attempted.Swap(true)
	}
	return false
}

// tryAutoDeleteOAuthAccountOn401 handles a confirmed gateway 401 only when the
// operator enabled the destructive policy. A credential CAS miss means a
// concurrent reauthorization won; the stale request still fails over without
// mutating the newly authorized account.
func (s *RateLimitService) tryAutoDeleteOAuthAccountOn401(ctx context.Context, account *Account) bool {
	if s == nil || account == nil || account.Type != AccountTypeOAuth || account.IsCredentialShadow() {
		return false
	}
	if markOAuth401CleanupAttempted(ctx) || !s.oauth401CleanupEnabled(ctx) {
		return false
	}
	repo, ok := s.accountRepo.(oauth401AccountDeletionRepository)
	if !ok {
		slog.Error("oauth_401_auto_delete_repository_unavailable", "account_id", account.ID)
		return false
	}

	deleteCtx, cancel := openAIAccountStateContext(ctx)
	deletedIDs, deleted, err := repo.DeleteOAuthAccountIfCredentialsUnchanged(deleteCtx, account.ID, account.Credentials)
	cancel()
	if err != nil {
		slog.Error("oauth_401_auto_delete_failed", "account_id", account.ID, "platform", account.Platform, "error", err)
		return false
	}
	if !deleted {
		slog.Info("oauth_401_auto_delete_skipped_stale_credentials", "account_id", account.ID, "platform", account.Platform)
		return true
	}

	cleanupBase := context.Background()
	if ctx != nil {
		cleanupBase = context.WithoutCancel(ctx)
	}
	cleanupCtx, cleanupCancel := context.WithTimeout(cleanupBase, oauth401PostDeleteCleanupTimeout)
	defer cleanupCancel()
	if s.tokenCacheInvalidator != nil {
		if err := s.tokenCacheInvalidator.InvalidateToken(cleanupCtx, account); err != nil {
			slog.Warn("oauth_401_auto_delete_token_cache_cleanup_failed", "account_id", account.ID, "error", err)
		}
	}
	for _, accountID := range deletedIDs {
		if s.runtimeStateCleaner != nil {
			s.runtimeStateCleaner.DeleteAccountRuntimeState(accountID)
		} else {
			s.DeleteAccountRuntimeState(accountID)
		}
	}
	account.Status = StatusError
	account.Schedulable = false
	if s.runtimeBlocker != nil {
		s.runtimeBlocker.BlockAccountScheduling(account, time.Time{}, "oauth_401_deleted")
	}
	slog.Warn("oauth_401_account_auto_deleted", "account_id", account.ID, "platform", account.Platform, "deleted_account_count", len(deletedIDs))
	return true
}

func (s *RateLimitService) oauth401CleanupEnabled(ctx context.Context) bool {
	if s == nil || s.settingService == nil {
		return false
	}
	now := time.Now().UnixNano()
	if cached, ok := s.oauth401CleanupPolicyCache.Load().(*cachedOAuth401CleanupPolicy); ok &&
		cached != nil && now < cached.expiresAt {
		return cached.enabled
	}
	value, _, _ := s.oauth401CleanupPolicySF.Do("oauth_401_cleanup", func() (any, error) {
		checkNow := time.Now().UnixNano()
		if cached, ok := s.oauth401CleanupPolicyCache.Load().(*cachedOAuth401CleanupPolicy); ok &&
			cached != nil && checkNow < cached.expiresAt {
			return cached.enabled, nil
		}
		enabled := false
		if settings, err := s.settingService.GetOAuth401CleanupSettings(ctx); err == nil && settings != nil {
			enabled = settings.Enabled
		} else if err != nil {
			slog.Warn("oauth_401_cleanup_settings_load_failed", "error", err)
		}
		s.oauth401CleanupPolicyCache.Store(&cachedOAuth401CleanupPolicy{
			enabled:   enabled,
			expiresAt: time.Now().Add(openAIFailurePolicyCacheTTL).UnixNano(),
		})
		return enabled, nil
	})
	enabled, _ := value.(bool)
	return enabled
}

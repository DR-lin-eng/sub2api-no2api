package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

const openAIOAuthQuotaAutoEnableBatchSize = 500

type openAIOAuthQuotaAutoEnableRepository interface {
	AutoEnableOpenAIOAuthAccountIfMarked(ctx context.Context, accountID int64) (bool, error)
	AutoEnableOpenAIOAuthAccountsAfterQuotaReset(ctx context.Context, now time.Time, limit int) ([]int64, error)
}

// MaybeAutoEnableOpenAIAccountAfterQuotaQuery restores only accounts that the
// OAuth failure circuit marked and only when a live main-Codex result contains
// a known, non-exhausted window.
func (s *RateLimitService) MaybeAutoEnableOpenAIAccountAfterQuotaQuery(
	ctx context.Context,
	accountID int64,
	usage *OpenAIQuotaUsage,
) (*Account, bool, error) {
	if s == nil || s.accountRepo == nil || accountID <= 0 || !isOpenAIQuotaUsageKnownAvailable(usage) {
		return nil, false, nil
	}
	settings := s.openAIFailurePolicySettings(ctx)
	if !settings.AutoEnableWhenQuotaAvailableEnabled {
		return nil, false, nil
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, false, err
	}
	if !isOpenAIOAuthFailureCircuitAutoDisabled(account) {
		return account, false, nil
	}

	writer, ok := s.accountRepo.(openAIOAuthQuotaAutoEnableRepository)
	if !ok {
		return nil, false, fmt.Errorf("OpenAI OAuth quota auto-enable repository is unavailable")
	}
	enabled, err := writer.AutoEnableOpenAIOAuthAccountIfMarked(ctx, accountID)
	if err != nil || !enabled {
		return account, false, err
	}
	s.resetOpenAIFailureCounter(ctx, accountID)
	s.notifyAccountSchedulingBlockCleared(accountID)
	updated, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil {
		return nil, true, err
	}
	slog.Info("openai_oauth_account_auto_enabled_after_quota_query", "account_id", accountID)
	return updated, true, nil
}

// AutoEnableOpenAIAccountsAfterQuotaReset restores a bounded batch after the
// upstream-confirmed quota countdown ends. The repository compare-and-set
// keeps a concurrent administrator pause authoritative.
func (s *RateLimitService) AutoEnableOpenAIAccountsAfterQuotaReset(ctx context.Context, now time.Time) (int, error) {
	if s == nil || s.accountRepo == nil {
		return 0, nil
	}
	settings := s.openAIFailurePolicySettings(ctx)
	if !settings.AutoEnableAfterQuotaResetEnabled {
		return 0, nil
	}
	writer, ok := s.accountRepo.(openAIOAuthQuotaAutoEnableRepository)
	if !ok {
		return 0, fmt.Errorf("OpenAI OAuth quota auto-enable repository is unavailable")
	}
	accountIDs, err := writer.AutoEnableOpenAIOAuthAccountsAfterQuotaReset(ctx, now, openAIOAuthQuotaAutoEnableBatchSize)
	if err != nil {
		return 0, err
	}
	for _, accountID := range accountIDs {
		s.resetOpenAIFailureCounter(ctx, accountID)
		s.notifyAccountSchedulingBlockCleared(accountID)
	}
	if len(accountIDs) > 0 {
		slog.Info("openai_oauth_accounts_auto_enabled_after_quota_reset", "count", len(accountIDs))
	}
	return len(accountIDs), nil
}

func isOpenAIOAuthFailureCircuitAutoDisabled(account *Account) bool {
	if account == nil || account.Platform != PlatformOpenAI || account.Type != AccountTypeOAuth ||
		account.Status != StatusActive || account.Schedulable || account.Extra == nil {
		return false
	}
	if account.AutoPauseOnExpired && account.ExpiresAt != nil && !time.Now().Before(*account.ExpiresAt) {
		return false
	}
	source, _ := account.Extra[AccountAutoEnableSourceExtraKey].(string)
	return source == AccountAutoEnableSourceOpenAIOAuthFailure
}

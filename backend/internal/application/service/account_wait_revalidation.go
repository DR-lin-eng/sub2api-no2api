package service

import (
	"context"
	"errors"
	"fmt"
)

// ErrAccountSchedulingChanged means a request waited for an account slot, but
// the account left the schedulable pool before that slot became available.
// Callers must release the acquired slot and run account selection again.
var ErrAccountSchedulingChanged = errors.New("account scheduling changed while waiting")

// EnsureAccountSchedulableAfterWait rechecks a queued selection after its
// concurrency slot becomes available. Existing upstream requests are allowed
// to finish, while queued requests leave accounts that were disabled meanwhile.
func (s *GatewayService) EnsureAccountSchedulableAfterWait(ctx context.Context, groupID *int64, sessionHash string, accountID int64) error {
	schedulable, err := accountSchedulableForWaitRevalidation(ctx, s.accountRepo, s.schedulerSnapshot, accountID)
	if err != nil {
		return fmt.Errorf("recheck account %d after wait: %w", accountID, err)
	}
	if schedulable {
		return nil
	}
	if sessionHash != "" && s.cache != nil {
		_ = s.cache.DeleteSessionAccountID(ctx, derefGroupID(groupID), sessionHash)
	}
	return fmt.Errorf("%w: account %d", ErrAccountSchedulingChanged, accountID)
}

// EnsureAccountSchedulableAfterWait prevents a stale WaitPlan from forwarding
// to an OpenAI account that was disabled while the request was queued.
func (s *OpenAIGatewayService) EnsureAccountSchedulableAfterWait(ctx context.Context, groupID *int64, sessionHash string, accountID int64) error {
	schedulable, err := accountSchedulableForWaitRevalidation(ctx, s.accountRepo, s.schedulerSnapshot, accountID)
	if err != nil {
		return fmt.Errorf("recheck openai account %d after wait: %w", accountID, err)
	}
	if schedulable {
		return nil
	}
	_ = s.deleteStickySessionAccountID(ctx, groupID, sessionHash)
	return fmt.Errorf("%w: account %d", ErrAccountSchedulingChanged, accountID)
}

func accountSchedulableForWaitRevalidation(ctx context.Context, repo AccountRepository, snapshot *SchedulerSnapshotService, accountID int64) (bool, error) {
	if ImageIndependentSchedulingFromContext(ctx) {
		if repo == nil {
			return false, errors.New("account state unavailable")
		}
		account, err := repo.GetByID(ctx, accountID)
		if errors.Is(err, ErrAccountNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return account.IsSchedulableWithContext(ctx), nil
	}
	var cacheErr error
	if snapshot != nil {
		state, found, err := snapshot.cachedAccountSchedulingState(ctx, accountID)
		if err == nil && found {
			return state.Exists && state.Schedulable, nil
		}
		cacheErr = err
	}

	if repo != nil {
		if reader, ok := repo.(AccountSchedulingStateReader); ok {
			state, err := reader.GetAccountSchedulingState(ctx, accountID)
			if errors.Is(err, ErrAccountNotFound) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			return state.Exists && state.Schedulable, nil
		}

		account, err := repo.GetByID(ctx, accountID)
		if errors.Is(err, ErrAccountNotFound) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		return account != nil && account.IsSchedulable(), nil
	}
	if cacheErr != nil {
		return false, cacheErr
	}
	return false, errors.New("account state unavailable")
}

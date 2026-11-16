package service

import "context"

// Snapshot membership follows persistent configuration. Cooldowns expire on
// reads and never require a database write or periodic rebuild to rejoin.
func (s *SchedulerSnapshotService) loadAccountsFromDB(ctx context.Context, bucket SchedulerBucket, useMixed bool) ([]Account, error) {
	if ImageIndependentSchedulingFromContext(ctx) {
		return s.loadImageSchedulingAccountsFromDB(ctx, bucket, useMixed)
	}
	if s.accountRepo == nil {
		return nil, ErrSchedulerCacheNotReady
	}
	var groupID *int64
	if !s.isRunModeSimple() && bucket.GroupID > 0 {
		groupID = &bucket.GroupID
	}
	platforms := []string{bucket.Platform}
	if useMixed {
		platforms = append(platforms, PlatformAntigravity)
	}
	accounts, err := s.accountRepo.ListModelAvailabilityCandidates(ctx, groupID, platforms, s.isRunModeSimple())
	if err != nil || !useMixed {
		return accounts, err
	}
	filtered := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		if account.Platform == PlatformAntigravity && !account.IsMixedSchedulingEnabled() {
			continue
		}
		filtered = append(filtered, account)
	}
	return filtered, nil
}

func schedulableAccountValues(accounts []Account) []Account {
	result := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		if account.IsSchedulable() {
			result = append(result, account)
		}
	}
	return result
}

func derefSchedulableAccounts(accounts []*Account) []Account {
	result := make([]Account, 0, len(accounts))
	for _, account := range accounts {
		if account != nil && account.IsSchedulable() {
			result = append(result, *account)
		}
	}
	return result
}

func (s *SchedulerSnapshotService) loadImageSchedulingAccountsFromDB(ctx context.Context, bucket SchedulerBucket, useMixed bool) ([]Account, error) {
	if s.accountRepo == nil {
		return nil, ErrSchedulerCacheNotReady
	}
	groupID := bucket.GroupID
	if s.isRunModeSimple() {
		groupID = 0
	}

	if useMixed {
		platforms := []string{bucket.Platform, PlatformAntigravity}
		var accounts []Account
		var err error
		if groupID > 0 {
			accounts, err = s.accountRepo.ListSchedulableByGroupIDAndPlatforms(ctx, groupID, platforms)
		} else if s.isRunModeSimple() {
			accounts, err = s.accountRepo.ListSchedulableByPlatforms(ctx, platforms)
		} else {
			accounts, err = s.accountRepo.ListSchedulableUngroupedByPlatforms(ctx, platforms)
		}
		if err != nil {
			return nil, err
		}
		filtered := make([]Account, 0, len(accounts))
		for _, acc := range accounts {
			if acc.Platform == PlatformAntigravity && !acc.IsMixedSchedulingEnabled() {
				continue
			}
			filtered = append(filtered, acc)
		}
		return filtered, nil
	}

	if groupID > 0 {
		return s.accountRepo.ListSchedulableByGroupIDAndPlatform(ctx, groupID, bucket.Platform)
	}
	if s.isRunModeSimple() {
		return s.accountRepo.ListSchedulableByPlatform(ctx, bucket.Platform)
	}
	return s.accountRepo.ListSchedulableUngroupedByPlatform(ctx, bucket.Platform)
}

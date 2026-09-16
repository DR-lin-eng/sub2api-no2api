package service

import (
	"context"
	"sort"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"golang.org/x/sync/errgroup"
)

const accountBatchDeleteMaxConcurrency = 5

type AccountBatchDeleteError struct {
	AccountID int64  `json:"account_id"`
	Error     string `json:"error"`
}

type AccountBatchDeleteResult struct {
	Total      int                       `json:"total"`
	Success    int                       `json:"success"`
	Failed     int                       `json:"failed"`
	SuccessIDs []int64                   `json:"success_ids"`
	FailedIDs  []int64                   `json:"failed_ids"`
	Errors     []AccountBatchDeleteError `json:"errors"`
}

func (s *adminServiceImpl) BatchDeleteAccounts(ctx context.Context, ids []int64) (*AccountBatchDeleteResult, error) {
	accountIDs := normalizeAccountBatchDeleteIDs(ids)
	result := &AccountBatchDeleteResult{Total: len(accountIDs)}
	if len(accountIDs) == 0 {
		return result, nil
	}

	accounts, err := s.GetAccountsByIDs(ctx, accountIDs)
	if err != nil {
		return nil, err
	}
	requested := make(map[int64]struct{}, len(accountIDs))
	accountsByID := make(map[int64]*Account, len(accounts))
	for _, id := range accountIDs {
		requested[id] = struct{}{}
	}
	for _, account := range accounts {
		if account != nil {
			accountsByID[account.ID] = account
		}
	}

	rootIDs := make([]int64, 0, len(accountIDs))
	dependents := make(map[int64][]int64)
	for _, accountID := range accountIDs {
		account := accountsByID[accountID]
		if account == nil {
			result.FailedIDs = append(result.FailedIDs, accountID)
			result.Errors = append(result.Errors, AccountBatchDeleteError{AccountID: accountID, Error: "account not found"})
			continue
		}

		rootID := accountID
		visited := map[int64]struct{}{accountID: {}}
		for {
			current := accountsByID[rootID]
			if current == nil || current.ParentAccountID == nil {
				break
			}
			parentID := *current.ParentAccountID
			if _, selected := requested[parentID]; !selected || accountsByID[parentID] == nil {
				break
			}
			if _, cyclic := visited[parentID]; cyclic {
				rootID = accountID
				break
			}
			visited[parentID] = struct{}{}
			rootID = parentID
		}
		if rootID == accountID {
			rootIDs = append(rootIDs, accountID)
		} else {
			dependents[rootID] = append(dependents[rootID], accountID)
		}
	}

	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(accountBatchDeleteMaxConcurrency)
	var mu sync.Mutex
	for _, id := range rootIDs {
		accountID := id
		group.Go(func() error {
			deleteErr := s.deleteAccount(groupCtx, accountID, true)
			affected := append([]int64{accountID}, dependents[accountID]...)
			mu.Lock()
			defer mu.Unlock()
			if deleteErr != nil {
				for _, affectedID := range affected {
					result.FailedIDs = append(result.FailedIDs, affectedID)
					result.Errors = append(result.Errors, AccountBatchDeleteError{AccountID: affectedID, Error: deleteErr.Error()})
				}
				return nil
			}
			result.SuccessIDs = append(result.SuccessIDs, affected...)
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	if len(result.SuccessIDs) > 0 {
		if _, err := s.rebalanceProxyAssignmentsIfEnabled(ctx); err != nil {
			logger.LegacyPrintf("service.admin_account", "rebalance after account batch delete failed: err=%v", err)
		}
	}

	sort.Slice(result.SuccessIDs, func(i, j int) bool { return result.SuccessIDs[i] < result.SuccessIDs[j] })
	sort.Slice(result.FailedIDs, func(i, j int) bool { return result.FailedIDs[i] < result.FailedIDs[j] })
	sort.Slice(result.Errors, func(i, j int) bool { return result.Errors[i].AccountID < result.Errors[j].AccountID })
	result.Success = len(result.SuccessIDs)
	result.Failed = len(result.FailedIDs)
	return result, nil
}

func normalizeAccountBatchDeleteIDs(ids []int64) []int64 {
	seen := make(map[int64]struct{}, len(ids))
	normalized := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i] < normalized[j] })
	return normalized
}

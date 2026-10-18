package service

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	MaxUpstreamModelSyncBatchSize = 100
	upstreamModelSyncConcurrency  = 4
	upstreamModelSyncTimeout      = 20 * time.Second
)

// UpstreamModelMappingWriter merges only new model IDs into the current
// mapping, atomically preserving concurrent mapping edits and credentials.
type UpstreamModelMappingWriter interface {
	MergeUpstreamModelMapping(context.Context, *Account, []string) (int, error)
}

var ErrUpstreamModelSyncAccountChanged = errors.New("account changed during upstream model sync")

type UpstreamModelSyncResult struct {
	AccountID   int64  `json:"account_id"`
	AccountName string `json:"account_name,omitempty"`
	Status      string `json:"status"`
	ModelCount  int    `json:"model_count"`
	AddedCount  int    `json:"added_count"`
	ErrorKind   string `json:"error_kind,omitempty"`
	Error       string `json:"error,omitempty"`
}

type BatchUpstreamModelSyncResult struct {
	Total   int                       `json:"total"`
	Success int                       `json:"success"`
	Failed  int                       `json:"failed"`
	Skipped int                       `json:"skipped"`
	Results []UpstreamModelSyncResult `json:"results"`
}

// BatchSyncUpstreamModels uses the existing platform-specific live probes.
// A failed/empty probe never writes; each successful account commits separately.
func (s *AccountTestService) BatchSyncUpstreamModels(ctx context.Context, accountIDs []int64) (*BatchUpstreamModelSyncResult, error) {
	if len(accountIDs) == 0 || len(accountIDs) > MaxUpstreamModelSyncBatchSize {
		return nil, newUpstreamModelSyncConfigError("Provide between 1 and 100 account IDs", nil)
	}
	ids := make([]int64, 0, len(accountIDs))
	seen := make(map[int64]bool, len(accountIDs))
	for _, id := range accountIDs {
		if id <= 0 {
			return nil, newUpstreamModelSyncConfigError("Account IDs must be positive", nil)
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	if s == nil || s.accountRepo == nil {
		return nil, newUpstreamModelSyncConfigError("Account repository is not configured", nil)
	}
	writer, ok := s.accountRepo.(UpstreamModelMappingWriter)
	if !ok {
		return nil, newUpstreamModelSyncConfigError("Upstream model persistence is not configured", nil)
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	result := &BatchUpstreamModelSyncResult{Total: len(ids), Results: make([]UpstreamModelSyncResult, len(ids))}
	jobs := make(chan int, len(ids))
	for i := range ids {
		jobs <- i
	}
	close(jobs)
	var workers sync.WaitGroup
	for worker := 0; worker < min(upstreamModelSyncConcurrency, len(ids)); worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				result.Results[i] = s.syncUpstreamModelsForAccount(ctx, writer, ids[i])
			}
		}()
	}
	workers.Wait()
	for _, item := range result.Results {
		switch item.Status {
		case "success":
			result.Success++
		case "skipped":
			result.Skipped++
		default:
			result.Failed++
		}
	}
	return result, nil
}

func (s *AccountTestService) syncUpstreamModelsForAccount(ctx context.Context, writer UpstreamModelMappingWriter, id int64) UpstreamModelSyncResult {
	result := UpstreamModelSyncResult{AccountID: id, Status: "failed"}
	ctx, cancel := context.WithTimeout(ctx, upstreamModelSyncTimeout)
	defer cancel()
	if ctx.Err() != nil {
		result.ErrorKind, result.Error = "canceled", "Model sync was canceled or timed out"
		return result
	}
	account, err := s.accountRepo.GetByID(ctx, id)
	if err != nil || account == nil {
		result.ErrorKind, result.Error = "account", "Account could not be loaded"
		return result
	}
	result.AccountName = account.Name
	if account.IsCredentialShadow() {
		result.Status, result.ErrorKind, result.Error = "skipped", "unsupported", "Sync the parent account for a credential shadow"
		return result
	}
	models, err := s.FetchUpstreamSupportedModels(ctx, account)
	if err != nil {
		result.ErrorKind, result.Error = "upstream", "Failed to request upstream model list"
		var syncErr *UpstreamModelSyncError
		if errors.As(err, &syncErr) {
			result.ErrorKind, result.Error = string(syncErr.Kind), syncErr.SafeMessage()
			if syncErr.Kind == UpstreamModelSyncErrorUnsupported {
				result.Status = "skipped"
			}
		}
		return result
	}
	models = dedupeAndSortModelIDs(models)
	if len(models) == 0 {
		result.ErrorKind, result.Error = "upstream", "Upstream returned no supported models"
		return result
	}
	if ctx.Err() != nil {
		result.ErrorKind, result.Error = "canceled", "Model sync was canceled or timed out"
		return result
	}
	added, err := writer.MergeUpstreamModelMapping(ctx, account, models)
	if err != nil {
		result.ErrorKind, result.Error = "persistence", "Failed to save upstream models"
		if errors.Is(err, ErrUpstreamModelSyncAccountChanged) {
			result.ErrorKind, result.Error = "account_changed", "Account changed during sync; retry with its current configuration"
		}
		return result
	}
	result.Status, result.ModelCount, result.AddedCount = "success", len(models), added
	if s.upstreamModelsCache != nil {
		s.upstreamModelsCache.InvalidateAvailableModelsCache(nil, account.Platform)
		for _, groupID := range account.GroupIDs {
			s.upstreamModelsCache.InvalidateAvailableModelsCache(&groupID, account.Platform)
		}
	}
	return result
}

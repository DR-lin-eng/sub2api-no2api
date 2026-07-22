package service

import (
	"context"

	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
)

type UpstreamBillingRateSnapshotItem struct {
	AccountID int64                         `json:"account_id"`
	Snapshot  *UpstreamBillingProbeSnapshot `json:"snapshot,omitempty"`
}

type UpstreamBillingRateProjection struct {
	AccountID int64
	Extra     map[string]any
}

type UpstreamBillingRateListFilters struct {
	Platform, AccountType, Status, Search, PrivacyMode string
	GroupID                                            int64
}

type upstreamBillingRateProjectionLister interface {
	ListUpstreamBillingRateProjections(
		context.Context,
		pagination.PaginationParams,
		UpstreamBillingRateListFilters,
	) ([]UpstreamBillingRateProjection, int64, error)
}

func (s *adminServiceImpl) ListUpstreamBillingRateSnapshots(
	ctx context.Context,
	page, pageSize int,
	filters UpstreamBillingRateListFilters,
	sortBy, sortOrder string,
) ([]UpstreamBillingRateSnapshotItem, int64, error) {
	lister, ok := s.accountRepo.(upstreamBillingRateProjectionLister)
	if !ok {
		return nil, 0, ErrUpstreamBillingProbeUnavailable
	}
	projections, total, err := lister.ListUpstreamBillingRateProjections(ctx, pagination.PaginationParams{
		Page: page, PageSize: pageSize, SortBy: sortBy, SortOrder: sortOrder,
	}, filters)
	if err != nil {
		return nil, 0, err
	}
	items := make([]UpstreamBillingRateSnapshotItem, 0, len(projections))
	for _, projection := range projections {
		item := UpstreamBillingRateSnapshotItem{AccountID: projection.AccountID}
		account := &Account{ID: projection.AccountID, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: projection.Extra}
		if snapshot := decodeUpstreamBillingProbeSnapshot(account.Extra); snapshot != nil && validUpstreamBillingSnapshot(snapshot) {
			item.Snapshot = snapshot
		}
		items = append(items, item)
	}
	return items, total, nil
}

func validUpstreamBillingSnapshot(snapshot *UpstreamBillingProbeSnapshot) bool {
	if snapshot == nil {
		return false
	}
	switch snapshot.Status {
	case UpstreamBillingProbeStatusOK, UpstreamBillingProbeStatusUnsupported, UpstreamBillingProbeStatusFailed:
		return !snapshot.LastAttemptAt.IsZero() && !snapshot.NextProbeAt.IsZero()
	default:
		return false
	}
}

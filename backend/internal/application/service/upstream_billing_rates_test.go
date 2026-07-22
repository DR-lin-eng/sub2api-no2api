package service

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
	"github.com/stretchr/testify/require"
)

type upstreamBillingRateProjectionRepo struct {
	AccountRepository
	items   []UpstreamBillingRateProjection
	total   int64
	params  pagination.PaginationParams
	filters UpstreamBillingRateListFilters
}

func (r *upstreamBillingRateProjectionRepo) ListUpstreamBillingRateProjections(
	_ context.Context,
	params pagination.PaginationParams,
	filters UpstreamBillingRateListFilters,
) ([]UpstreamBillingRateProjection, int64, error) {
	r.params = params
	r.filters = filters
	return r.items, r.total, nil
}

func TestListUpstreamBillingRateSnapshotsUsesCompactProjection(t *testing.T) {
	now := time.Date(2026, 7, 18, 1, 0, 0, 0, time.UTC)
	repo := &upstreamBillingRateProjectionRepo{
		total: 3,
		items: []UpstreamBillingRateProjection{
			{
				AccountID: 12,
				Extra: map[string]any{UpstreamBillingProbeExtraKey: map[string]any{
					"status":          UpstreamBillingProbeStatusOK,
					"last_attempt_at": now.Format(time.RFC3339Nano),
					"next_probe_at":   now.Add(time.Hour).Format(time.RFC3339Nano),
					"data": map[string]any{
						"resolved_rate_multiplier": 2.5,
					},
				}},
			},
			{AccountID: 7},
			{AccountID: 3, Extra: map[string]any{UpstreamBillingProbeExtraKey: map[string]any{"status": "legacy-invalid"}}},
		},
	}
	service := &adminServiceImpl{accountRepo: repo}
	filters := UpstreamBillingRateListFilters{Platform: PlatformOpenAI, AccountType: AccountTypeAPIKey, Search: "needle"}

	items, total, err := service.ListUpstreamBillingRateSnapshots(context.Background(), 2, 25, filters, "upstream_billing_rate", "desc")

	require.NoError(t, err)
	require.Equal(t, int64(3), total)
	require.Equal(t, pagination.PaginationParams{Page: 2, PageSize: 25, SortBy: "upstream_billing_rate", SortOrder: "desc"}, repo.params)
	require.Equal(t, filters, repo.filters)
	require.Equal(t, []int64{12, 7, 3}, []int64{items[0].AccountID, items[1].AccountID, items[2].AccountID})
	require.NotNil(t, items[0].Snapshot)
	require.Equal(t, 2.5, items[0].Snapshot.Data["resolved_rate_multiplier"])
	require.Nil(t, items[1].Snapshot)
	require.Nil(t, items[2].Snapshot)
}

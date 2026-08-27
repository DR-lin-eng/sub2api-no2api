package activitycenter

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
	"github.com/stretchr/testify/require"
)

type fakeCampaignRepo struct {
	items  map[int64]*Campaign
	nextID int64
}

func newFakeCampaignRepo() *fakeCampaignRepo {
	return &fakeCampaignRepo{items: map[int64]*Campaign{}, nextID: 1}
}

func (r *fakeCampaignRepo) Create(_ context.Context, campaign *Campaign) error {
	copied := *campaign
	copied.ID = r.nextID
	r.nextID++
	now := time.Unix(1700000000, 0)
	copied.CreatedAt = now
	copied.UpdatedAt = now
	r.items[copied.ID] = &copied
	*campaign = copied
	return nil
}

func (r *fakeCampaignRepo) GetByID(_ context.Context, id int64) (*Campaign, error) {
	item, ok := r.items[id]
	if !ok {
		return nil, ErrCampaignNotFound
	}
	copied := *item
	return &copied, nil
}

func (r *fakeCampaignRepo) GetVisibleByID(_ context.Context, id int64, now time.Time) (*Campaign, error) {
	item, ok := r.items[id]
	if !ok || !item.IsVisibleAt(now) {
		return nil, ErrCampaignNotFound
	}
	copied := *item
	return &copied, nil
}

func (r *fakeCampaignRepo) Update(_ context.Context, campaign *Campaign) error {
	if _, ok := r.items[campaign.ID]; !ok {
		return ErrCampaignNotFound
	}
	copied := *campaign
	copied.UpdatedAt = time.Unix(1700000001, 0)
	r.items[copied.ID] = &copied
	*campaign = copied
	return nil
}

func (r *fakeCampaignRepo) Delete(_ context.Context, id int64) error {
	if _, ok := r.items[id]; !ok {
		return ErrCampaignNotFound
	}
	delete(r.items, id)
	return nil
}

func (r *fakeCampaignRepo) List(_ context.Context, params pagination.PaginationParams, _ ListFilters) ([]Campaign, *pagination.PaginationResult, error) {
	out := make([]Campaign, 0, len(r.items))
	for _, item := range r.items {
		out = append(out, *item)
	}
	return out, &pagination.PaginationResult{Total: int64(len(out)), Page: params.Page, PageSize: params.PageSize, Pages: 1}, nil
}

func (r *fakeCampaignRepo) ListVisible(_ context.Context, now time.Time) ([]Campaign, error) {
	out := make([]Campaign, 0, len(r.items))
	for _, item := range r.items {
		if item.IsVisibleAt(now) {
			out = append(out, *item)
		}
	}
	return out, nil
}

func TestCampaignServiceCreateNormalizesDefaults(t *testing.T) {
	svc := NewService(newFakeCampaignRepo())

	created, err := svc.Create(context.Background(), &CreateInput{
		Title:    "  Launch week  ",
		Subtitle: "  New benefits  ",
		Content:  "  Details  ",
	})

	require.NoError(t, err)
	require.Equal(t, int64(1), created.ID)
	require.Equal(t, "Launch week", created.Title)
	require.Equal(t, "New benefits", created.Subtitle)
	require.Equal(t, "custom", created.Type)
	require.Equal(t, "draft", created.Status)
	require.Equal(t, "Details", created.Content)
}

func TestCampaignServiceRejectsInvalidSchedule(t *testing.T) {
	svc := NewService(newFakeCampaignRepo())
	start := time.Unix(200, 0)
	end := time.Unix(100, 0)

	_, err := svc.Create(context.Background(), &CreateInput{
		Title:    "Bad range",
		Type:     CampaignTypeLottery,
		Status:   CampaignStatusActive,
		StartsAt: &start,
		EndsAt:   &end,
	})

	require.ErrorIs(t, err, ErrCampaignScheduleInvalid)
}

func TestCampaignServiceUpdateValidatesEnums(t *testing.T) {
	repo := newFakeCampaignRepo()
	svc := NewService(repo)
	created, err := svc.Create(context.Background(), &CreateInput{Title: "A"})
	require.NoError(t, err)

	badType := "banner"
	_, err = svc.Update(context.Background(), created.ID, &UpdateInput{Type: &badType})

	require.ErrorIs(t, err, ErrCampaignTypeInvalid)
}

func TestCampaignVisibleWindow(t *testing.T) {
	now := time.Unix(1000, 0)
	start := now.Add(-time.Minute)
	end := now.Add(time.Minute)
	campaign := &Campaign{
		Title:    "Visible",
		Status:   CampaignStatusActive,
		StartsAt: &start,
		EndsAt:   &end,
	}

	require.True(t, campaign.IsVisibleAt(now))
	require.False(t, campaign.IsVisibleAt(end))
}

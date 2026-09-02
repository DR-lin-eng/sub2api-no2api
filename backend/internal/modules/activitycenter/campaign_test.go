package activitycenter

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
	"github.com/stretchr/testify/require"
)

type fakeCampaignRepo struct {
	items          map[int64]*Campaign
	records        []Record
	checkinRecords []CheckinRecord
	nextID         int64
	eligible       bool
	eligibleSet    bool
	groupIDs       []int64
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

func (r *fakeCampaignRepo) CreateRecord(_ context.Context, record *Record) error {
	copied := *record
	copied.ID = r.nextID
	r.nextID++
	copied.CreatedAt = time.Now()
	r.records = append(r.records, copied)
	*record = copied
	return nil
}

func (r *fakeCampaignRepo) WithParticipationTx(ctx context.Context, _ int64, fn func(context.Context) error) error {
	return fn(ctx)
}

type fakeRewardGranter struct {
	grants []RewardGrant
}

func (g *fakeRewardGranter) Grant(_ context.Context, _ int64, grant RewardGrant) error {
	g.grants = append(g.grants, grant)
	return nil
}

func (g *fakeRewardGranter) AfterCommit(_ context.Context, _ int64, _ RewardGrant) {}

func (r *fakeCampaignRepo) CountUserPoolRecordsSince(_ context.Context, userID, campaignID int64, poolID string, since time.Time) (int64, error) {
	var count int64
	for _, record := range r.records {
		if record.UserID == userID && record.CampaignID == campaignID && record.PoolID == poolID && !record.CreatedAt.Before(since) {
			count++
		}
	}
	return count, nil
}

func (r *fakeCampaignRepo) CountPrizeRecords(_ context.Context, campaignID int64, prizeID string) (int64, error) {
	var count int64
	for _, record := range r.records {
		if record.CampaignID == campaignID && record.PrizeID == prizeID {
			count++
		}
	}
	return count, nil
}

func (r *fakeCampaignRepo) ListPrizeIssuedCodes(_ context.Context, campaignID int64, prizeID string) ([]string, error) {
	codes := make([]string, 0)
	for _, record := range r.records {
		if record.CampaignID != campaignID || record.PrizeID != prizeID {
			continue
		}
		var payload struct {
			Code string `json:"code"`
		}
		if json.Unmarshal([]byte(record.RewardPayloadJSON), &payload) == nil && payload.Code != "" {
			codes = append(codes, payload.Code)
		}
	}
	return codes, nil
}

func (r *fakeCampaignRepo) UserHasAllowedGroup(_ context.Context, userID int64, groupIDs []int64) (bool, error) {
	if r.eligibleSet {
		return r.eligible, nil
	}
	return true, nil
}

func (r *fakeCampaignRepo) ListUserAllowedGroupIDs(_ context.Context, _ int64) ([]int64, error) {
	return append([]int64(nil), r.groupIDs...), nil
}

func (r *fakeCampaignRepo) ListRecords(_ context.Context, params pagination.PaginationParams, filters RecordFilters) ([]Record, *pagination.PaginationResult, error) {
	out := make([]Record, 0, len(r.records))
	for _, record := range r.records {
		if filters.UserID > 0 && record.UserID != filters.UserID {
			continue
		}
		if filters.CampaignID > 0 && record.CampaignID != filters.CampaignID {
			continue
		}
		if filters.Type != "" && record.CampaignType != filters.Type {
			continue
		}
		out = append(out, record)
	}
	return out, &pagination.PaginationResult{Total: int64(len(out)), Page: params.Page, PageSize: params.PageSize, Pages: 1}, nil
}

func (r *fakeCampaignRepo) GetCheckinStatus(_ context.Context, campaignID, userID int64, checkinDate time.Time, _ int) (*CheckinStatus, error) {
	status := &CheckinStatus{}
	for i := range r.checkinRecords {
		item := r.checkinRecords[i]
		if item.CampaignID == campaignID && item.UserID == userID {
			status.Records = append(status.Records, item)
		}
	}
	if len(status.Records) > 0 {
		item := status.Records[len(status.Records)-1]
		status.LastCheckinDate = &item.CheckinDate
		status.CheckedToday = item.CheckinDate.Format("2006-01-02") == checkinDate.Format("2006-01-02")
		status.StreakDays, status.CycleDay = item.StreakDays, item.CycleDay
	}
	return status, nil
}

func (r *fakeCampaignRepo) CreateCheckinRecord(_ context.Context, record *CheckinRecord) error {
	for _, item := range r.checkinRecords {
		if item.CampaignID == record.CampaignID && item.UserID == record.UserID && item.CheckinDate.Format("2006-01-02") == record.CheckinDate.Format("2006-01-02") {
			return ErrCampaignAlreadyCheckedIn
		}
	}
	record.ID = r.nextID
	r.nextID++
	record.CreatedAt = time.Now()
	r.checkinRecords = append(r.checkinRecords, *record)
	return nil
}

func (r *fakeCampaignRepo) ListCheckinLeaderboard(_ context.Context, _ int64, _ int) ([]CheckinLeaderboardEntry, error) {
	return nil, nil
}

func TestCampaignServiceParticipateLotteryRecordsPrize(t *testing.T) {
	repo := newFakeCampaignRepo()
	svc := NewService(repo)
	granter := &fakeRewardGranter{}
	svc.SetRewardGranter(granter)
	created, err := svc.Create(context.Background(), &CreateInput{
		Title:      "Lucky",
		Type:       CampaignTypeLottery,
		Status:     CampaignStatusActive,
		ConfigJSON: `{"lottery":{"pools":[{"id":"pool-1","name":"Default","enabled":true,"daily_limit":2,"prizes":[{"id":"prize-1","label":"Balance","prize_type":"balance","value_amount":"5","weight":1,"color":"#22c55e"}]}]}}`,
	})
	require.NoError(t, err)

	record, err := svc.Participate(context.Background(), created.ID, ParticipateInput{UserID: 1001, PoolID: "pool-1"})

	require.NoError(t, err)
	require.Equal(t, CampaignTypeLottery, record.CampaignType)
	require.Equal(t, "pool-1", record.PoolID)
	require.Equal(t, "prize-1", record.PrizeID)
	require.Equal(t, "Balance", record.PrizeLabel)
	require.Equal(t, "balance", record.PrizeType)
	require.Equal(t, "#22c55e", record.PrizeColor)
	require.Equal(t, "won", record.ResultStatus)
	require.Equal(t, "granted", record.RewardStatus)
	require.Contains(t, record.RewardPayloadJSON, `"value_amount":"5"`)
	require.Len(t, repo.records, 1)
	require.Equal(t, []RewardGrant{{PrizeType: "balance", ValueAmount: "5"}}, granter.grants)
}

func TestCampaignServiceParticipateLotteryEnforcesDailyLimit(t *testing.T) {
	repo := newFakeCampaignRepo()
	svc := NewService(repo)
	created, err := svc.Create(context.Background(), &CreateInput{
		Title:      "Limited",
		Type:       CampaignTypeLottery,
		Status:     CampaignStatusActive,
		ConfigJSON: `{"lottery":{"pools":[{"id":"pool-1","enabled":true,"daily_limit":1,"prizes":[{"id":"prize-1","label":"Thanks","prize_type":"none","weight":1}]}]}}`,
	})
	require.NoError(t, err)

	_, err = svc.Participate(context.Background(), created.ID, ParticipateInput{UserID: 1001, PoolID: "pool-1"})
	require.NoError(t, err)
	_, err = svc.Participate(context.Background(), created.ID, ParticipateInput{UserID: 1001, PoolID: "pool-1"})

	require.ErrorIs(t, err, ErrCampaignDailyLimit)
}

func TestCampaignServiceParticipateLotterySkipsSoldOutPrize(t *testing.T) {
	repo := newFakeCampaignRepo()
	svc := NewService(repo)
	created, err := svc.Create(context.Background(), &CreateInput{
		Title:  "Stock",
		Type:   CampaignTypeLottery,
		Status: CampaignStatusActive,
		ConfigJSON: `{"lottery":{"pools":[{"id":"pool-1","enabled":true,"prizes":[` +
			`{"id":"sold-out","label":"Sold out","prize_type":"balance","value_amount":"5","weight":999,"available_count":0},` +
			`{"id":"fallback","label":"Thanks","prize_type":"none","weight":1}` +
			`] }]}}`,
	})
	require.NoError(t, err)

	record, err := svc.Participate(context.Background(), created.ID, ParticipateInput{UserID: 1001, PoolID: "pool-1"})

	require.NoError(t, err)
	require.Equal(t, "fallback", record.PrizeID)
	require.Equal(t, "none", record.ResultStatus)
	require.Equal(t, "none", record.RewardStatus)
}

func TestCampaignServicePrizeStockStatsReportsRemaining(t *testing.T) {
	repo := newFakeCampaignRepo()
	svc := NewService(repo)
	created, err := svc.Create(context.Background(), &CreateInput{
		Title:  "Stock stats",
		Type:   CampaignTypeLottery,
		Status: CampaignStatusActive,
		ConfigJSON: `{"lottery":{"pools":[{"id":"pool-1","enabled":true,"prizes":[` +
			`{"id":"prize-1","label":"Balance","prize_type":"balance","value_amount":"5","weight":1,"available_count":3}` +
			`] }]}}`,
	})
	require.NoError(t, err)

	repo.records = []Record{
		{CampaignID: created.ID, PrizeID: "prize-1"},
	}

	stats, err := svc.PrizeStockStats(context.Background(), created)

	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, "pool-1", stats[0].PoolID)
	require.Equal(t, "prize-1", stats[0].PrizeID)
	require.Equal(t, int64(1), stats[0].IssuedCount)
	require.NotNil(t, stats[0].AvailableCount)
	require.NotNil(t, stats[0].RemainingCount)
	require.Equal(t, int64(3), *stats[0].AvailableCount)
	require.Equal(t, int64(2), *stats[0].RemainingCount)
}

func TestCampaignServiceParticipateLotteryIssuesEachCardOnce(t *testing.T) {
	repo := newFakeCampaignRepo()
	svc := NewService(repo)
	granter := &fakeRewardGranter{}
	svc.SetRewardGranter(granter)
	created, err := svc.Create(context.Background(), &CreateInput{
		Title:      "Cards",
		Type:       CampaignTypeLottery,
		Status:     CampaignStatusActive,
		ConfigJSON: `{"lottery":{"pools":[{"id":"pool-1","enabled":true,"daily_limit":3,"prizes":[{"id":"card-1","label":"Card","prize_type":"card","weight":1,"codes":["CODE-A","CODE-B"]}]}]}}`,
	})
	require.NoError(t, err)

	first, err := svc.Participate(context.Background(), created.ID, ParticipateInput{UserID: 1001, PoolID: "pool-1"})
	require.NoError(t, err)
	second, err := svc.Participate(context.Background(), created.ID, ParticipateInput{UserID: 1001, PoolID: "pool-1"})
	require.NoError(t, err)
	_, err = svc.Participate(context.Background(), created.ID, ParticipateInput{UserID: 1001, PoolID: "pool-1"})

	require.ErrorIs(t, err, ErrCampaignNoPrize)
	require.Contains(t, first.RewardPayloadJSON, `"code":"CODE-A"`)
	require.Contains(t, second.RewardPayloadJSON, `"code":"CODE-B"`)
	require.Equal(t, "CODE-A", granter.grants[0].Code)
	require.Equal(t, "CODE-B", granter.grants[1].Code)
}

func TestCampaignServiceParticipateLotteryIgnoresCardAvailableCount(t *testing.T) {
	repo := newFakeCampaignRepo()
	svc := NewService(repo)
	granter := &fakeRewardGranter{}
	svc.SetRewardGranter(granter)
	created, err := svc.Create(context.Background(), &CreateInput{
		Title:  "Card stock",
		Type:   CampaignTypeLottery,
		Status: CampaignStatusActive,
		ConfigJSON: `{"lottery":{"pools":[{"id":"pool-1","enabled":true,"prizes":[` +
			`{"id":"card-1","label":"Card","prize_type":"card","weight":1,"available_count":0,"codes":["CODE-A","CODE-B"]}` +
			`] }]}}`,
	})
	require.NoError(t, err)

	first, err := svc.Participate(context.Background(), created.ID, ParticipateInput{UserID: 1001, PoolID: "pool-1"})
	require.NoError(t, err)
	second, err := svc.Participate(context.Background(), created.ID, ParticipateInput{UserID: 1001, PoolID: "pool-1"})
	require.NoError(t, err)
	_, err = svc.Participate(context.Background(), created.ID, ParticipateInput{UserID: 1001, PoolID: "pool-1"})

	require.ErrorIs(t, err, ErrCampaignNoPrize)
	require.Contains(t, first.RewardPayloadJSON, `"code":"CODE-A"`)
	require.Contains(t, second.RewardPayloadJSON, `"code":"CODE-B"`)
	require.Equal(t, 2, len(granter.grants))
}

func TestCampaignServiceParticipateLotteryRequiresEligibleGroup(t *testing.T) {
	repo := newFakeCampaignRepo()
	repo.eligibleSet = true
	repo.eligible = false
	svc := NewService(repo)
	created, err := svc.Create(context.Background(), &CreateInput{
		Title:      "Group only",
		Type:       CampaignTypeLottery,
		Status:     CampaignStatusActive,
		ConfigJSON: `{"lottery":{"pools":[{"id":"pool-1","required_group_ids":[7],"enabled":true,"prizes":[{"id":"prize-1","label":"A","prize_type":"none","weight":1}]}]}}`,
	})
	require.NoError(t, err)

	_, err = svc.Participate(context.Background(), created.ID, ParticipateInput{UserID: 1001, PoolID: "pool-1"})

	require.ErrorIs(t, err, ErrCampaignNotEligible)
	require.Empty(t, repo.records)
}

func TestCampaignServiceParticipateNonLotteryRecordsActivity(t *testing.T) {
	repo := newFakeCampaignRepo()
	svc := NewService(repo)
	created, err := svc.Create(context.Background(), &CreateInput{
		Title:  "Custom",
		Type:   CampaignTypeCustom,
		Status: CampaignStatusActive,
	})
	require.NoError(t, err)

	record, err := svc.Participate(context.Background(), created.ID, ParticipateInput{UserID: 1001})

	require.NoError(t, err)
	require.Equal(t, CampaignTypeCustom, record.CampaignType)
	require.Equal(t, "recorded", record.ResultStatus)
	require.Equal(t, "none", record.RewardStatus)
	require.Empty(t, record.PrizeID)
	require.Len(t, repo.records, 1)
}

func TestCampaignServiceListUserRecordsFiltersByUser(t *testing.T) {
	repo := newFakeCampaignRepo()
	repo.records = []Record{
		{ID: 1, UserID: 1001, CampaignID: 1, CampaignType: CampaignTypeLottery},
		{ID: 2, UserID: 2002, CampaignID: 1, CampaignType: CampaignTypeLottery},
	}
	svc := NewService(repo)

	records, page, err := svc.ListUserRecords(context.Background(), 1001, pagination.PaginationParams{Page: 1, PageSize: 20})

	require.NoError(t, err)
	require.Equal(t, int64(1), page.Total)
	require.Len(t, records, 1)
	require.Equal(t, int64(1001), records[0].UserID)
}

func TestCampaignServiceVisibleLotteryOnlyReturnsEligiblePools(t *testing.T) {
	repo := newFakeCampaignRepo()
	svc := NewService(repo)
	created, err := svc.Create(context.Background(), &CreateInput{
		Title:  "Eligible pools",
		Type:   CampaignTypeLottery,
		Status: CampaignStatusActive,
		ConfigJSON: `{"lottery":{"pools":[` +
			`{"id":"public","enabled":true,"prizes":[{"id":"public-none","label":"Public","prize_type":"none","weight":1}]},` +
			`{"id":"vip","required_group_ids":[7],"enabled":true,"prizes":[{"id":"vip-none","label":"VIP","prize_type":"none","weight":1}]}` +
			`]}}`,
	})
	require.NoError(t, err)

	item, err := svc.GetVisibleForUser(context.Background(), created.ID, 1001)
	require.NoError(t, err)
	config, err := parseActivityConfig(item.ConfigJSON)
	require.NoError(t, err)
	require.Len(t, config.Lottery.Pools, 1)
	require.Equal(t, "public", config.Lottery.Pools[0].ID)

	repo.groupIDs = []int64{7}
	item, err = svc.GetVisibleForUser(context.Background(), created.ID, 1001)
	require.NoError(t, err)
	config, err = parseActivityConfig(item.ConfigJSON)
	require.NoError(t, err)
	require.Len(t, config.Lottery.Pools, 2)
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

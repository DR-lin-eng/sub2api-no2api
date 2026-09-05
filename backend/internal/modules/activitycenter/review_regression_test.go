package activitycenter

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func reviewCheckinConfig() *ActivityConfig {
	c := &ActivityConfig{Checkin: &CheckinConfig{Timezone: "UTC", CycleType: "weekly"}}
	for day := 1; day <= 7; day++ {
		c.Checkin.DailyRewards = append(c.Checkin.DailyRewards, CheckinReward{Day: day, RewardType: "balance", Value: "1"})
	}
	return c
}
func reviewCampaign(t *testing.T, typ string, config *ActivityConfig) *Campaign {
	t.Helper()
	b, err := json.Marshal(config)
	require.NoError(t, err)
	return &Campaign{ID: 1, Title: "Review", Type: typ, Status: CampaignStatusActive, ConfigJSON: string(b)}
}
func TestReviewRejectsNonFiniteRewards(t *testing.T) {
	for _, value := range []string{"NaN", "+Inf", "-Inf"} {
		t.Run(value, func(t *testing.T) {
			c := reviewCheckinConfig()
			c.Checkin.DailyRewards[0].Value = value
			require.Error(t, normalizeCampaign(reviewCampaign(t, CampaignTypeCheckin, c)))
			c = &ActivityConfig{Lottery: &LotteryConfig{Pools: []LotteryPool{{ID: "p", Prizes: []LotteryPrize{{ID: "x", Label: "x", Weight: 1, PrizeType: "balance", ValueAmount: value}}}}}}
			require.Error(t, normalizeCampaign(reviewCampaign(t, CampaignTypeLottery, c)))
		})
	}
}
func TestReviewCheckinValidation(t *testing.T) {
	cases := map[string]func(*ActivityConfig){
		"incomplete calendar": func(c *ActivityConfig) { c.Checkin.DailyRewards = c.Checkin.DailyRewards[:1] },
		"invalid timezone":    func(c *ActivityConfig) { c.Checkin.Timezone = "Not/AZone" },
		"negative group":      func(c *ActivityConfig) { c.Checkin.RequiredGroupIDs = []int64{-1} },
		"invalid subscription duration": func(c *ActivityConfig) {
			c.Checkin.DailyRewards[0] = CheckinReward{Day: 1, RewardType: "subscription", Value: "oops", RewardGroupID: 1}
		},
	}
	for name, edit := range cases {
		t.Run(name, func(t *testing.T) {
			c := reviewCheckinConfig()
			edit(c)
			require.Error(t, normalizeCampaign(reviewCampaign(t, CampaignTypeCheckin, c)))
		})
	}
}
func TestReviewLegacyInflationValidation(t *testing.T) {
	c := &ActivityConfig{Redeem: &InflateConfig{MinValue: "NaN", MaxValue: "100", MinInflatePct: "10", MaxInflatePct: "10"}}
	require.Error(t, normalizeCampaign(reviewCampaign(t, CampaignTypeRedeem, c)))
	require.Error(t, normalizeCampaign(reviewCampaign(t, CampaignTypeInflate, c)))
}
func TestReviewCheckinCyclePreservesStreak(t *testing.T) {
	for _, streak := range []int{7, 8, 14} {
		t.Run(fmt.Sprint(streak), func(t *testing.T) {
			r := newFakeCampaignRepo()
			r.items[1] = reviewCampaign(t, CampaignTypeCheckin, reviewCheckinConfig())
			r.checkinRecords = []CheckinRecord{{CampaignID: 1, UserID: 7, CheckinDate: time.Now().UTC().AddDate(0, 0, -1), CycleDay: (streak-1)%7 + 1, StreakDays: streak, CycleNo: (streak-1)/7 + 1}}
			s := NewService(r)
			s.SetRewardGranter(&fakeRewardGranter{})
			_, status, err := s.Checkin(context.Background(), 1, 7)
			require.NoError(t, err)
			require.Equal(t, streak+1, status.StreakDays)
			require.Equal(t, streak%7+1, status.CycleDay)
			require.Equal(t, streak/7+1, r.checkinRecords[1].CycleNo)
		})
	}
}
func TestReviewCheckinRejectsOtherCampaignTypes(t *testing.T) {
	r := newFakeCampaignRepo()
	r.items[1] = reviewCampaign(t, CampaignTypeCustom, reviewCheckinConfig())
	s := NewService(r)
	s.SetRewardGranter(&fakeRewardGranter{})
	_, _, err := s.Checkin(context.Background(), 1, 7)
	require.Error(t, err)
	require.Empty(t, r.records)
}

type reviewCountingRepo struct {
	*fakeCampaignRepo
	countCalls, batchCalls int
}

func (r *reviewCountingRepo) CountPrizeRecords(ctx context.Context, campaignID int64, prizeID string) (int64, error) {
	r.countCalls++
	return r.fakeCampaignRepo.CountPrizeRecords(ctx, campaignID, prizeID)
}
func (r *reviewCountingRepo) CountPrizeRecordsBatch(_ context.Context, _ int64) (map[string]int64, error) {
	r.batchCalls++
	return map[string]int64{}, nil
}
func TestReviewDrawBatchesStockReads(t *testing.T) {
	r := &reviewCountingRepo{fakeCampaignRepo: newFakeCampaignRepo()}
	s := NewService(r)
	pool := &LotteryPool{ID: "pool"}
	stock := 10
	for i := 0; i < 30; i++ {
		pool.Prizes = append(pool.Prizes, LotteryPrize{ID: fmt.Sprint(i), PrizeType: "balance", ValueAmount: "1", Weight: 1, AvailableCount: &stock})
	}
	_, _, err := s.drawPrize(context.Background(), 1, pool)
	require.NoError(t, err)
	require.Equal(t, 0, r.countCalls, "no per-prize round trips")
	require.Equal(t, 1, r.batchCalls, "one stock batch")
}

func TestReviewCardRestockDoesNotReissueMovedCodes(t *testing.T) {
	r := newFakeCampaignRepo()
	r.records = []Record{{CampaignID: 1, PrizeID: "old", RewardPayloadJSON: `{"code":"issued"}`}}
	s := NewService(r)
	pool := &LotteryPool{ID: "p", Prizes: []LotteryPrize{{ID: "old", Label: "new stock", PrizeType: "card", Codes: []string{"fresh"}, Weight: 1}}}
	_, code, err := s.drawPrize(context.Background(), 1, pool)
	require.NoError(t, err)
	require.Equal(t, "fresh", code)
	pool.Prizes[0].ID = "new-id"
	pool.Prizes[0].Codes = []string{"issued"}
	_, _, err = s.drawPrize(context.Background(), 1, pool)
	require.ErrorIs(t, err, ErrCampaignNoPrize)
}

//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/modules/activitycenter"
	"github.com/stretchr/testify/require"
)

type activityTestGranter struct{ users service.UserRepository }

func (g activityTestGranter) Grant(ctx context.Context, userID int64, reward activitycenter.RewardGrant) error {
	if reward.PrizeType != "balance" {
		return nil
	}
	amount, err := strconv.ParseFloat(reward.ValueAmount, 64)
	if err != nil {
		return err
	}
	return g.users.UpdateBalance(ctx, userID, amount)
}
func (activityTestGranter) AfterCommit(context.Context, int64, activitycenter.RewardGrant) {}

func activityTestConfig() activitycenter.ActivityConfig {
	cfg := activitycenter.ActivityConfig{Checkin: &activitycenter.CheckinConfig{Timezone: "UTC", CycleType: "weekly"}}
	for i := 1; i <= 7; i++ {
		cfg.Checkin.DailyRewards = append(cfg.Checkin.DailyRewards, activitycenter.CheckinReward{Day: i, RewardType: "balance", Value: "1"})
	}
	return cfg
}
func activityTestSetup(t *testing.T, typ string, config activitycenter.ActivityConfig) (*activityCenterRepository, *activitycenter.Service, int64, int64) {
	t.Helper()
	ctx := context.Background()
	client := testEntClient(t)
	u, err := client.User.Create().SetEmail(fmt.Sprintf("activity-%d@example.test", time.Now().UnixNano())).SetPasswordHash("test-only").SetBalance(0).Save(ctx)
	require.NoError(t, err)
	repo := NewActivityCenterRepository(integrationDB, client).(*activityCenterRepository)
	svc := activitycenter.NewService(repo)
	svc.SetRewardGranter(activityTestGranter{NewUserRepository(client, integrationDB)})
	raw, err := json.Marshal(config)
	require.NoError(t, err)
	c, err := svc.Create(ctx, &activitycenter.CreateInput{Title: "PR31 integration", Type: typ, Status: activitycenter.CampaignStatusActive, ConfigJSON: string(raw)})
	require.NoError(t, err)
	t.Cleanup(func() {
		for _, table := range []string{"act_checkin_records", "act_checkin_summaries", "act_participation_records"} {
			_, _ = integrationDB.Exec("DELETE FROM "+table+" WHERE campaign_id=$1", c.ID)
		}
		_, _ = integrationDB.Exec("DELETE FROM act_campaigns WHERE id=$1", c.ID)
		_ = client.User.DeleteOneID(u.ID).Exec(ctx)
	})
	return repo, svc, c.ID, u.ID
}
func activityBalance(t *testing.T, userID int64) float64 {
	t.Helper()
	u, err := testEntClient(t).User.Get(context.Background(), userID)
	require.NoError(t, err)
	return u.Balance
}
func activityCount(t *testing.T, table string, campaignID int64) int {
	t.Helper()
	var n int
	err := integrationDB.QueryRow("SELECT COUNT(*) FROM "+table+" WHERE campaign_id=$1", campaignID).Scan(&n)
	require.NoError(t, err)
	return n
}

func TestActivityCheckinConcurrentExactlyOnce(t *testing.T) {
	_, svc, id, user := activityTestSetup(t, activitycenter.CampaignTypeCheckin, activityTestConfig())
	var successes atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, 16)
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, err := svc.Checkin(context.Background(), id, user)
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, activitycenter.ErrCampaignAlreadyCheckedIn) {
				errs <- err
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.EqualValues(t, 1, successes.Load())
	require.Equal(t, 1.0, activityBalance(t, user))
	require.Equal(t, 1, activityCount(t, "act_checkin_records", id))
	require.Equal(t, 1, activityCount(t, "act_participation_records", id))
	board, err := svc.CheckinLeaderboard(context.Background(), id)
	require.NoError(t, err)
	require.Len(t, board, 1)
	require.Equal(t, 1, board[0].CheckinCount)
	require.Empty(t, board[0].UserEmail)
}

type activityFailRecordRepo struct{ activitycenter.Repository }

func (activityFailRecordRepo) CreateRecord(context.Context, *activitycenter.Record) error {
	return errors.New("injected participation record failure")
}
func TestActivityRewardAndRecordRollback(t *testing.T) {
	repo, _, id, user := activityTestSetup(t, activitycenter.CampaignTypeCheckin, activityTestConfig())
	svc := activitycenter.NewService(activityFailRecordRepo{repo})
	svc.SetRewardGranter(activityTestGranter{NewUserRepository(testEntClient(t), integrationDB)})
	_, _, err := svc.Checkin(context.Background(), id, user)
	require.ErrorContains(t, err, "injected")
	require.Zero(t, activityBalance(t, user))
	for _, table := range []string{"act_checkin_records", "act_checkin_summaries", "act_participation_records"} {
		require.Zero(t, activityCount(t, table, id))
	}
}
func TestActivityLotteryFiniteStockConcurrent(t *testing.T) {
	stock := 3
	cfg := activitycenter.ActivityConfig{Lottery: &activitycenter.LotteryConfig{Pools: []activitycenter.LotteryPool{{ID: "p", Prizes: []activitycenter.LotteryPrize{{ID: "cash", Label: "cash", PrizeType: "balance", ValueAmount: "1", Weight: 1, AvailableCount: &stock}}}}}}
	_, svc, id, user := activityTestSetup(t, activitycenter.CampaignTypeLottery, cfg)
	var successes atomic.Int64
	var wg sync.WaitGroup
	errs := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.Participate(context.Background(), id, activitycenter.ParticipateInput{UserID: user, PoolID: "p"})
			if err == nil {
				successes.Add(1)
			} else if !errors.Is(err, activitycenter.ErrCampaignNoPrize) {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.EqualValues(t, 3, successes.Load())
	require.Equal(t, 3.0, activityBalance(t, user))
	require.Equal(t, 3, activityCount(t, "act_participation_records", id))
}
func TestActivityLotteryCardCodesExactlyOnce(t *testing.T) {
	cfg := activitycenter.ActivityConfig{Lottery: &activitycenter.LotteryConfig{Pools: []activitycenter.LotteryPool{{ID: "p", Prizes: []activitycenter.LotteryPrize{{ID: "card", Label: "card", PrizeType: "card", Codes: []string{"test-code-A", "test-code-B"}, Weight: 1}}}}}}
	_, svc, id, user := activityTestSetup(t, activitycenter.CampaignTypeLottery, cfg)
	var wg sync.WaitGroup
	records := make(chan *activitycenter.Record, 10)
	errs := make(chan error, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := svc.Participate(context.Background(), id, activitycenter.ParticipateInput{UserID: user, PoolID: "p"})
			if err == nil {
				records <- r
			} else if !errors.Is(err, activitycenter.ErrCampaignNoPrize) {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(records)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	codes := map[string]bool{}
	for record := range records {
		var payload struct {
			Code string `json:"code"`
		}
		require.NoError(t, json.Unmarshal([]byte(record.RewardPayloadJSON), &payload))
		require.False(t, codes[payload.Code])
		codes[payload.Code] = true
	}
	require.Len(t, codes, 2)
	require.Equal(t, 2, activityCount(t, "act_participation_records", id))
}
func TestActivityCheckinLocksAreUserScoped(t *testing.T) {
	repo, _, id, user := activityTestSetup(t, activitycenter.CampaignTypeCheckin, activityTestConfig())
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- repo.WithCheckinTx(context.Background(), id, user, func(context.Context) error { close(entered); <-release; return nil })
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := repo.WithCheckinTx(ctx, id, user+1, func(context.Context) error { return nil })
	close(release)
	require.NoError(t, <-done)
	require.NoError(t, err, "another user must not queue behind the whole campaign")
}
func TestActivityOuterTransactionRollback(t *testing.T) {
	repo, svc, id, user := activityTestSetup(t, activitycenter.CampaignTypeCheckin, activityTestConfig())
	tx, err := testEntClient(t).Tx(context.Background())
	require.NoError(t, err)
	_, _, err = svc.Checkin(dbent.NewTxContext(context.Background(), tx), id, user)
	require.NoError(t, err)
	require.NoError(t, tx.Rollback())
	require.Zero(t, activityBalance(t, user))
	require.Zero(t, activityCount(t, "act_checkin_summaries", id))
	counts, err := repo.CountPrizeRecordsBatch(context.Background(), id)
	require.NoError(t, err)
	require.Empty(t, counts)
}
func TestActivityGenericParticipationUsesValidJSON(t *testing.T) {
	_, svc, id, user := activityTestSetup(t, activitycenter.CampaignTypeCustom, activitycenter.ActivityConfig{})
	record, err := svc.Participate(context.Background(), id, activitycenter.ParticipateInput{UserID: user})
	require.NoError(t, err)
	require.Equal(t, "recorded", record.ResultStatus)
}

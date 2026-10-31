//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

func newVideoBillingFixture(t *testing.T, balance float64) (*usageBillingRepository, *service.BalanceHoldRecord) {
	t.Helper()
	client := testEntClient(t)
	user := mustCreateUser(t, client, &service.User{Email: uuid.NewString() + "@video.test", PasswordHash: "hash", Balance: balance})
	key := mustCreateApiKey(t, client, &service.APIKey{UserID: user.ID, Key: "sk-video-" + uuid.NewString(), Name: "video", Quota: 1})
	account := mustCreateAccount(t, client, &service.Account{
		Name: "video-" + uuid.NewString(), Type: service.AccountTypeAPIKey,
		Extra: map[string]any{"quota_limit": float64(100)},
	})
	t.Cleanup(func() {
		ctx := context.Background()
		for _, cleanup := range []struct {
			query string
			id    int64
		}{
			{`DELETE FROM usage_logs WHERE user_id=$1`, user.ID},
			{`DELETE FROM video_balance_holds WHERE user_id=$1`, user.ID},
			{`DELETE FROM usage_billing_dedup WHERE api_key_id=$1`, key.ID},
			{`DELETE FROM usage_billing_dedup_archive WHERE api_key_id=$1`, key.ID},
			{`DELETE FROM user_platform_quotas WHERE user_id=$1`, user.ID},
			{`DELETE FROM api_keys WHERE id=$1`, key.ID},
			{`DELETE FROM accounts WHERE id=$1`, account.ID},
			{`DELETE FROM users WHERE id=$1`, user.ID},
		} {
			_, err := integrationDB.ExecContext(ctx, cleanup.query, cleanup.id)
			require.NoError(t, err)
		}
	})
	return &usageBillingRepository{db: integrationDB}, &service.BalanceHoldRecord{
		HoldKey: uuid.NewString(), UserID: user.ID, APIKeyID: key.ID, AccountID: account.ID,
		Model: "seedance", Resolution: "720p", DurationSeconds: 8,
		HoldAmount: 2, BaseAmount: 4, RateMultiplier: .5, Currency: "USD",
		Status: service.BalanceHoldStatusPending, ExpiresAt: time.Now().Add(time.Hour),
	}
}

func videoCaptureForTest(record *service.BalanceHoldRecord, amount float64) *service.VideoBalanceCapture {
	seconds, resolution, mode := 4, "720p", "video"
	return &service.VideoBalanceCapture{HoldKey: record.HoldKey,
		Billing: &service.UsageBillingCommand{
			UserID: record.UserID, APIKeyID: record.APIKeyID, AccountID: record.AccountID,
			AccountType: service.AccountTypeAPIKey, GroupID: record.GroupID, Model: record.Model,
			APIKeyQuotaCost: amount, APIKeyRateLimitCost: amount, AccountQuotaCost: amount * 2,
			QuotaPlatform: service.PlatformMultimodal, UserPlatformQuotaCost: amount,
		},
		UsageLog: &service.UsageLog{
			UserID: record.UserID, APIKeyID: record.APIKeyID, AccountID: record.AccountID,
			RequestID: record.UpstreamRequestID, Model: record.Model, RequestedModel: record.Model,
			TotalCost: amount * 2, ActualCost: amount, RateMultiplier: .5,
			VideoCount: 1, VideoResolution: &resolution, VideoDurationSeconds: &seconds,
			BillingMode: &mode, BillingType: service.BillingTypeBalance, RequestType: service.RequestTypeSync,
		},
	}
}

func videoBalanceForTest(t *testing.T, record *service.BalanceHoldRecord) (float64, float64) {
	t.Helper()
	var balance, frozen float64
	require.NoError(t, integrationDB.QueryRow(`SELECT balance, frozen_balance FROM users WHERE id=$1`, record.UserID).Scan(&balance, &frozen))
	return balance, frozen
}

func reserveBoundVideo(t *testing.T, repo *usageBillingRepository, record *service.BalanceHoldRecord) *service.BalanceHoldRecord {
	t.Helper()
	created, err := repo.ReserveVideoBalance(context.Background(), record)
	require.NoError(t, err)
	created.UpstreamRequestID = "video-" + uuid.NewString()
	require.NoError(t, repo.BindBalanceHoldUpstreamRequest(context.Background(), created.HoldKey, created.UpstreamRequestID, created.AccountID))
	return created
}

func TestVideoBalanceReserveRollbackLeavesNoOrphan(t *testing.T) {
	repo, record := newVideoBillingFixture(t, 1)
	_, err := repo.ReserveVideoBalance(context.Background(), record)
	require.ErrorIs(t, err, service.ErrBalanceHoldInsufficientBalance)
	var rows int
	require.NoError(t, integrationDB.QueryRow(`SELECT count(*) FROM video_balance_holds WHERE hold_key=$1`, record.HoldKey).Scan(&rows))
	require.Zero(t, rows)
	require.NoError(t, integrationDB.QueryRow(`SELECT count(*) FROM usage_billing_dedup WHERE request_id=$1`, service.BalanceHoldRequestID(record.HoldKey)).Scan(&rows))
	require.Zero(t, rows)
	balance, frozen := videoBalanceForTest(t, record)
	require.InDelta(t, 1, balance, 1e-8)
	require.Zero(t, frozen)
}

func TestVideoBalanceCaptureAtomicLogQuotaRecoveryAndReplay(t *testing.T) {
	ctx := context.Background()
	repo, record := newVideoBillingFixture(t, 100)
	record = reserveBoundVideo(t, repo, record)
	input := videoCaptureForTest(record, .75)
	// Fail after money/quota updates but before the terminal write. PostgreSQL
	// must roll back every effect, including the dedup claim, then allow retry.
	name := "video_log_failure_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	identifier := pq.QuoteIdentifier(name)
	_, err := integrationDB.Exec(`CREATE FUNCTION ` + identifier + `() RETURNS trigger LANGUAGE plpgsql AS $$
	 BEGIN IF NEW.request_id = TG_ARGV[0] THEN RAISE EXCEPTION 'injected video log failure'; END IF; RETURN NEW; END $$`)
	require.NoError(t, err)
	_, err = integrationDB.Exec(`CREATE TRIGGER ` + identifier + ` BEFORE INSERT ON usage_logs FOR EACH ROW EXECUTE FUNCTION ` + identifier + `(` + pq.QuoteLiteral(record.UpstreamRequestID) + `)`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.Exec(`DROP TRIGGER IF EXISTS ` + identifier + ` ON usage_logs`)
		_, _ = integrationDB.Exec(`DROP FUNCTION IF EXISTS ` + identifier + `()`)
	})
	_, err = repo.CaptureVideoBalance(ctx, input)
	require.ErrorContains(t, err, "injected video log failure")
	balance, frozen := videoBalanceForTest(t, record)
	require.InDelta(t, 98, balance, 1e-8)
	require.InDelta(t, 2, frozen, 1e-8)
	var used float64
	require.NoError(t, integrationDB.QueryRow(`SELECT quota_used FROM api_keys WHERE id=$1`, record.APIKeyID).Scan(&used))
	require.Zero(t, used)
	loaded, err := repo.GetBalanceHoldByUpstreamRequestID(ctx, record.UpstreamRequestID)
	require.NoError(t, err)
	require.Equal(t, service.BalanceHoldStatusPending, loaded.Status)
	_, err = integrationDB.Exec(`DROP TRIGGER ` + identifier + ` ON usage_logs`)
	require.NoError(t, err)
	result, err := repo.CaptureVideoBalance(ctx, input)
	require.NoError(t, err)
	require.True(t, result.Billing.Applied)
	require.InDelta(t, .75, *result.Record.ActualAmount, 1e-8)
	// A different computed amount/duration on replay returns the stored result.
	replay, err := repo.CaptureVideoBalance(ctx, videoCaptureForTest(record, 1.5))
	require.NoError(t, err)
	require.False(t, replay.Billing.Applied)
	require.InDelta(t, .75, *replay.Record.ActualAmount, 1e-8)
	// A stale pending snapshot can never refund a captured hold.
	released, err := repo.ReleaseVideoBalance(ctx, record)
	require.NoError(t, err)
	require.Equal(t, service.BalanceHoldStatusSettled, released.Status)
	balance, frozen = videoBalanceForTest(t, record)
	require.InDelta(t, 99.25, balance, 1e-8)
	require.Zero(t, frozen)
	var keyUsage, rateUsage, accountUsage, platformUsage float64
	require.NoError(t, integrationDB.QueryRow(`SELECT quota_used, usage_5h FROM api_keys WHERE id=$1`, record.APIKeyID).Scan(&keyUsage, &rateUsage))
	require.NoError(t, integrationDB.QueryRow(`SELECT (extra->>'quota_used')::numeric FROM accounts WHERE id=$1`, record.AccountID).Scan(&accountUsage))
	require.NoError(t, integrationDB.QueryRow(`SELECT daily_usage_usd FROM user_platform_quotas WHERE user_id=$1 AND platform=$2`, record.UserID, service.PlatformMultimodal).Scan(&platformUsage))
	require.InDelta(t, .75, keyUsage, 1e-8)
	require.InDelta(t, .75, rateUsage, 1e-8)
	require.InDelta(t, 1.5, accountUsage, 1e-8)
	require.InDelta(t, .75, platformUsage, 1e-8)
	var logs int
	require.NoError(t, integrationDB.QueryRow(`SELECT count(*) FROM usage_logs WHERE request_id=$1 AND api_key_id=$2`, record.UpstreamRequestID, record.APIKeyID).Scan(&logs))
	require.Equal(t, 1, logs)
}

func TestVideoBalanceCaptureReleaseRacePreservesOtherHold(t *testing.T) {
	ctx := context.Background()
	repo, record := newVideoBillingFixture(t, 100)
	record = reserveBoundVideo(t, repo, record)
	other := *record
	other.HoldKey, other.UpstreamRequestID, other.HoldAmount = uuid.NewString(), "", 3
	_, err := repo.ReserveVideoBalance(ctx, &other)
	require.NoError(t, err)
	start := make(chan struct{})
	errors := make(chan error, 20)
	var wg sync.WaitGroup
	for index := 0; index < 20; index++ {
		wg.Add(1)
		go func(release bool) {
			defer wg.Done()
			<-start
			var err error
			if release {
				_, err = repo.ReleaseVideoBalance(ctx, record)
			} else {
				_, err = repo.CaptureVideoBalance(ctx, videoCaptureForTest(record, .75))
				// Losing a release race must fail closed before delivery.
				if err == service.ErrBalanceHoldFailed {
					err = nil
				}
			}
			errors <- err
		}(index%2 == 0)
	}
	close(start)
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}
	loaded, err := repo.GetBalanceHoldByUpstreamRequestID(ctx, record.UpstreamRequestID)
	require.NoError(t, err)
	balance, frozen := videoBalanceForTest(t, record)
	require.InDelta(t, 3, frozen, 1e-8, "the other hold's frozen funds must remain untouched")
	switch loaded.Status {
	case service.BalanceHoldStatusReleased:
		require.InDelta(t, 97, balance, 1e-8)
	case service.BalanceHoldStatusSettled:
		require.InDelta(t, 96.25, balance, 1e-8)
	default:
		t.Fatalf("unexpected terminal status: %s", loaded.Status)
	}
}

func TestVideoBalanceExpiryGraceDoesNotStarveUnboundRefunds(t *testing.T) {
	ctx := context.Background()
	repo := &usageBillingRepository{db: integrationDB}
	prefix := "sweep-" + uuid.NewString()
	_, err := integrationDB.ExecContext(ctx, `INSERT INTO video_balance_holds
	 (hold_key, upstream_request_id, user_id, api_key_id, expires_at)
	 SELECT $1 || i::text, $1 || i::text, 1, 1, NOW()-INTERVAL '1 hour' FROM generate_series(1, 201) i`, prefix)
	require.NoError(t, err)
	unbound := prefix + "-unbound"
	_, err = integrationDB.ExecContext(ctx, `INSERT INTO video_balance_holds
	 (hold_key, user_id, api_key_id, expires_at) VALUES ($1, 1, 1, NOW()-INTERVAL '1 minute')`, unbound)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.Exec(`DELETE FROM video_balance_holds WHERE hold_key LIKE $1`, prefix+"%")
	})
	rows, err := repo.ListExpiredBalanceHolds(ctx, time.Now(), 200)
	require.NoError(t, err)
	found := false
	for _, record := range rows {
		if record.HoldKey == unbound {
			found = true
		}
	}
	require.True(t, found, fmt.Sprintf("unbound refund starved behind %d grace-period holds", 201))
}

func TestVideoBalanceLegacyInterruptedCaptureCannotBeRefunded(t *testing.T) {
	ctx := context.Background()
	repo, record := newVideoBillingFixture(t, 100)
	record = reserveBoundVideo(t, repo, record)
	// A preview node may have committed capture separately from its hold state.
	// A digest alone cannot prove the captured amount; do not invent a refund.
	_, err := integrationDB.ExecContext(ctx, `INSERT INTO usage_billing_dedup
	 (request_id, api_key_id, request_fingerprint) VALUES ($1, $2, 'preview-capture')`,
		service.BalanceCaptureRequestID(record.HoldKey), record.APIKeyID)
	require.NoError(t, err)
	_, err = repo.ReleaseVideoBalance(ctx, record)
	require.ErrorContains(t, err, "requires capture reconciliation")
	_, err = repo.CaptureVideoBalance(ctx, videoCaptureForTest(record, .75))
	require.Error(t, err)
	loaded, err := repo.GetBalanceHoldByUpstreamRequestID(ctx, record.UpstreamRequestID)
	require.NoError(t, err)
	require.Equal(t, service.BalanceHoldStatusPending, loaded.Status)
	balance, frozen := videoBalanceForTest(t, record)
	require.InDelta(t, 98, balance, 1e-8)
	require.InDelta(t, 2, frozen, 1e-8)
}

func TestVideoBalanceLegacyInterruptedReleaseCannotCaptureOtherFunds(t *testing.T) {
	ctx := context.Background()
	repo, record := newVideoBillingFixture(t, 100)
	record = reserveBoundVideo(t, repo, record)
	_, err := integrationDB.ExecContext(ctx, `INSERT INTO usage_billing_dedup
	 (request_id, api_key_id, request_fingerprint) VALUES ($1, $2, 'preview-release')`,
		service.BalanceReleaseRequestID(record.HoldKey), record.APIKeyID)
	require.NoError(t, err)
	_, err = repo.CaptureVideoBalance(ctx, videoCaptureForTest(record, .75))
	require.ErrorContains(t, err, "requires release reconciliation")
	balance, frozen := videoBalanceForTest(t, record)
	require.InDelta(t, 98, balance, 1e-8)
	require.InDelta(t, 2, frozen, 1e-8)
}

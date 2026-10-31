package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

var _ service.VideoBalanceHoldRepository = (*usageBillingRepository)(nil)
var _ service.VideoBalanceHoldRepository = (*queuedUsageBillingRepository)(nil)

const videoBalanceHoldColumns = `hold_key, request_id, upstream_request_id, user_id, api_key_id,
 COALESCE(group_id, 0), COALESCE(account_id, 0), model, resolution, duration_seconds,
 hold_amount, actual_amount, currency, status, expires_at, created_at, updated_at,
 base_amount, rate_multiplier, settled_duration_seconds`

func validVideoMoney(amount float64) bool {
	return amount >= 0 && !math.IsNaN(amount) && !math.IsInf(amount, 0)
}

// ReserveVideoBalance commits the recoverable hold and the money movement
// together. A canceled request or failed insert cannot orphan frozen funds.
func (r *usageBillingRepository) ReserveVideoBalance(ctx context.Context, record *service.BalanceHoldRecord) (*service.BalanceHoldRecord, error) {
	if record == nil || record.HoldKey == "" || record.UserID <= 0 || record.APIKeyID <= 0 ||
		!validVideoMoney(record.HoldAmount) || !validVideoMoney(record.BaseAmount) || !validVideoMoney(record.RateMultiplier) {
		return nil, service.ErrBalanceHoldFailed
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	expiresAt := record.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = time.Now().Add(defaultBalanceHoldTTL)
	}
	created, err := scanBalanceHoldRecord(tx.QueryRowContext(ctx, `
		INSERT INTO video_balance_holds (
		 hold_key, request_id, user_id, api_key_id, group_id, account_id,
		 model, resolution, duration_seconds, hold_amount, currency, status, expires_at,
		 base_amount, rate_multiplier)
		VALUES ($1, $2, $3, $4, NULLIF($5, 0), NULLIF($6, 0), $7, $8, $9, $10,
		        $11, 'pending', $12, $13, $14)
		RETURNING `+videoBalanceHoldColumns,
		record.HoldKey, record.RequestID, record.UserID, record.APIKeyID, record.GroupID, record.AccountID,
		record.Model, record.Resolution, record.DurationSeconds, record.HoldAmount, record.Currency,
		expiresAt, record.BaseAmount, record.RateMultiplier))
	if err != nil {
		return nil, err
	}
	cmd := &service.BalanceHoldCommand{
		RequestID: service.BalanceHoldRequestID(record.HoldKey), APIKeyID: record.APIKeyID,
		UserID: record.UserID, HoldKey: record.HoldKey, HoldAmount: record.HoldAmount,
	}
	cmd.Normalize()
	if _, err := r.claimUsageBillingRequest(ctx, tx, cmd.RequestID, cmd.APIKeyID, cmd.RequestFingerprint); err != nil {
		return nil, err
	}
	if _, err := reserveUsageBillingBalance(ctx, tx, cmd); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return created, nil
}

func lockVideoBalanceHold(ctx context.Context, tx *sql.Tx, holdKey string, userID, apiKeyID int64) (*service.BalanceHoldRecord, error) {
	record, err := scanBalanceHoldRecord(tx.QueryRowContext(ctx,
		`SELECT `+videoBalanceHoldColumns+` FROM video_balance_holds WHERE hold_key = $1 FOR UPDATE`, strings.TrimSpace(holdKey)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, service.ErrBalanceHoldNotFound
	}
	if err != nil {
		return nil, err
	}
	if record.UserID != userID || record.APIKeyID != apiKeyID {
		return nil, service.ErrBalanceHoldNotFound
	}
	return record, nil
}

// CaptureVideoBalance serializes capture/release on the hold row. The usage
// claim, money, quotas, scheduler/auth outboxes, usage log and settled status
// either all commit or all roll back. Replays return the committed amount.
func (r *usageBillingRepository) CaptureVideoBalance(ctx context.Context, input *service.VideoBalanceCapture) (*service.VideoBalanceCaptureResult, error) {
	if input == nil || input.Billing == nil || input.UsageLog == nil {
		return nil, service.ErrBalanceHoldFailed
	}
	cmd := *input.Billing
	log := *input.UsageLog
	if !validVideoMoney(log.ActualCost) || !validVideoMoney(log.TotalCost) ||
		cmd.BalanceCost != 0 || cmd.SubscriptionCost != 0 ||
		log.UserID != cmd.UserID || log.APIKeyID != cmd.APIKeyID {
		return nil, service.ErrBalanceHoldFailed
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	record, err := lockVideoBalanceHold(ctx, tx, input.HoldKey, cmd.UserID, cmd.APIKeyID)
	if err != nil {
		return nil, err
	}
	result := &service.VideoBalanceCaptureResult{Record: record, Billing: &service.UsageBillingApplyResult{}}
	if record.Status == service.BalanceHoldStatusSettled {
		return result, nil
	}
	if record.Status != service.BalanceHoldStatusPending || record.UpstreamRequestID == "" ||
		record.AccountID != cmd.AccountID || record.GroupID != cmd.GroupID || log.ActualCost > record.HoldAmount {
		return nil, service.ErrBalanceHoldFailed
	}
	released, err := usageBillingHoldClaimExists(ctx, tx, service.BalanceReleaseRequestID(record.HoldKey), record.APIKeyID)
	if err != nil {
		return nil, err
	}
	if released {
		return nil, fmt.Errorf("pending video hold requires release reconciliation: %s", record.HoldKey)
	}
	cmd.RequestID = service.BalanceCaptureRequestID(record.HoldKey)
	cmd.Normalize()
	applied, err := r.claimUsageBillingKey(ctx, tx, &cmd)
	if err != nil {
		return nil, err
	}
	if !applied {
		// A pending row with an older capture claim predates atomic settlement.
		// Its amount cannot be reconstructed from a digest. Preserve funds and
		// require reconciliation instead of guessing or refunding a paid task.
		return nil, fmt.Errorf("pending video hold has a previous capture claim: %s", record.HoldKey)
	}
	amount := service.QuantizeUsageBillingAmount(log.ActualCost)
	funds, err := captureUsageBillingBalance(ctx, tx, &service.BalanceHoldCommand{
		UserID: record.UserID, HoldKey: record.HoldKey, HoldAmount: record.HoldAmount, ActualAmount: amount,
	})
	if err != nil {
		return nil, err
	}
	result.Billing.Applied = true
	result.Billing.NewBalance = funds.NewBalance
	if err := r.applyUsageBillingEffects(ctx, tx, &cmd, result.Billing); err != nil {
		return nil, err
	}
	if cmd.APIKeyQuotaCost > 0 && cmd.APIKeyAuthCacheKey != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO auth_cache_invalidation_outbox (cache_key) VALUES ($1)`, cmd.APIKeyAuthCacheKey); err != nil {
			return nil, err
		}
	}
	if err := execUsageLogInsertNoResult(ctx, tx, prepareUsageLogInsert(&log)); err != nil {
		return nil, err
	}
	seconds := record.DurationSeconds
	if log.VideoDurationSeconds != nil {
		seconds = *log.VideoDurationSeconds
	}
	if _, err := tx.ExecContext(ctx, `UPDATE video_balance_holds
		SET status = 'settled', actual_amount = $2, settled_duration_seconds = $3, updated_at = NOW()
		WHERE hold_key = $1`, record.HoldKey, amount, seconds); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	record.Status, record.ActualAmount, record.SettledDurationSeconds = service.BalanceHoldStatusSettled, &amount, seconds
	return result, nil
}

func (r *usageBillingRepository) ReleaseVideoBalance(ctx context.Context, snapshot *service.BalanceHoldRecord) (*service.BalanceHoldRecord, error) {
	if snapshot == nil {
		return nil, service.ErrBalanceHoldNotFound
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	record, err := lockVideoBalanceHold(ctx, tx, snapshot.HoldKey, snapshot.UserID, snapshot.APIKeyID)
	if err != nil {
		return nil, err
	}
	if record.Status != service.BalanceHoldStatusPending {
		return record, nil
	}
	if record.UpstreamRequestID != snapshot.UpstreamRequestID {
		return nil, service.ErrBalanceHoldFailed
	}
	// Do not refund an interrupted capture performed by an earlier preview build.
	claimed, err := usageBillingHoldClaimExists(ctx, tx, service.BalanceCaptureRequestID(record.HoldKey), record.APIKeyID)
	if err != nil {
		return nil, err
	}
	if claimed {
		return nil, fmt.Errorf("pending video hold requires capture reconciliation: %s", record.HoldKey)
	}
	cmd := &service.BalanceHoldCommand{
		RequestID: service.BalanceReleaseRequestID(record.HoldKey), APIKeyID: record.APIKeyID,
		UserID: record.UserID, HoldKey: record.HoldKey, HoldAmount: record.HoldAmount,
	}
	cmd.Normalize()
	applied, err := r.claimUsageBillingRequest(ctx, tx, cmd.RequestID, cmd.APIKeyID, cmd.RequestFingerprint)
	if err != nil {
		return nil, err
	}
	if applied {
		if _, err := releaseUsageBillingBalance(ctx, tx, cmd); err != nil {
			return nil, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE video_balance_holds SET status = 'released', updated_at = NOW() WHERE hold_key = $1`, record.HoldKey); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	record.Status = service.BalanceHoldStatusReleased
	return record, nil
}

func (r *queuedUsageBillingRepository) ReserveVideoBalance(ctx context.Context, record *service.BalanceHoldRecord) (*service.BalanceHoldRecord, error) {
	return r.direct.ReserveVideoBalance(ctx, record)
}

func (r *queuedUsageBillingRepository) CaptureVideoBalance(ctx context.Context, input *service.VideoBalanceCapture) (*service.VideoBalanceCaptureResult, error) {
	result, err := r.direct.CaptureVideoBalance(ctx, input)
	if err == nil && result != nil && result.Billing != nil && result.Billing.Applied {
		// Same cache invalidation protocol as ordinary durable billing. The
		// money transaction is already committed; Redis failure never retries it.
		cmd := *input.Billing
		cmd.RequestID = service.BalanceCaptureRequestID(input.HoldKey)
		cmd.Normalize()
		_ = r.completePendingOverlay(&cmd)
	}
	return result, err
}

func (r *queuedUsageBillingRepository) ReleaseVideoBalance(ctx context.Context, record *service.BalanceHoldRecord) (*service.BalanceHoldRecord, error) {
	return r.direct.ReleaseVideoBalance(ctx, record)
}

func (r *queuedUsageBillingRepository) BindBalanceHoldUpstreamRequest(ctx context.Context, key, upstream string, accountID int64) error {
	return r.direct.BindBalanceHoldUpstreamRequest(ctx, key, upstream, accountID)
}

func (r *queuedUsageBillingRepository) GetBalanceHoldByUpstreamRequestID(ctx context.Context, upstream string) (*service.BalanceHoldRecord, error) {
	return r.direct.GetBalanceHoldByUpstreamRequestID(ctx, upstream)
}

func (r *queuedUsageBillingRepository) ListExpiredBalanceHolds(ctx context.Context, now time.Time, limit int) ([]*service.BalanceHoldRecord, error) {
	return r.direct.ListExpiredBalanceHolds(ctx, now, limit)
}

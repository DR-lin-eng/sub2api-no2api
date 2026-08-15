package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

func (r *queuedUsageBillingRepository) GetUsageBillingQueueSnapshot(ctx context.Context) (*service.UsageBillingQueueSnapshot, error) {
	if r == nil || r.db == nil {
		return nil, service.ErrUsageBillingQueueUnavailable
	}
	snapshot := &service.UsageBillingQueueSnapshot{CollectedAt: time.Now().UTC()}
	err := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE settled_at IS NULL),
			COUNT(*) FILTER (WHERE settled_at IS NOT NULL),
			COUNT(*) FILTER (WHERE reconcile_required_at IS NOT NULL),
			COALESCE(EXTRACT(EPOCH FROM NOW() - MIN(created_at) FILTER (WHERE settled_at IS NULL)), 0),
			COALESCE(EXTRACT(EPOCH FROM NOW() - MIN(settled_at) FILTER (WHERE settled_at IS NOT NULL)), 0),
			COALESCE(MAX(attempts), 0),
			COALESCE(MAX(cleanup_attempts), 0),
			COALESCE(SUM(attempts), 0),
			COALESCE(SUM(cleanup_attempts), 0),
			(SELECT COUNT(*) FROM usage_billing_dead_letters)
		FROM usage_billing_jobs
	`).Scan(
		&snapshot.UnsettledCount,
		&snapshot.CleanupPendingCount,
		&snapshot.ReconcileRequiredCount,
		&snapshot.OldestUnsettledAgeSeconds,
		&snapshot.OldestCleanupAgeSeconds,
		&snapshot.MaxSettlementAttempts,
		&snapshot.MaxCleanupAttempts,
		&snapshot.SettlementAttemptsTotal,
		&snapshot.CleanupAttemptsTotal,
		&snapshot.DeadLetterCount,
	)
	if err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT COALESCE(last_error_class, ''), COUNT(*)
		FROM usage_billing_jobs
		WHERE last_error_class IS NOT NULL AND last_error_class <> ''
		GROUP BY last_error_class
	`)
	if err != nil {
		return nil, err
	}
	snapshot.ErrorClassCounts = make(map[string]int64)
	for rows.Next() {
		var class string
		var count int64
		if err := rows.Scan(&class, &count); err != nil {
			_ = rows.Close()
			return nil, err
		}
		snapshot.ErrorClassCounts[class] = count
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	snapshot.Node = r.UsageBillingQueueNodeStats()
	snapshot.Alerting = snapshot.DeadLetterCount > 0 ||
		snapshot.ReconcileRequiredCount > 0 ||
		snapshot.MaxSettlementAttempts >= r.retryAlertThreshold() ||
		snapshot.MaxCleanupAttempts >= r.retryAlertThreshold() ||
		snapshot.OldestUnsettledAgeSeconds >= r.oldestAgeThreshold().Seconds() ||
		snapshot.OldestCleanupAgeSeconds >= r.oldestAgeThreshold().Seconds()
	return snapshot, nil
}

func (r *queuedUsageBillingRepository) ListUsageBillingQueueJobs(ctx context.Context, filter service.UsageBillingQueueJobFilter) (*service.UsageBillingQueueJobList, error) {
	if r == nil || r.db == nil {
		return nil, service.ErrUsageBillingQueueUnavailable
	}
	filter = normalizeUsageBillingQueueJobFilter(filter)
	state := strings.ToLower(strings.TrimSpace(filter.State))
	if state != "" && state != "all" && state != "unsettled" && state != "cleanup" && state != "reconcile" {
		return nil, fmt.Errorf("%w: invalid state %q", service.ErrUsageBillingQueueFilterInvalid, state)
	}
	query := strings.TrimSpace(filter.Query)
	var total int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM usage_billing_jobs
		WHERE (
			$1 = '' OR $1 = 'all'
			OR ($1 = 'unsettled' AND settled_at IS NULL)
			OR ($1 = 'cleanup' AND settled_at IS NOT NULL)
			OR ($1 = 'reconcile' AND reconcile_required_at IS NOT NULL)
		)
		AND (
			$2 = ''
			OR request_id ILIKE '%' || $2 || '%'
			OR api_key_id::text = $2
		)
	`, state, query).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT
			id, request_id, api_key_id,
			CASE
				WHEN reconcile_required_at IS NOT NULL THEN 'reconcile'
				WHEN settled_at IS NOT NULL THEN 'cleanup'
				ELSE 'unsettled'
			END,
			attempts, cleanup_attempts, available_at,
			COALESCE(last_error, ''), COALESCE(last_error_class, ''),
			created_at, updated_at, settled_at, last_attempt_at,
			COALESCE(last_claimed_by, ''), reconcile_required_at
		FROM usage_billing_jobs
		WHERE (
			$1 = '' OR $1 = 'all'
			OR ($1 = 'unsettled' AND settled_at IS NULL)
			OR ($1 = 'cleanup' AND settled_at IS NOT NULL)
			OR ($1 = 'reconcile' AND reconcile_required_at IS NOT NULL)
		)
		AND (
			$2 = ''
			OR request_id ILIKE '%' || $2 || '%'
			OR api_key_id::text = $2
		)
		ORDER BY COALESCE(reconcile_required_at, created_at) DESC, id DESC
		LIMIT $3 OFFSET $4
	`, state, query, filter.PageSize, (filter.Page-1)*filter.PageSize)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.UsageBillingQueueJob, 0, filter.PageSize)
	for rows.Next() {
		var item service.UsageBillingQueueJob
		var settledAt, lastAttemptAt, reconcileAt sql.NullTime
		if err := rows.Scan(
			&item.ID,
			&item.RequestID,
			&item.APIKeyID,
			&item.State,
			&item.Attempts,
			&item.CleanupAttempts,
			&item.AvailableAt,
			&item.LastError,
			&item.LastErrorClass,
			&item.CreatedAt,
			&item.UpdatedAt,
			&settledAt,
			&lastAttemptAt,
			&item.LastClaimedBy,
			&reconcileAt,
		); err != nil {
			return nil, err
		}
		item.SettledAt = nullTimePointer(settledAt)
		item.LastAttemptAt = nullTimePointer(lastAttemptAt)
		item.ReconcileRequiredAt = nullTimePointer(reconcileAt)
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &service.UsageBillingQueueJobList{Jobs: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (r *queuedUsageBillingRepository) ListUsageBillingDeadLetters(ctx context.Context, filter service.UsageBillingQueueJobFilter) (*service.UsageBillingDeadLetterList, error) {
	if r == nil || r.db == nil {
		return nil, service.ErrUsageBillingQueueUnavailable
	}
	filter = normalizeUsageBillingQueueJobFilter(filter)
	query := strings.TrimSpace(filter.Query)
	var total int64
	if err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM usage_billing_dead_letters
		WHERE $1 = ''
			OR request_id ILIKE '%' || $1 || '%'
			OR api_key_id::text = $1
			OR reason ILIKE '%' || $1 || '%'
	`, query).Scan(&total); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, source_job_id, request_id, api_key_id, attempts, reason,
			created_at, failed_at, replay_count, last_replayed_at,
			last_replayed_by, COALESCE(replay_reason, '')
		FROM usage_billing_dead_letters
		WHERE $1 = ''
			OR request_id ILIKE '%' || $1 || '%'
			OR api_key_id::text = $1
			OR reason ILIKE '%' || $1 || '%'
		ORDER BY failed_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`, query, filter.PageSize, (filter.Page-1)*filter.PageSize)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	items := make([]service.UsageBillingDeadLetter, 0, filter.PageSize)
	for rows.Next() {
		var item service.UsageBillingDeadLetter
		var replayedAt sql.NullTime
		var replayedBy sql.NullInt64
		if err := rows.Scan(
			&item.ID,
			&item.SourceJobID,
			&item.RequestID,
			&item.APIKeyID,
			&item.Attempts,
			&item.Reason,
			&item.CreatedAt,
			&item.FailedAt,
			&item.ReplayCount,
			&replayedAt,
			&replayedBy,
			&item.ReplayReason,
		); err != nil {
			return nil, err
		}
		item.LastReplayedAt = nullTimePointer(replayedAt)
		if replayedBy.Valid {
			value := replayedBy.Int64
			item.LastReplayedBy = &value
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &service.UsageBillingDeadLetterList{DeadLetters: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (r *queuedUsageBillingRepository) RetryUsageBillingQueueJob(ctx context.Context, input service.UsageBillingJobRetry) error {
	reason, err := normalizeUsageBillingAdminAction(input.OperatorID, input.Reason)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var requestID string
	var apiKeyID int64
	if err := tx.QueryRowContext(ctx, `
		UPDATE usage_billing_jobs
		SET available_at = NOW(),
			reconcile_required_at = NULL,
			last_error = NULL,
			last_error_class = NULL,
			updated_at = NOW()
		WHERE id = $1
		RETURNING request_id, api_key_id
	`, input.JobID).Scan(&requestID, &apiKeyID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.ErrUsageBillingQueueJobNotFound
		}
		return err
	}
	if err := insertUsageBillingAdminAction(ctx, tx, input.OperatorID, "retry", &input.JobID, nil, requestID, apiKeyID, reason); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	r.signalConsumersAfterCommit()
	return nil
}

func (r *queuedUsageBillingRepository) ReplayUsageBillingDeadLetter(ctx context.Context, input service.UsageBillingDeadLetterReplay) error {
	reason, err := normalizeUsageBillingAdminAction(input.OperatorID, input.Reason)
	if err != nil {
		return err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var sourceJobID, apiKeyID int64
	var requestID, fingerprint string
	var payload []byte
	if err := tx.QueryRowContext(ctx, `
		SELECT source_job_id, request_id, api_key_id, request_fingerprint, payload
		FROM usage_billing_dead_letters
		WHERE id = $1
		FOR UPDATE
	`, input.DeadLetterID).Scan(&sourceJobID, &requestID, &apiKeyID, &fingerprint, &payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.ErrUsageBillingDeadLetterNotFound
		}
		return err
	}
	var cmd service.UsageBillingCommand
	if err := json.Unmarshal(payload, &cmd); err != nil {
		return fmt.Errorf("%w: %v", service.ErrUsageBillingDeadLetterInvalid, err)
	}
	cmd.Normalize()
	if cmd.RequestID != requestID || cmd.APIKeyID != apiKeyID || cmd.RequestFingerprint != fingerprint {
		return fmt.Errorf("%w: identity mismatch", service.ErrUsageBillingDeadLetterInvalid)
	}
	var replayedJobID int64
	if err := tx.QueryRowContext(ctx, `
		INSERT INTO usage_billing_jobs (
			request_id, api_key_id, request_fingerprint, payload, available_at
		) VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (request_id, api_key_id) DO UPDATE SET
			settled_at = NULL,
			available_at = NOW(),
			attempts = 0,
			cleanup_attempts = 0,
			reconcile_required_at = NULL,
			last_error = NULL,
			last_error_class = NULL,
			updated_at = NOW()
		WHERE usage_billing_jobs.request_fingerprint = EXCLUDED.request_fingerprint
		RETURNING id
	`, requestID, apiKeyID, fingerprint, payload).Scan(&replayedJobID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return service.ErrUsageBillingRequestConflict
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE usage_billing_dead_letters
		SET replay_count = replay_count + 1,
			last_replayed_at = NOW(),
			last_replayed_by = $2,
			replay_reason = $3
		WHERE id = $1
	`, input.DeadLetterID, input.OperatorID, reason); err != nil {
		return err
	}
	if err := insertUsageBillingAdminAction(ctx, tx, input.OperatorID, "replay", &replayedJobID, &input.DeadLetterID, requestID, apiKeyID, reason); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	r.signalConsumersAfterCommit()
	_ = sourceJobID
	return nil
}

func insertUsageBillingAdminAction(
	ctx context.Context,
	tx *sql.Tx,
	operatorID int64,
	action string,
	jobID, deadLetterID *int64,
	requestID string,
	apiKeyID int64,
	reason string,
) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO usage_billing_admin_actions (
			operator_id, action, source_job_id, source_dead_letter_id,
			request_id, api_key_id, reason
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, operatorID, action, jobID, deadLetterID, requestID, apiKeyID, reason)
	return err
}

func normalizeUsageBillingQueueJobFilter(filter service.UsageBillingQueueJobFilter) service.UsageBillingQueueJobFilter {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 50
	}
	filter.PageSize = min(filter.PageSize, 200)
	return filter
}

func normalizeUsageBillingAdminAction(operatorID int64, reason string) (string, error) {
	if operatorID <= 0 {
		return "", service.ErrUsageBillingAdminReasonRequired
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "", service.ErrUsageBillingAdminReasonRequired
	}
	if len(reason) > 2000 {
		reason = reason[:2000]
	}
	return reason, nil
}

func nullTimePointer(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

var _ service.UsageBillingQueueAdmin = (*queuedUsageBillingRepository)(nil)

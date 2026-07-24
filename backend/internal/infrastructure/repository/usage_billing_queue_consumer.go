package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

func (r *queuedUsageBillingRepository) runConsumer(ctx context.Context, workerID int) {
	defer r.wg.Done()
	// One leader performs startup and cross-instance discovery. Other workers
	// stay event-driven until a local enqueue or discovered backlog wakes them.
	if workerID != 0 && !r.waitForConsumer(ctx, workerID) {
		return
	}
	for ctx.Err() == nil {
		processed, err := r.processJobBatch(ctx)
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("durable usage billing consumer failed", "worker", workerID, "error", err)
		}
		if processed > 0 {
			if workerID == 0 {
				r.wakeConsumers()
			}
			continue
		}
		if !r.waitForConsumer(ctx, workerID) {
			return
		}
	}
}

func (r *queuedUsageBillingRepository) waitForConsumer(ctx context.Context, workerID int) bool {
	if workerID != 0 {
		select {
		case <-ctx.Done():
			return false
		case <-r.wakeCh:
			return true
		}
	}

	timer := time.NewTimer(r.pollInterval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-r.wakeCh:
		return true
	case <-timer.C:
		return true
	}
}

func (r *queuedUsageBillingRepository) processJobBatch(parent context.Context) (_ int, err error) {
	ctx, cancel := context.WithTimeout(parent, r.commandTimeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, request_id, api_key_id, request_fingerprint, payload, attempts, created_at
		FROM usage_billing_jobs
		WHERE available_at <= NOW()
		ORDER BY available_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT $1
	`, r.readBatchSize)
	if err != nil {
		return 0, err
	}
	jobs := make([]usageBillingJob, 0, r.readBatchSize)
	for rows.Next() {
		var job usageBillingJob
		if err := rows.Scan(&job.id, &job.requestID, &job.apiKeyID, &job.requestFingerprint, &job.payload, &job.attempts, &job.createdAt); err != nil {
			_ = rows.Close()
			return 0, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if len(jobs) == 0 {
		return 0, nil
	}

	completions, fastErr := r.applyJobBatchFast(ctx, tx, jobs)
	if fastErr != nil {
		_ = tx.Rollback()
		tx = nil
		// Isolate an invalid or concurrently deleted entity without degrading the
		// normal batch path. The next loop retries the remaining healthy jobs.
		return r.processSingleJob(parent)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	tx = nil
	for _, completion := range completions {
		r.completePendingOverlay(&completion.cmd)
	}
	return len(jobs), nil
}

func (r *queuedUsageBillingRepository) processSingleJob(parent context.Context) (_ int, err error) {
	ctx, cancel := context.WithTimeout(parent, r.commandTimeout)
	defer cancel()
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	rows, err := tx.QueryContext(ctx, `
		SELECT id, request_id, api_key_id, request_fingerprint, payload, attempts, created_at
		FROM usage_billing_jobs
		WHERE available_at <= NOW()
		ORDER BY available_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`)
	if err != nil {
		return 0, err
	}
	var job usageBillingJob
	if !rows.Next() {
		_ = rows.Close()
		return 0, nil
	}
	if err := rows.Scan(&job.id, &job.requestID, &job.apiKeyID, &job.requestFingerprint, &job.payload, &job.attempts, &job.createdAt); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	cmd, _, err := r.applyJobWithSavepoint(ctx, tx, job)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	tx = nil
	if cmd != nil {
		r.completePendingOverlay(cmd)
	}
	return 1, nil
}

func (r *queuedUsageBillingRepository) applyJobBatchFast(ctx context.Context, tx *sql.Tx, jobs []usageBillingJob) ([]usageBillingCompletion, error) {
	commands := make(map[int64]*service.UsageBillingCommand, len(jobs))
	jobsByID := make(map[int64]usageBillingJob, len(jobs))
	claimInputs := make([]usageBillingClaimInput, 0, len(jobs))
	completions := make([]usageBillingCompletion, 0, len(jobs))
	for _, job := range jobs {
		jobsByID[job.id] = job
		var cmd service.UsageBillingCommand
		if err := json.Unmarshal(job.payload, &cmd); err != nil {
			if deadErr := deadLetterUsageBillingJob(ctx, tx, job, fmt.Sprintf("%v: %v", errUsageBillingJobPayloadInvalid, err)); deadErr != nil {
				return nil, deadErr
			}
			continue
		}
		cmd.Normalize()
		if cmd.RequestID != job.requestID || cmd.APIKeyID != job.apiKeyID || cmd.RequestFingerprint != job.requestFingerprint {
			if deadErr := deadLetterUsageBillingJob(ctx, tx, job, errUsageBillingJobPayloadInvalid.Error()+": identity mismatch"); deadErr != nil {
				return nil, deadErr
			}
			continue
		}
		commands[job.id] = &cmd
		claimInputs = append(claimInputs, usageBillingClaimInput{
			JobID:              job.id,
			RequestID:          job.requestID,
			APIKeyID:           job.apiKeyID,
			RequestFingerprint: job.requestFingerprint,
		})
	}
	if len(claimInputs) == 0 {
		return completions, nil
	}
	payload, err := json.Marshal(claimInputs)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, usageBillingClaimBatchSQL, payload)
	if err != nil {
		return nil, err
	}
	claimStatus := make(map[int64]usageBillingEnqueueStatus, len(claimInputs))
	for rows.Next() {
		var jobID int64
		var status string
		if err := rows.Scan(&jobID, &status); err != nil {
			_ = rows.Close()
			return nil, err
		}
		claimStatus[jobID] = usageBillingEnqueueStatus(status)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	inserted := make([]*service.UsageBillingCommand, 0, len(commands))
	terminalIDs := make([]int64, 0, len(commands))
	for jobID, cmd := range commands {
		status, ok := claimStatus[jobID]
		if !ok {
			return nil, errors.New("durable usage billing claim result missing")
		}
		switch status {
		case usageBillingEnqueueInserted:
			inserted = append(inserted, cmd)
			terminalIDs = append(terminalIDs, jobID)
			completions = append(completions, usageBillingCompletion{cmd: *cmd})
		case usageBillingEnqueueApplied:
			terminalIDs = append(terminalIDs, jobID)
			completions = append(completions, usageBillingCompletion{cmd: *cmd})
		default:
			if err := deadLetterUsageBillingJob(ctx, tx, jobsByID[jobID], service.ErrUsageBillingRequestConflict.Error()); err != nil {
				return nil, err
			}
			completions = append(completions, usageBillingCompletion{cmd: *cmd})
		}
	}
	if err := applyAggregatedUsageBillingEffects(ctx, tx, inserted); err != nil {
		return nil, err
	}
	if len(terminalIDs) > 0 {
		idsJSON, err := json.Marshal(terminalIDs)
		if err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM usage_billing_jobs
			WHERE id IN (
				SELECT value::bigint FROM jsonb_array_elements_text($1::jsonb)
			)
		`, idsJSON); err != nil {
			return nil, err
		}
	}
	return completions, nil
}

func applyAggregatedUsageBillingEffects(ctx context.Context, tx *sql.Tx, commands []*service.UsageBillingCommand) error {
	balances := make(map[int64]float64)
	subscriptions := make(map[int64]float64)
	apiKeyQuotas := make(map[int64]float64)
	apiKeyRateLimits := make(map[int64]float64)
	accountQuotas := make(map[int64]float64)
	platformQuotas := make(map[string]*usageBillingPlatformQuotaAggregate)
	for _, cmd := range commands {
		if cmd == nil {
			continue
		}
		if cmd.BalanceCost > 0 {
			balances[cmd.UserID] += cmd.BalanceCost
		}
		if cmd.SubscriptionCost > 0 && cmd.SubscriptionID != nil {
			subscriptions[*cmd.SubscriptionID] += cmd.SubscriptionCost
		}
		if cmd.APIKeyQuotaCost > 0 {
			apiKeyQuotas[cmd.APIKeyID] += cmd.APIKeyQuotaCost
		}
		if cmd.APIKeyRateLimitCost > 0 {
			apiKeyRateLimits[cmd.APIKeyID] += cmd.APIKeyRateLimitCost
		}
		if cmd.AccountQuotaCost > 0 && (strings.EqualFold(cmd.AccountType, service.AccountTypeAPIKey) || strings.EqualFold(cmd.AccountType, service.AccountTypeBedrock)) {
			accountQuotas[cmd.AccountID] += cmd.AccountQuotaCost
		}
		if cmd.UserPlatformQuotaCost > 0 && strings.TrimSpace(cmd.QuotaPlatform) != "" {
			platform := strings.TrimSpace(cmd.QuotaPlatform)
			key := strconv.FormatInt(cmd.UserID, 10) + "\x00" + platform
			aggregate := platformQuotas[key]
			if aggregate == nil {
				aggregate = &usageBillingPlatformQuotaAggregate{userID: cmd.UserID, platform: platform}
				platformQuotas[key] = aggregate
			}
			aggregate.amount += cmd.UserPlatformQuotaCost
		}
	}
	for subscriptionID, amount := range subscriptions {
		if err := incrementUsageBillingSubscription(ctx, tx, subscriptionID, amount); err != nil {
			return err
		}
	}
	for userID, amount := range balances {
		if _, _, err := deductUsageBillingBalance(ctx, tx, userID, amount); err != nil {
			return err
		}
	}
	for apiKeyID, amount := range apiKeyQuotas {
		if _, err := incrementUsageBillingAPIKeyQuota(ctx, tx, apiKeyID, amount); err != nil {
			return err
		}
	}
	for apiKeyID, amount := range apiKeyRateLimits {
		if err := incrementUsageBillingAPIKeyRateLimit(ctx, tx, apiKeyID, amount); err != nil {
			return err
		}
	}
	for accountID, amount := range accountQuotas {
		if _, err := incrementUsageBillingAccountQuota(ctx, tx, accountID, amount); err != nil {
			return err
		}
	}
	for _, quota := range platformQuotas {
		if err := incrementUsageBillingUserPlatformQuota(ctx, tx, quota.userID, quota.platform, quota.amount); err != nil {
			return err
		}
	}
	return nil
}

func (r *queuedUsageBillingRepository) applyJobWithSavepoint(ctx context.Context, tx *sql.Tx, job usageBillingJob) (*service.UsageBillingCommand, bool, error) {
	if _, err := tx.ExecContext(ctx, "SAVEPOINT usage_billing_job"); err != nil {
		return nil, false, err
	}
	var cmd service.UsageBillingCommand
	err := json.Unmarshal(job.payload, &cmd)
	if err != nil {
		err = fmt.Errorf("%w: %v", errUsageBillingJobPayloadInvalid, err)
	}
	if err == nil {
		cmd.Normalize()
		if cmd.RequestID != job.requestID || cmd.APIKeyID != job.apiKeyID || cmd.RequestFingerprint != job.requestFingerprint {
			err = fmt.Errorf("%w: identity mismatch", errUsageBillingJobPayloadInvalid)
		}
	}
	if err == nil {
		var applied bool
		applied, err = r.direct.claimUsageBillingKey(ctx, tx, &cmd)
		if err == nil && applied {
			result := &service.UsageBillingApplyResult{Applied: true}
			err = r.direct.applyUsageBillingEffects(ctx, tx, &cmd, result)
		}
		if err == nil && !applied {
			if _, rollbackErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT usage_billing_job"); rollbackErr != nil {
				return nil, false, rollbackErr
			}
		}
		if err == nil {
			if _, deleteErr := tx.ExecContext(ctx, "DELETE FROM usage_billing_jobs WHERE id = $1", job.id); deleteErr != nil {
				return nil, false, deleteErr
			}
			if _, releaseErr := tx.ExecContext(ctx, "RELEASE SAVEPOINT usage_billing_job"); releaseErr != nil {
				return nil, false, releaseErr
			}
			return &cmd, applied, nil
		}
	}

	if _, rollbackErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT usage_billing_job"); rollbackErr != nil {
		return nil, false, rollbackErr
	}
	if isPermanentUsageBillingError(err) {
		if deadErr := deadLetterUsageBillingJob(ctx, tx, job, err.Error()); deadErr != nil {
			return nil, false, deadErr
		}
		if _, releaseErr := tx.ExecContext(ctx, "RELEASE SAVEPOINT usage_billing_job"); releaseErr != nil {
			return nil, false, releaseErr
		}
		if cmd.RequestID == "" {
			return nil, false, nil
		}
		return &cmd, false, nil
	}

	delay := usageBillingRetryDelay(job.attempts+1, r.maxRetryDelay)
	if _, updateErr := tx.ExecContext(ctx, `
		UPDATE usage_billing_jobs
		SET attempts = attempts + 1,
			last_error = $2,
			available_at = NOW() + ($3 * INTERVAL '1 millisecond'),
			updated_at = NOW()
		WHERE id = $1
	`, job.id, truncateUsageBillingError(err), delay.Milliseconds()); updateErr != nil {
		return nil, false, updateErr
	}
	if _, releaseErr := tx.ExecContext(ctx, "RELEASE SAVEPOINT usage_billing_job"); releaseErr != nil {
		return nil, false, releaseErr
	}
	return nil, false, nil
}

func deadLetterUsageBillingJob(ctx context.Context, tx *sql.Tx, job usageBillingJob, reason string) error {
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO usage_billing_dead_letters (
			source_job_id, request_id, api_key_id, request_fingerprint,
			payload, attempts, reason, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (request_id, api_key_id) DO UPDATE SET
			reason = EXCLUDED.reason,
			attempts = EXCLUDED.attempts,
			failed_at = NOW()
	`, job.id, job.requestID, job.apiKeyID, job.requestFingerprint, job.payload, job.attempts+1, truncateUsageBillingError(errors.New(reason)), job.createdAt); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, "DELETE FROM usage_billing_jobs WHERE id = $1", job.id)
	return err
}

func usageBillingRetryDelay(attempt int, maxDelay time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	shift := min(attempt-1, 10)
	delay := time.Second * time.Duration(1<<shift)
	if maxDelay > 0 && delay > maxDelay {
		return maxDelay
	}
	return delay
}

func truncateUsageBillingError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 2000 {
		message = message[:2000]
	}
	return message
}

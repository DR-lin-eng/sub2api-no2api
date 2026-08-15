package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/lib/pq"
)

func (r *queuedUsageBillingRepository) runConsumer(ctx context.Context, workerID int) {
	defer r.wg.Done()
	// One leader performs startup and cross-instance discovery. Other workers
	// stay event-driven until a local enqueue or discovered backlog wakes them.
	if workerID != 0 && !r.waitForConsumer(ctx, workerID) {
		return
	}
	for ctx.Err() == nil {
		if !r.consumerAllowed(workerID) {
			if !r.waitForConsumer(ctx, workerID) {
				return
			}
			continue
		}
		processed, err := r.processUsageBillingCycle(ctx, workerID, false)
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

func (r *queuedUsageBillingRepository) runRescue(ctx context.Context) {
	defer r.wg.Done()
	ticker := time.NewTicker(r.rescueScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !r.rescueAllowed() {
				continue
			}
			processed, err := r.processUsageBillingCycle(ctx, 0, true)
			if err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("durable usage billing rescue failed", "error", err)
			}
			if processed > 0 {
				r.wakeConsumers()
			}
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

func (r *queuedUsageBillingRepository) processUsageBillingCycle(parent context.Context, workerID int, rescue bool) (int, error) {
	beginner := usageBillingTxBeginner(r.db)
	release := func() {}
	if rescue {
		acquireCtx, cancel := context.WithTimeout(parent, r.commandTimeout)
		conn, unlock, acquired, err := r.tryAcquireUsageBillingRescueSlot(acquireCtx)
		cancel()
		if err != nil {
			return 0, err
		}
		if !acquired {
			return 0, nil
		}
		if conn != nil {
			beginner = conn
			release = unlock
		}
	}
	defer release()
	settled, settleErr := r.processUnsettledJobBatch(parent, workerID, rescue, beginner)
	cleaned, cleanupErr := r.processCleanupJobBatch(parent, workerID, rescue, beginner)
	// The return value is a liveness hint used by the worker loop. Keep it in
	// job units rather than double-counting a job settled and cleaned in one cycle.
	return max(settled, cleaned), errors.Join(settleErr, cleanupErr)
}

func (r *queuedUsageBillingRepository) processUnsettledJobBatch(parent context.Context, workerID int, rescue bool, beginner usageBillingTxBeginner) (processed int, err error) {
	started := time.Now()
	defer func() { r.recordUsageBillingBatch(started, processed, rescue, false, err) }()
	ctx, cancel := context.WithTimeout(parent, r.commandTimeout)
	defer cancel()
	if beginner == nil {
		beginner = r.db
	}
	tx, err := beginner.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	if !rescue {
		acquired, err := r.tryAcquireUsageBillingClusterSlot(ctx, tx)
		if err != nil {
			return 0, err
		}
		if !acquired {
			return 0, nil
		}
	}
	if r.runtime != nil {
		r.runtime.activeBatches.Add(1)
		defer r.runtime.activeBatches.Add(-1)
	}
	batchSize := r.readBatchSize
	if rescue {
		batchSize = r.rescueBatchSize
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id, request_id, api_key_id, request_fingerprint, payload, attempts, created_at
		FROM usage_billing_jobs
		WHERE settled_at IS NULL
			AND available_at <= NOW()
			AND (NOT $2::boolean OR created_at <= NOW() - ($3 * INTERVAL '1 millisecond'))
		ORDER BY available_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT $1
	`, batchSize, rescue, r.rescueStaleAfter.Milliseconds())
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
	if err := markUsageBillingJobsClaimed(ctx, tx, jobs, r.usageBillingInstanceID()); err != nil {
		return 0, err
	}

	settledCount, fastErr := r.applyJobBatchFast(ctx, tx, jobs)
	if fastErr != nil {
		_ = tx.Rollback()
		tx = nil
		// Isolate an invalid or concurrently deleted entity without degrading the
		// normal batch path. The next loop retries the remaining healthy jobs.
		return r.processSingleUnsettledJob(parent, workerID, rescue, beginner)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	tx = nil
	r.wakeConsumers()
	return settledCount, nil
}

func (r *queuedUsageBillingRepository) processSingleUnsettledJob(parent context.Context, workerID int, rescue bool, beginner usageBillingTxBeginner) (_ int, err error) {
	ctx, cancel := context.WithTimeout(parent, r.commandTimeout)
	defer cancel()
	if beginner == nil {
		beginner = r.db
	}
	tx, err := beginner.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	if !rescue {
		acquired, err := r.tryAcquireUsageBillingClusterSlot(ctx, tx)
		if err != nil {
			return 0, err
		}
		if !acquired {
			return 0, nil
		}
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, request_id, api_key_id, request_fingerprint, payload, attempts, created_at
		FROM usage_billing_jobs
		WHERE settled_at IS NULL
			AND available_at <= NOW()
			AND (NOT $1::boolean OR created_at <= NOW() - ($2 * INTERVAL '1 millisecond'))
		ORDER BY available_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT 1
	`, rescue, r.rescueStaleAfter.Milliseconds())
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
	if err := markUsageBillingJobsClaimed(ctx, tx, []usageBillingJob{job}, r.usageBillingInstanceID()); err != nil {
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
		r.wakeConsumers()
	}
	return 1, nil
}

type usageBillingCleanupFailure struct {
	jobID      int64
	errorClass string
	errorText  string
}

type usageBillingCleanupFailureInput struct {
	JobID      int64  `json:"job_id"`
	ErrorClass string `json:"error_class"`
	ErrorText  string `json:"error_text"`
}

func (r *queuedUsageBillingRepository) processCleanupJobBatch(parent context.Context, workerID int, rescue bool, beginner usageBillingTxBeginner) (processed int, err error) {
	started := time.Now()
	defer func() { r.recordUsageBillingBatch(started, processed, rescue, true, err) }()
	ctx, cancel := context.WithTimeout(parent, r.commandTimeout)
	defer cancel()
	if beginner == nil {
		beginner = r.db
	}
	tx, err := beginner.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()
	if !rescue {
		acquired, err := r.tryAcquireUsageBillingClusterSlot(ctx, tx)
		if err != nil {
			return 0, err
		}
		if !acquired {
			return 0, nil
		}
	}
	if r.runtime != nil {
		r.runtime.activeBatches.Add(1)
		defer r.runtime.activeBatches.Add(-1)
	}
	batchSize := max(1, r.cleanupBatchSize)
	if rescue {
		batchSize = max(1, r.rescueCleanupBatchSize)
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, request_id, api_key_id, request_fingerprint, payload, attempts, created_at
		FROM usage_billing_jobs
		WHERE settled_at IS NOT NULL
			AND available_at <= NOW()
			AND (NOT $2::boolean OR settled_at <= NOW() - ($3 * INTERVAL '1 millisecond'))
		ORDER BY available_at, id
		FOR UPDATE SKIP LOCKED
		LIMIT $1
	`, batchSize, rescue, r.rescueStaleAfter.Milliseconds())
	if err != nil {
		return 0, err
	}
	jobs := make([]usageBillingJob, 0, batchSize)
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
	ids := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.id)
	}
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		return 0, err
	}
	lease := max(r.commandTimeout*2, time.Minute)
	if _, err := tx.ExecContext(ctx, `
		UPDATE usage_billing_jobs
		SET available_at = NOW() + ($2 * INTERVAL '1 millisecond'),
			last_attempt_at = NOW(),
			last_claimed_by = $3,
			updated_at = NOW()
		WHERE id IN (
			SELECT value::bigint FROM jsonb_array_elements_text($1::jsonb)
		)
	`, idsJSON, lease.Milliseconds(), r.usageBillingInstanceID()); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	tx = nil

	completed := make([]int64, 0, len(jobs))
	failures := make([]usageBillingCleanupFailure, 0)
	errorsSeen := make([]error, 0)
	for _, job := range jobs {
		var cmd service.UsageBillingCommand
		if unmarshalErr := json.Unmarshal(job.payload, &cmd); unmarshalErr != nil {
			failure := fmt.Errorf("%w: %v", errUsageBillingJobPayloadInvalid, unmarshalErr)
			failures = append(failures, usageBillingCleanupFailure{jobID: job.id, errorClass: "cleanup_payload_invalid", errorText: truncateUsageBillingError(failure)})
			errorsSeen = append(errorsSeen, failure)
			continue
		}
		cmd.Normalize()
		if cmd.RequestID != job.requestID || cmd.APIKeyID != job.apiKeyID || cmd.RequestFingerprint != job.requestFingerprint {
			failure := fmt.Errorf("%w: identity mismatch", errUsageBillingJobPayloadInvalid)
			failures = append(failures, usageBillingCleanupFailure{jobID: job.id, errorClass: "cleanup_payload_invalid", errorText: truncateUsageBillingError(failure)})
			errorsSeen = append(errorsSeen, failure)
			continue
		}
		if cleanupErr := r.completePendingOverlayContext(ctx, &cmd); cleanupErr != nil {
			failures = append(failures, usageBillingCleanupFailure{jobID: job.id, errorClass: "redis_overlay_cleanup", errorText: truncateUsageBillingError(cleanupErr)})
			errorsSeen = append(errorsSeen, cleanupErr)
			continue
		}
		completed = append(completed, job.id)
	}
	if err := r.finalizeUsageBillingCleanup(ctx, beginner, completed, failures); err != nil {
		return 0, errors.Join(append(errorsSeen, err)...)
	}
	return len(completed), errors.Join(errorsSeen...)
}

func (r *queuedUsageBillingRepository) finalizeUsageBillingCleanup(
	ctx context.Context,
	beginner usageBillingTxBeginner,
	completed []int64,
	failures []usageBillingCleanupFailure,
) error {
	if len(completed) == 0 && len(failures) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if beginner == nil {
		beginner = r.db
	}
	tx, err := beginner.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if len(completed) > 0 {
		payload, err := json.Marshal(completed)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			DELETE FROM usage_billing_jobs
			WHERE settled_at IS NOT NULL
				AND id IN (
					SELECT value::bigint FROM jsonb_array_elements_text($1::jsonb)
				)
		`, payload); err != nil {
			return err
		}
	}
	if len(failures) > 0 {
		inputs := make([]usageBillingCleanupFailureInput, 0, len(failures))
		for _, failure := range failures {
			inputs = append(inputs, usageBillingCleanupFailureInput{
				JobID:      failure.jobID,
				ErrorClass: failure.errorClass,
				ErrorText:  failure.errorText,
			})
		}
		payload, err := json.Marshal(inputs)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			WITH failed AS (
				SELECT job_id, error_class, error_text
				FROM jsonb_to_recordset($1::jsonb) AS x(
					job_id bigint,
					error_class text,
					error_text text
				)
			)
			UPDATE usage_billing_jobs AS jobs
			SET cleanup_attempts = jobs.cleanup_attempts + 1,
				last_error = failed.error_text,
				last_error_class = failed.error_class,
				reconcile_required_at = CASE
					WHEN jobs.cleanup_attempts + 1 >= $2
					OR COALESCE(jobs.settled_at, jobs.created_at) <= NOW() - ($3 * INTERVAL '1 millisecond')
					THEN COALESCE(jobs.reconcile_required_at, NOW())
					ELSE jobs.reconcile_required_at
				END,
				available_at = NOW() + (
					CASE
						WHEN jobs.cleanup_attempts + 1 >= $2
							OR COALESCE(jobs.settled_at, jobs.created_at) <= NOW() - ($3 * INTERVAL '1 millisecond')
						THEN $4
						ELSE $5
					END * INTERVAL '1 millisecond'
				),
				updated_at = NOW()
			FROM failed
			WHERE jobs.id = failed.job_id
				AND jobs.settled_at IS NOT NULL
		`, payload, r.retryAlertThreshold(), r.oldestAgeThreshold().Milliseconds(), r.reconcileRetryInterval().Milliseconds(), max(time.Second, r.maxRetryDelay).Milliseconds()); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, failure := range failures {
		r.recordUsageBillingRetryClass(failure.errorClass, 1)
	}
	return nil
}

func markUsageBillingJobsClaimed(ctx context.Context, tx *sql.Tx, jobs []usageBillingJob, instanceID string) error {
	if len(jobs) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.id)
	}
	payload, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE usage_billing_jobs
		SET last_attempt_at = NOW(),
			last_claimed_by = $2,
			updated_at = NOW()
		WHERE id IN (
			SELECT value::bigint FROM jsonb_array_elements_text($1::jsonb)
		)
	`, payload, instanceID)
	return err
}

func (r *queuedUsageBillingRepository) applyJobBatchFast(ctx context.Context, tx *sql.Tx, jobs []usageBillingJob) (int, error) {
	commands := make(map[int64]*service.UsageBillingCommand, len(jobs))
	jobsByID := make(map[int64]usageBillingJob, len(jobs))
	claimInputs := make([]usageBillingClaimInput, 0, len(jobs))
	for _, job := range jobs {
		jobsByID[job.id] = job
		var cmd service.UsageBillingCommand
		if err := json.Unmarshal(job.payload, &cmd); err != nil {
			if deadErr := deadLetterUsageBillingJob(ctx, tx, job, fmt.Sprintf("%v: %v", errUsageBillingJobPayloadInvalid, err)); deadErr != nil {
				return 0, deadErr
			}
			continue
		}
		cmd.Normalize()
		if cmd.RequestID != job.requestID || cmd.APIKeyID != job.apiKeyID || cmd.RequestFingerprint != job.requestFingerprint {
			if deadErr := deadLetterUsageBillingJob(ctx, tx, job, errUsageBillingJobPayloadInvalid.Error()+": identity mismatch"); deadErr != nil {
				return 0, deadErr
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
		return 0, nil
	}
	payload, err := json.Marshal(claimInputs)
	if err != nil {
		return 0, err
	}
	rows, err := tx.QueryContext(ctx, usageBillingClaimBatchSQL, payload)
	if err != nil {
		return 0, err
	}
	claimStatus := make(map[int64]usageBillingEnqueueStatus, len(claimInputs))
	for rows.Next() {
		var jobID int64
		var status string
		if err := rows.Scan(&jobID, &status); err != nil {
			_ = rows.Close()
			return 0, err
		}
		claimStatus[jobID] = usageBillingEnqueueStatus(status)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}

	inserted := make([]*service.UsageBillingCommand, 0, len(commands))
	terminalIDs := make([]int64, 0, len(commands))
	for jobID, cmd := range commands {
		status, ok := claimStatus[jobID]
		if !ok {
			return 0, errors.New("durable usage billing claim result missing")
		}
		switch status {
		case usageBillingEnqueueInserted:
			inserted = append(inserted, cmd)
			terminalIDs = append(terminalIDs, jobID)
		case usageBillingEnqueueApplied:
			terminalIDs = append(terminalIDs, jobID)
		default:
			if err := deadLetterUsageBillingJob(ctx, tx, jobsByID[jobID], service.ErrUsageBillingRequestConflict.Error()); err != nil {
				return 0, err
			}
		}
	}
	if err := applyAggregatedUsageBillingEffects(ctx, tx, inserted); err != nil {
		return 0, err
	}
	if len(terminalIDs) > 0 {
		idsJSON, err := json.Marshal(terminalIDs)
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE usage_billing_jobs
			SET settled_at = COALESCE(settled_at, NOW()),
				available_at = NOW(),
				last_error = NULL,
				last_error_class = NULL,
				reconcile_required_at = NULL,
				updated_at = NOW()
			WHERE id IN (
				SELECT value::bigint FROM jsonb_array_elements_text($1::jsonb)
			)
		`, idsJSON); err != nil {
			return 0, err
		}
	}
	return len(terminalIDs), nil
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
	for _, subscriptionID := range sortedUsageBillingInt64Keys(subscriptions) {
		amount := subscriptions[subscriptionID]
		if err := incrementUsageBillingSubscription(ctx, tx, subscriptionID, amount); err != nil {
			return err
		}
	}
	for _, userID := range sortedUsageBillingInt64Keys(balances) {
		amount := balances[userID]
		if _, _, err := deductUsageBillingBalance(ctx, tx, userID, amount); err != nil {
			return err
		}
	}
	for _, apiKeyID := range sortedUsageBillingInt64Keys(apiKeyQuotas) {
		amount := apiKeyQuotas[apiKeyID]
		if _, err := incrementUsageBillingAPIKeyQuota(ctx, tx, apiKeyID, amount); err != nil {
			return err
		}
	}
	for _, apiKeyID := range sortedUsageBillingInt64Keys(apiKeyRateLimits) {
		amount := apiKeyRateLimits[apiKeyID]
		if err := incrementUsageBillingAPIKeyRateLimit(ctx, tx, apiKeyID, amount); err != nil {
			return err
		}
	}
	for _, accountID := range sortedUsageBillingInt64Keys(accountQuotas) {
		amount := accountQuotas[accountID]
		if _, err := incrementUsageBillingAccountQuota(ctx, tx, accountID, amount); err != nil {
			return err
		}
	}
	platformKeys := make([]string, 0, len(platformQuotas))
	for key := range platformQuotas {
		platformKeys = append(platformKeys, key)
	}
	sort.Strings(platformKeys)
	for _, key := range platformKeys {
		quota := platformQuotas[key]
		if err := incrementUsageBillingUserPlatformQuota(ctx, tx, quota.userID, quota.platform, quota.amount); err != nil {
			return err
		}
	}
	return nil
}

func sortedUsageBillingInt64Keys[T any](values map[int64]T) []int64 {
	keys := make([]int64, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	return keys
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
			if _, updateErr := tx.ExecContext(ctx, `
				UPDATE usage_billing_jobs
				SET settled_at = COALESCE(settled_at, NOW()),
					available_at = NOW(),
					last_error = NULL,
					last_error_class = NULL,
					reconcile_required_at = NULL,
					updated_at = NOW()
				WHERE id = $1
			`, job.id); updateErr != nil {
				return nil, false, updateErr
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
	errorClass := classifyUsageBillingError(err)
	if _, updateErr := tx.ExecContext(ctx, `
		UPDATE usage_billing_jobs
		SET attempts = attempts + 1,
			last_error = $2,
			last_error_class = $3,
			reconcile_required_at = CASE
				WHEN attempts + 1 >= $4
					OR created_at <= NOW() - ($5 * INTERVAL '1 millisecond')
				THEN COALESCE(reconcile_required_at, NOW())
				ELSE reconcile_required_at
			END,
			available_at = NOW() + (
				CASE
					WHEN attempts + 1 >= $4
						OR created_at <= NOW() - ($5 * INTERVAL '1 millisecond')
					THEN $6
					ELSE $7
				END * INTERVAL '1 millisecond'
			),
			updated_at = NOW()
		WHERE id = $1
	`, job.id, truncateUsageBillingError(err), errorClass, r.retryAlertThreshold(), r.oldestAgeThreshold().Milliseconds(), r.reconcileRetryInterval().Milliseconds(), delay.Milliseconds()); updateErr != nil {
		return nil, false, updateErr
	}
	r.recordUsageBillingRetryClass(errorClass, 1)
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
	// Keep the durable row in the settled cleanup channel. Permanent failures
	// must never be retried automatically, but any Redis overlay created by the
	// producer still needs the normal idempotent cleanup path.
	_, err := tx.ExecContext(ctx, `
		UPDATE usage_billing_jobs
		SET settled_at = COALESCE(settled_at, NOW()),
			available_at = NOW(),
			last_error = $2,
			last_error_class = 'permanent',
			reconcile_required_at = NULL,
			updated_at = NOW()
		WHERE id = $1
	`, job.id, truncateUsageBillingError(errors.New(reason)))
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

func classifyUsageBillingError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		switch pqErr.Code.Class() {
		case "08", "40", "53", "55", "57", "58":
			return "postgres_transient"
		case "23":
			return "postgres_constraint"
		default:
			return "postgres"
		}
	}
	if isPermanentUsageBillingError(err) {
		return "permanent"
	}
	return "transient"
}

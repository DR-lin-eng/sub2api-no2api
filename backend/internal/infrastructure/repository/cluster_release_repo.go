package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

type clusterReleaseRepository struct {
	db *sql.DB
}

func NewClusterReleaseRepository(db *sql.DB) service.ClusterReleaseRepository {
	return &clusterReleaseRepository{db: db}
}

func (r *clusterReleaseRepository) EnsureState(ctx context.Context, initialVersion string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO cluster_release_state (singleton, desired_version)
		VALUES (TRUE, $1)
		ON CONFLICT (singleton) DO UPDATE SET
			desired_version = CASE
				WHEN cluster_release_state.desired_version = '' THEN EXCLUDED.desired_version
				ELSE cluster_release_state.desired_version
			END,
			updated_at = CASE
				WHEN cluster_release_state.desired_version = '' THEN NOW()
				ELSE cluster_release_state.updated_at
			END
	`, initialVersion)
	return err
}

func (r *clusterReleaseRepository) GetState(ctx context.Context) (*service.ClusterReleaseState, error) {
	state := &service.ClusterReleaseState{}
	var active sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT desired_version, active_rollout_id, generation, updated_at
		FROM cluster_release_state
		WHERE singleton = TRUE
	`).Scan(&state.DesiredVersion, &active, &state.Generation, &state.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if active.Valid {
		state.ActiveRolloutID = active.String
	}
	return state, nil
}

func (r *clusterReleaseRepository) CreateRollout(ctx context.Context, rollout service.ClusterRollout, targets []service.ClusterRolloutTarget) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var active sql.NullString
	if err := tx.QueryRowContext(ctx, `
		SELECT active_rollout_id
		FROM cluster_release_state
		WHERE singleton = TRUE
		FOR UPDATE
	`).Scan(&active); err != nil {
		return err
	}
	if active.Valid && active.String != "" {
		return service.ErrClusterRolloutActive
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO cluster_release_rollouts (
			id, source_version, target_version, status, strategy,
			max_unavailable, created_by, started_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, rollout.ID, rollout.SourceVersion, rollout.TargetVersion, rollout.Status,
		rollout.Strategy, rollout.MaxUnavailable, rollout.CreatedBy, rollout.StartedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return service.ErrClusterRolloutActive
		}
		return err
	}

	for i := range targets {
		target := targets[i]
		_, err = tx.ExecContext(ctx, `
			INSERT INTO cluster_release_targets (
				rollout_id, node_id, node_name, ordinal, source_version, target_version,
				status, source_runner_id, completed_at
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		`, rollout.ID, target.NodeID, target.NodeName, target.Ordinal, target.SourceVersion,
			target.TargetVersion, target.Status, target.SourceRunnerID, target.CompletedAt)
		if err != nil {
			return err
		}
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE cluster_release_state
		SET active_rollout_id = $1,
			generation = generation + 1,
			updated_at = NOW()
		WHERE singleton = TRUE
	`, rollout.ID)
	if err != nil {
		return err
	}
	return tx.Commit()
}

type clusterReleaseRowScanner interface {
	Scan(dest ...any) error
}

func scanClusterRollout(scanner clusterReleaseRowScanner) (*service.ClusterRollout, error) {
	rollout := &service.ClusterRollout{}
	var completed sql.NullTime
	err := scanner.Scan(
		&rollout.ID,
		&rollout.SourceVersion,
		&rollout.TargetVersion,
		&rollout.Status,
		&rollout.Strategy,
		&rollout.MaxUnavailable,
		&rollout.CreatedBy,
		&rollout.ErrorMessage,
		&rollout.StartedAt,
		&completed,
		&rollout.CreatedAt,
		&rollout.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if completed.Valid {
		value := completed.Time
		rollout.CompletedAt = &value
	}
	return rollout, nil
}

const clusterRolloutSelect = `
	SELECT id, source_version, target_version, status, strategy,
		max_unavailable, created_by, error_message, started_at,
		completed_at, created_at, updated_at
	FROM cluster_release_rollouts
`

func (r *clusterReleaseRepository) GetRollout(ctx context.Context, rolloutID string) (*service.ClusterRollout, error) {
	rollout, err := scanClusterRollout(r.db.QueryRowContext(ctx, clusterRolloutSelect+` WHERE id = $1`, rolloutID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	targets, err := r.listTargets(ctx, rolloutID)
	if err != nil {
		return nil, err
	}
	rollout.Targets = targets
	return rollout, nil
}

func (r *clusterReleaseRepository) GetActiveRollout(ctx context.Context) (*service.ClusterRollout, error) {
	state, err := r.GetState(ctx)
	if err != nil || state == nil || state.ActiveRolloutID == "" {
		return nil, err
	}
	return r.GetRollout(ctx, state.ActiveRolloutID)
}

func (r *clusterReleaseRepository) ListRecentRollouts(ctx context.Context, limit int) ([]service.ClusterRollout, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id
		FROM cluster_release_rollouts
		ORDER BY created_at DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	result := make([]service.ClusterRollout, 0, len(ids))
	for _, id := range ids {
		rollout, err := r.GetRollout(ctx, id)
		if err != nil {
			return nil, err
		}
		if rollout != nil {
			result = append(result, *rollout)
		}
	}
	return result, nil
}

func (r *clusterReleaseRepository) listTargets(ctx context.Context, rolloutID string) ([]service.ClusterRolloutTarget, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT rollout_id, node_id, node_name, ordinal, source_version, target_version,
			status, attempt, lease_owner, lease_until, source_runner_id,
			observed_runner_id, verification_count, last_verified_heartbeat,
			error_message, started_at, completed_at, created_at, updated_at
		FROM cluster_release_targets
		WHERE rollout_id = $1
		ORDER BY ordinal ASC
	`, rolloutID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	targets := make([]service.ClusterRolloutTarget, 0)
	for rows.Next() {
		target, err := scanClusterRolloutTarget(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, *target)
	}
	return targets, rows.Err()
}

func scanClusterRolloutTarget(scanner clusterReleaseRowScanner) (*service.ClusterRolloutTarget, error) {
	target := &service.ClusterRolloutTarget{}
	var leaseUntil sql.NullTime
	var lastVerified sql.NullTime
	var started sql.NullTime
	var completed sql.NullTime
	err := scanner.Scan(
		&target.RolloutID,
		&target.NodeID,
		&target.NodeName,
		&target.Ordinal,
		&target.SourceVersion,
		&target.TargetVersion,
		&target.Status,
		&target.Attempt,
		&target.LeaseOwner,
		&leaseUntil,
		&target.SourceRunnerID,
		&target.ObservedRunnerID,
		&target.VerificationCount,
		&lastVerified,
		&target.ErrorMessage,
		&started,
		&completed,
		&target.CreatedAt,
		&target.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if leaseUntil.Valid {
		value := leaseUntil.Time
		target.LeaseUntil = &value
	}
	if lastVerified.Valid {
		value := lastVerified.Time
		target.LastVerifiedHeartbeat = &value
	}
	if started.Valid {
		value := started.Time
		target.StartedAt = &value
	}
	if completed.Valid {
		value := completed.Time
		target.CompletedAt = &value
	}
	return target, nil
}

func (r *clusterReleaseRepository) GetTargetForNode(ctx context.Context, rolloutID, nodeID string) (*service.ClusterRolloutTarget, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT rollout_id, node_id, node_name, ordinal, source_version, target_version,
			status, attempt, lease_owner, lease_until, source_runner_id,
			observed_runner_id, verification_count, last_verified_heartbeat,
			error_message, started_at, completed_at, created_at, updated_at
		FROM cluster_release_targets
		WHERE rollout_id = $1 AND node_id = $2
	`, rolloutID, nodeID)
	target, err := scanClusterRolloutTarget(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return target, err
}

func (r *clusterReleaseRepository) GetRunnerInstance(ctx context.Context, runnerID string) (*service.ClusterInstance, error) {
	instance := &service.ClusterInstance{}
	var stoppedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT instance.node_id, instance.runner_id, node.display_name, instance.deployment_mode, instance.worker_mode, instance.worker_enabled,
			instance.version, instance.hostname, instance.process_id, instance.database_ok, instance.redis_ok,
			instance.started_at, instance.last_seen_at, instance.stopped_at
		FROM cluster_instances AS instance
		JOIN cluster_nodes AS node ON node.node_id = instance.node_id
		WHERE instance.runner_id = $1
	`, runnerID).Scan(
		&instance.NodeID,
		&instance.RunnerID,
		&instance.NodeName,
		&instance.DeploymentMode,
		&instance.WorkerMode,
		&instance.WorkerEnabled,
		&instance.Version,
		&instance.Hostname,
		&instance.ProcessID,
		&instance.DatabaseOK,
		&instance.RedisOK,
		&instance.StartedAt,
		&instance.LastSeenAt,
		&stoppedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if stoppedAt.Valid {
		value := stoppedAt.Time
		instance.StoppedAt = &value
	}
	return instance, nil
}

func (r *clusterReleaseRepository) ClaimTarget(ctx context.Context, rolloutID, nodeID, runnerID string, leaseUntil time.Time) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	var rolloutStatus string
	err = tx.QueryRowContext(ctx, `
		SELECT status
		FROM cluster_release_rollouts
		WHERE id = $1
		FOR UPDATE
	`, rolloutID).Scan(&rolloutStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return false, service.ErrClusterRolloutNotFound
	}
	if err != nil {
		return false, err
	}
	if rolloutStatus != service.ClusterRolloutStatusRunning {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE cluster_release_targets AS target
		SET status = $4,
			attempt = attempt + 1,
			lease_owner = $3,
			lease_until = $5,
			error_message = '',
			started_at = COALESCE(target.started_at, NOW()),
			updated_at = NOW()
		WHERE target.rollout_id = $1
			AND target.node_id = $2
			AND target.status = $6
			AND NOT EXISTS (
				SELECT 1 FROM cluster_release_targets active
				WHERE active.rollout_id = target.rollout_id
					AND active.status IN ($4, $7, $8, $9)
			)
			AND NOT EXISTS (
				SELECT 1 FROM cluster_release_targets previous
				WHERE previous.rollout_id = target.rollout_id
					AND previous.ordinal < target.ordinal
					AND previous.status <> $10
			)
	`, rolloutID, nodeID, runnerID, service.ClusterRolloutTargetDraining, leaseUntil,
		service.ClusterRolloutTargetPending,
		service.ClusterRolloutTargetInstalling, service.ClusterRolloutTargetRestarting,
		service.ClusterRolloutTargetVerifying, service.ClusterRolloutTargetSucceeded)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (r *clusterReleaseRepository) SetTargetStatus(ctx context.Context, rolloutID, nodeID, runnerID, status, errorMessage string, leaseUntil *time.Time) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE cluster_release_targets
		SET status = $4,
			error_message = $5,
			lease_until = $6,
			lease_owner = CASE WHEN $3 = '' THEN lease_owner ELSE $3 END,
			updated_at = NOW()
		WHERE rollout_id = $1
			AND node_id = $2
			AND ($3 = '' OR lease_owner = '' OR lease_owner = $3)
	`, rolloutID, nodeID, runnerID, status, errorMessage, leaseUntil)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return service.ErrClusterRolloutTargetNotFound
	}
	if status == service.ClusterRolloutTargetFailed {
		_, err = tx.ExecContext(ctx, `
			UPDATE cluster_release_rollouts
			SET status = $2, error_message = $3, updated_at = NOW()
			WHERE id = $1 AND status IN ($4, $2)
		`, rolloutID, service.ClusterRolloutStatusPaused, errorMessage, service.ClusterRolloutStatusRunning)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *clusterReleaseRepository) ObserveTargetHeartbeat(
	ctx context.Context,
	rolloutID, nodeID, runnerID, version string,
	heartbeatAt time.Time,
	required int,
	leaseUntil time.Time,
) (*service.ClusterRolloutTarget, bool, error) {
	if required < 1 {
		required = 1
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()

	var targetVersion string
	var ordinal int
	var status string
	var rolloutStatus string
	var lastVerified sql.NullTime
	var count int
	err = tx.QueryRowContext(ctx, `
		SELECT target.target_version, target.ordinal, target.status,
			target.last_verified_heartbeat, target.verification_count, rollout.status
		FROM cluster_release_targets AS target
		JOIN cluster_release_rollouts AS rollout ON rollout.id = target.rollout_id
		WHERE target.rollout_id = $1 AND target.node_id = $2
		FOR UPDATE
	`, rolloutID, nodeID).Scan(&targetVersion, &ordinal, &status, &lastVerified, &count, &rolloutStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, service.ErrClusterRolloutTargetNotFound
	}
	if err != nil {
		return nil, false, err
	}
	if version != targetVersion {
		return nil, false, fmt.Errorf("node version %s does not match rollout target %s", version, targetVersion)
	}
	if status == service.ClusterRolloutTargetSucceeded {
		target, getErr := r.getTargetWithTx(ctx, tx, rolloutID, nodeID)
		return target, false, getErr
	}
	if status == service.ClusterRolloutTargetFailed || status == service.ClusterRolloutTargetCancelled {
		return nil, false, service.ErrClusterRolloutInvalidState
	}
	if status == service.ClusterRolloutTargetPending {
		if rolloutStatus != service.ClusterRolloutStatusRunning {
			return nil, false, service.ErrClusterRolloutInvalidState
		}
		var blocked bool
		err = tx.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM cluster_release_targets previous
				WHERE previous.rollout_id = $1
					AND previous.ordinal < $2
					AND previous.status <> $3
			) OR EXISTS (
				SELECT 1 FROM cluster_release_targets active
				WHERE active.rollout_id = $1
					AND active.node_id <> $4
					AND active.status IN ($5,$6,$7,$8)
			)
		`, rolloutID, ordinal, service.ClusterRolloutTargetSucceeded, nodeID,
			service.ClusterRolloutTargetDraining, service.ClusterRolloutTargetInstalling,
			service.ClusterRolloutTargetRestarting, service.ClusterRolloutTargetVerifying).Scan(&blocked)
		if err != nil {
			return nil, false, err
		}
		if blocked {
			return nil, false, service.ErrClusterRolloutInvalidState
		}
	}

	if !lastVerified.Valid || heartbeatAt.After(lastVerified.Time) {
		count++
		lastVerified = sql.NullTime{Time: heartbeatAt, Valid: true}
	}
	completed := count >= required
	nextStatus := service.ClusterRolloutTargetVerifying
	if completed {
		nextStatus = service.ClusterRolloutTargetSucceeded
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE cluster_release_targets
		SET status = $3::VARCHAR(24),
			lease_owner = $4,
			lease_until = CASE WHEN $3::VARCHAR(24) = $5::VARCHAR(24) THEN NULL ELSE $6::TIMESTAMPTZ END,
			observed_runner_id = $4,
			verification_count = $7,
			last_verified_heartbeat = $8,
			error_message = '',
			completed_at = CASE WHEN $3::VARCHAR(24) = $5::VARCHAR(24) THEN NOW() ELSE completed_at END,
			updated_at = NOW()
		WHERE rollout_id = $1 AND node_id = $2
	`, rolloutID, nodeID, nextStatus, runnerID, service.ClusterRolloutTargetSucceeded,
		leaseUntil, count, lastVerified.Time)
	if err != nil {
		return nil, false, err
	}

	rolloutCompleted := false
	if completed {
		var remaining int
		if err := tx.QueryRowContext(ctx, `
			SELECT COUNT(*)
			FROM cluster_release_targets
			WHERE rollout_id = $1 AND status <> $2
		`, rolloutID, service.ClusterRolloutTargetSucceeded).Scan(&remaining); err != nil {
			return nil, false, err
		}
		if remaining == 0 {
			rolloutCompleted = true
			if _, err := tx.ExecContext(ctx, `
				UPDATE cluster_release_rollouts
				SET status = $2, completed_at = NOW(), error_message = '', updated_at = NOW()
				WHERE id = $1
			`, rolloutID, service.ClusterRolloutStatusCompleted); err != nil {
				return nil, false, err
			}
			if _, err := tx.ExecContext(ctx, `
				UPDATE cluster_release_state
				SET desired_version = $2,
					active_rollout_id = NULL,
					generation = generation + 1,
					updated_at = NOW()
				WHERE singleton = TRUE AND active_rollout_id = $1
			`, rolloutID, targetVersion); err != nil {
				return nil, false, err
			}
		}
	}

	target, err := r.getTargetWithTx(ctx, tx, rolloutID, nodeID)
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return target, rolloutCompleted, nil
}

func (r *clusterReleaseRepository) getTargetWithTx(ctx context.Context, tx *sql.Tx, rolloutID, nodeID string) (*service.ClusterRolloutTarget, error) {
	return scanClusterRolloutTarget(tx.QueryRowContext(ctx, `
		SELECT rollout_id, node_id, node_name, ordinal, source_version, target_version,
			status, attempt, lease_owner, lease_until, source_runner_id,
			observed_runner_id, verification_count, last_verified_heartbeat,
			error_message, started_at, completed_at, created_at, updated_at
		FROM cluster_release_targets
		WHERE rollout_id = $1 AND node_id = $2
	`, rolloutID, nodeID))
}

func (r *clusterReleaseRepository) FailExpiredTargets(ctx context.Context, now time.Time) (int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, `
		UPDATE cluster_release_targets
		SET status = $2,
			error_message = 'node rollout lease expired',
			lease_until = NULL,
			updated_at = NOW()
		WHERE status IN ($3,$4,$5,$6)
			AND lease_until IS NOT NULL
			AND lease_until < $1
		RETURNING rollout_id
	`, now, service.ClusterRolloutTargetFailed, service.ClusterRolloutTargetDraining,
		service.ClusterRolloutTargetInstalling, service.ClusterRolloutTargetRestarting,
		service.ClusterRolloutTargetVerifying)
	if err != nil {
		return 0, err
	}
	rolloutIDs := make(map[string]struct{})
	var count int64
	for rows.Next() {
		var rolloutID string
		if err := rows.Scan(&rolloutID); err != nil {
			_ = rows.Close()
			return 0, err
		}
		rolloutIDs[rolloutID] = struct{}{}
		count++
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for rolloutID := range rolloutIDs {
		if _, err := tx.ExecContext(ctx, `
			UPDATE cluster_release_rollouts
			SET status = $2,
				error_message = 'node rollout lease expired',
				updated_at = NOW()
			WHERE id = $1 AND status = $3
		`, rolloutID, service.ClusterRolloutStatusPaused, service.ClusterRolloutStatusRunning); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

func (r *clusterReleaseRepository) PauseRollout(ctx context.Context, rolloutID, reason string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE cluster_release_rollouts
		SET status = $2, error_message = $3, updated_at = NOW()
		WHERE id = $1 AND status = $4
	`, rolloutID, service.ClusterRolloutStatusPaused, reason, service.ClusterRolloutStatusRunning)
	return clusterRolloutMutationResult(result, err)
}

func (r *clusterReleaseRepository) ResumeRollout(ctx context.Context, rolloutID string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE cluster_release_rollouts AS rollout
		SET status = $2, error_message = '', updated_at = NOW()
		WHERE rollout.id = $1
			AND rollout.status = $3
			AND NOT EXISTS (
				SELECT 1 FROM cluster_release_targets target
				WHERE target.rollout_id = rollout.id AND target.status = $4
			)
	`, rolloutID, service.ClusterRolloutStatusRunning, service.ClusterRolloutStatusPaused,
		service.ClusterRolloutTargetFailed)
	return clusterRolloutMutationResult(result, err)
}

func (r *clusterReleaseRepository) CancelRollout(ctx context.Context, rolloutID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var rolloutStatus string
	err = tx.QueryRowContext(ctx, `
		SELECT status
		FROM cluster_release_rollouts
		WHERE id = $1
		FOR UPDATE
	`, rolloutID).Scan(&rolloutStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return service.ErrClusterRolloutNotFound
	}
	if err != nil {
		return err
	}
	if rolloutStatus != service.ClusterRolloutStatusRunning && rolloutStatus != service.ClusterRolloutStatusPaused {
		return service.ErrClusterRolloutInvalidState
	}
	var active int
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM cluster_release_targets
		WHERE rollout_id = $1 AND status IN ($2,$3,$4,$5)
	`, rolloutID, service.ClusterRolloutTargetDraining, service.ClusterRolloutTargetInstalling,
		service.ClusterRolloutTargetRestarting, service.ClusterRolloutTargetVerifying).Scan(&active); err != nil {
		return err
	}
	if active > 0 {
		return service.ErrClusterRolloutActiveTarget
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE cluster_release_rollouts
		SET status = $2, completed_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND status IN ($3,$4)
	`, rolloutID, service.ClusterRolloutStatusCancelled, service.ClusterRolloutStatusRunning,
		service.ClusterRolloutStatusPaused)
	if err := clusterRolloutMutationResult(result, err); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE cluster_release_targets
		SET status = $2, completed_at = NOW(), updated_at = NOW()
		WHERE rollout_id = $1 AND status IN ($3,$4)
	`, rolloutID, service.ClusterRolloutTargetCancelled, service.ClusterRolloutTargetPending,
		service.ClusterRolloutTargetFailed); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE cluster_release_state
		SET active_rollout_id = NULL, generation = generation + 1, updated_at = NOW()
		WHERE singleton = TRUE AND active_rollout_id = $1
	`, rolloutID); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *clusterReleaseRepository) RetryTarget(ctx context.Context, rolloutID, nodeID string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `
		UPDATE cluster_release_targets
		SET status = $3,
			lease_owner = '',
			lease_until = NULL,
			observed_runner_id = '',
			verification_count = 0,
			last_verified_heartbeat = NULL,
			error_message = '',
			started_at = NULL,
			completed_at = NULL,
			updated_at = NOW()
		WHERE rollout_id = $1 AND node_id = $2 AND status = $4
	`, rolloutID, nodeID, service.ClusterRolloutTargetPending, service.ClusterRolloutTargetFailed)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return service.ErrClusterRolloutTargetNotFound
	}
	result, err = tx.ExecContext(ctx, `
		UPDATE cluster_release_rollouts
		SET status = $2, error_message = '', updated_at = NOW()
		WHERE id = $1 AND status = $3
	`, rolloutID, service.ClusterRolloutStatusRunning, service.ClusterRolloutStatusPaused)
	if err := clusterRolloutMutationResult(result, err); err != nil {
		return err
	}
	return tx.Commit()
}

func clusterRolloutMutationResult(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return service.ErrClusterRolloutInvalidState
	}
	return nil
}

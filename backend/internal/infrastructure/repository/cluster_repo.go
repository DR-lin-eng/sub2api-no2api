package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

type clusterRepository struct {
	db *sql.DB
}

func NewClusterRepository(db *sql.DB) service.ClusterRepository {
	return &clusterRepository{db: db}
}

func (r *clusterRepository) UpsertInstance(ctx context.Context, instance service.ClusterInstance) error {
	if r == nil || r.db == nil {
		return errors.New("cluster repository database is unavailable")
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO cluster_instances (
			runner_id, node_name, deployment_mode, worker_mode, worker_enabled,
			version, hostname, process_id, database_ok, redis_ok,
			started_at, last_seen_at, stopped_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NULL,NOW())
		ON CONFLICT (runner_id) DO UPDATE SET
			node_name = EXCLUDED.node_name,
			deployment_mode = EXCLUDED.deployment_mode,
			worker_mode = EXCLUDED.worker_mode,
			worker_enabled = EXCLUDED.worker_enabled,
			version = EXCLUDED.version,
			hostname = EXCLUDED.hostname,
			process_id = EXCLUDED.process_id,
			database_ok = EXCLUDED.database_ok,
			redis_ok = EXCLUDED.redis_ok,
			last_seen_at = EXCLUDED.last_seen_at,
			stopped_at = NULL,
			updated_at = NOW()
	`, instance.RunnerID, instance.NodeName, instance.DeploymentMode, instance.WorkerMode,
		instance.WorkerEnabled, instance.Version, instance.Hostname, instance.ProcessID,
		instance.DatabaseOK, instance.RedisOK, instance.StartedAt, instance.LastSeenAt)
	return err
}

func (r *clusterRepository) MarkInstanceStopped(ctx context.Context, runnerID string, stoppedAt time.Time) error {
	if r == nil || r.db == nil {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE cluster_instances
		SET stopped_at = $2, last_seen_at = $2, updated_at = NOW()
		WHERE runner_id = $1
	`, runnerID, stoppedAt)
	return err
}

func (r *clusterRepository) ListInstances(ctx context.Context) ([]service.ClusterInstance, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT runner_id, node_name, deployment_mode, worker_mode, worker_enabled,
			version, hostname, process_id, database_ok, redis_ok,
			started_at, last_seen_at, stopped_at
		FROM cluster_instances
		ORDER BY last_seen_at DESC, node_name ASC
		LIMIT 200
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	instances := make([]service.ClusterInstance, 0)
	for rows.Next() {
		var instance service.ClusterInstance
		var stoppedAt sql.NullTime
		if err := rows.Scan(
			&instance.RunnerID, &instance.NodeName, &instance.DeploymentMode,
			&instance.WorkerMode, &instance.WorkerEnabled, &instance.Version,
			&instance.Hostname, &instance.ProcessID, &instance.DatabaseOK,
			&instance.RedisOK, &instance.StartedAt, &instance.LastSeenAt, &stoppedAt,
		); err != nil {
			return nil, err
		}
		if stoppedAt.Valid {
			value := stoppedAt.Time
			instance.StoppedAt = &value
		}
		instances = append(instances, instance)
	}
	return instances, rows.Err()
}

func (r *clusterRepository) ExpireStaleTasks(ctx context.Context, now time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE cluster_task_runs
		SET status = $1,
			active_key = NULL,
			error_message = CASE WHEN error_message = '' THEN 'task lease expired' ELSE error_message END,
			finished_at = COALESCE(finished_at, $2),
			updated_at = NOW()
		WHERE status = $3 AND lease_until < $2
	`, service.ClusterTaskStatusLost, now, service.ClusterTaskStatusRunning)
	return err
}

func (r *clusterRepository) PruneRuntime(ctx context.Context, stoppedBefore, taskBefore time.Time, maxTaskHistory int) error {
	if maxTaskHistory <= 0 {
		maxTaskHistory = 10_000
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var acquired bool
	if err := tx.QueryRowContext(ctx, `
		SELECT pg_try_advisory_xact_lock(hashtextextended('sub2api:cluster-runtime-prune', 0))
	`).Scan(&acquired); err != nil {
		return err
	}
	if !acquired {
		return tx.Commit()
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM cluster_instances
		WHERE stopped_at IS NOT NULL AND stopped_at < $1
	`, stoppedBefore); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM cluster_task_runs
		WHERE status <> $1 AND finished_at IS NOT NULL AND finished_at < $2
	`, service.ClusterTaskStatusRunning, taskBefore); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM cluster_task_runs
		WHERE id IN (
			SELECT id FROM cluster_task_runs
			WHERE status <> $1
			ORDER BY started_at DESC, id DESC
			OFFSET $2
		)
	`, service.ClusterTaskStatusRunning, maxTaskHistory); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *clusterRepository) TryAcquireTask(ctx context.Context, task service.ClusterTaskRun) (bool, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	// Serialize claims by logical task key without holding a session-level lock.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, task.TaskKey); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE cluster_task_runs
		SET status = $1, active_key = NULL, error_message = 'task lease expired',
			finished_at = NOW(), updated_at = NOW()
		WHERE task_key = $2 AND status = $3 AND lease_until < NOW()
	`, service.ClusterTaskStatusLost, task.TaskKey, service.ClusterTaskStatusRunning); err != nil {
		return false, err
	}
	var active bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM cluster_task_runs
			WHERE task_key = $1 AND status = $2 AND lease_until >= NOW()
		)
	`, task.TaskKey, service.ClusterTaskStatusRunning).Scan(&active); err != nil {
		return false, err
	}
	if active {
		if err := tx.Commit(); err != nil {
			return false, err
		}
		return false, nil
	}
	metadata, err := marshalClusterJSON(task.Metadata)
	if err != nil {
		return false, err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO cluster_task_runs (
			run_id, task_key, active_key, status, node_name, runner_id,
			metadata, result, started_at, heartbeat_at, lease_until, updated_at
		) VALUES ($1,$2,$2,$3,$4,$5,$6,'{}'::jsonb,$7,$8,$9,NOW())
	`, task.RunID, task.TaskKey, service.ClusterTaskStatusRunning, task.NodeName,
		task.RunnerID, metadata, task.StartedAt, task.HeartbeatAt, task.LeaseUntil)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *clusterRepository) RenewTaskLease(ctx context.Context, runID, runnerID string, leaseUntil time.Time) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE cluster_task_runs
		SET heartbeat_at = NOW(), lease_until = $3, updated_at = NOW()
		WHERE run_id = $1 AND runner_id = $2 AND status = $4 AND lease_until >= NOW()
	`, runID, runnerID, leaseUntil, service.ClusterTaskStatusRunning)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return service.ErrClusterTaskLeaseLost
	}
	return nil
}

func (r *clusterRepository) FinishTask(ctx context.Context, runID, runnerID, status string, resultPayload map[string]any, errorMessage string) error {
	resultJSON, err := marshalClusterJSON(resultPayload)
	if err != nil {
		return err
	}
	result, err := r.db.ExecContext(ctx, `
		UPDATE cluster_task_runs
		SET status = $3, active_key = NULL, result = $4, error_message = $5,
			finished_at = NOW(), heartbeat_at = NOW(), updated_at = NOW()
		WHERE run_id = $1 AND runner_id = $2 AND status = $6 AND lease_until >= NOW()
	`, runID, runnerID, status, resultJSON, errorMessage, service.ClusterTaskStatusRunning)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return service.ErrClusterTaskLeaseLost
	}
	return nil
}

func (r *clusterRepository) ListTaskRuns(ctx context.Context, limit int) ([]service.ClusterTaskRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, run_id, task_key, status, node_name, runner_id,
			metadata, result, error_message, started_at, heartbeat_at,
			lease_until, finished_at
		FROM cluster_task_runs
		ORDER BY started_at DESC, id DESC
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	tasks := make([]service.ClusterTaskRun, 0)
	for rows.Next() {
		var task service.ClusterTaskRun
		var metadataJSON, resultJSON []byte
		var finishedAt sql.NullTime
		if err := rows.Scan(
			&task.ID, &task.RunID, &task.TaskKey, &task.Status, &task.NodeName,
			&task.RunnerID, &metadataJSON, &resultJSON, &task.ErrorMessage,
			&task.StartedAt, &task.HeartbeatAt, &task.LeaseUntil, &finishedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metadataJSON, &task.Metadata); err != nil {
			return nil, fmt.Errorf("decode cluster task metadata: %w", err)
		}
		if err := json.Unmarshal(resultJSON, &task.Result); err != nil {
			return nil, fmt.Errorf("decode cluster task result: %w", err)
		}
		if finishedAt.Valid {
			value := finishedAt.Time
			task.FinishedAt = &value
		}
		tasks = append(tasks, task)
	}
	return tasks, rows.Err()
}

func marshalClusterJSON(value map[string]any) ([]byte, error) {
	if value == nil {
		value = map[string]any{}
	}
	return json.Marshal(value)
}

var _ service.ClusterRepository = (*clusterRepository)(nil)

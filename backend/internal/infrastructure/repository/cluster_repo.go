package repository

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

type clusterRepository struct {
	db *sql.DB
}

func NewClusterRepository(db *sql.DB) service.ClusterRepository {
	return &clusterRepository{db: db}
}

func initialClusterNodeDisplayName(configuredName, nodeID string) string {
	digest := sha256.Sum256([]byte(nodeID))
	suffix := fmt.Sprintf("-%x", digest[:4])
	runes := []rune(strings.TrimSpace(configuredName))
	if limit := 128 - len(suffix); len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes) + suffix
}

func touchClusterNode(ctx context.Context, tx *sql.Tx, instance service.ClusterInstance) (bool, error) {
	result, err := tx.ExecContext(ctx, `
		UPDATE cluster_nodes
		SET configured_name = $2,
			last_seen_at = $3,
			updated_at = NOW()
		WHERE node_id = $1
	`, instance.NodeID, instance.NodeName, instance.LastSeenAt)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func insertClusterNode(ctx context.Context, tx *sql.Tx, instance service.ClusterInstance, displayName string) (bool, error) {
	result, err := tx.ExecContext(ctx, `
		INSERT INTO cluster_nodes (node_id, display_name, configured_name, last_seen_at)
		VALUES ($1,$2,$3,$4)
		ON CONFLICT DO NOTHING
	`, instance.NodeID, displayName, instance.NodeName, instance.LastSeenAt)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

func ensureClusterNode(ctx context.Context, tx *sql.Tx, instance service.ClusterInstance) error {
	if touched, err := touchClusterNode(ctx, tx, instance); err != nil || touched {
		return err
	}
	if inserted, err := insertClusterNode(ctx, tx, instance, instance.NodeName); err != nil || inserted {
		return err
	}
	// A concurrent heartbeat may have inserted this node while the first insert
	// lost a display-name race.
	if touched, err := touchClusterNode(ctx, tx, instance); err != nil || touched {
		return err
	}
	fallback := initialClusterNodeDisplayName(instance.NodeName, instance.NodeID)
	if inserted, err := insertClusterNode(ctx, tx, instance, fallback); err != nil || inserted {
		return err
	}
	if touched, err := touchClusterNode(ctx, tx, instance); err != nil || touched {
		return err
	}
	return service.ErrClusterNodeNameConflict
}

func (r *clusterRepository) UpsertInstance(ctx context.Context, instance service.ClusterInstance) error {
	if r == nil || r.db == nil {
		return errors.New("cluster repository database is unavailable")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// The first heartbeat after migration adopts history that used the old
	// configured node name as identity. The FK cascades the durable node_id into
	// historical runner rows, so upgrades stop creating visible logical nodes.
	_, err = tx.ExecContext(ctx, `
		UPDATE cluster_nodes
		SET node_id = $1,
			configured_name = $2,
			last_seen_at = $3,
			updated_at = NOW()
		WHERE node_id LIKE 'legacy-%'
			AND configured_name = $2
			AND NOT EXISTS (SELECT 1 FROM cluster_nodes WHERE node_id = $1)
	`, instance.NodeID, instance.NodeName, instance.LastSeenAt)
	if err != nil {
		return err
	}
	if err := ensureClusterNode(ctx, tx, instance); err != nil {
		return err
	}
	var (
		cpuUsagePercent        *float64
		memoryUsedBytes        *int64
		memoryLimitBytes       *int64
		memoryUsagePercent     *float64
		inFlightRequests       *int64
		goroutineCount         *int
		dbConnectionsActive    *int
		dbConnectionsIdle      *int
		dbConnectionsMax       *int
		redisConnectionsActive *int
		redisConnectionsIdle   *int
		redisConnectionsMax    *int
		metricsCollectedAt     *time.Time
	)
	if load := instance.Load; load != nil {
		cpuUsagePercent = load.CPUUsagePercent
		memoryUsedBytes = load.MemoryUsedBytes
		memoryLimitBytes = load.MemoryLimitBytes
		memoryUsagePercent = load.MemoryUsagePercent
		inFlightRequests = &load.InFlightRequests
		goroutineCount = &load.GoroutineCount
		dbConnectionsActive = &load.DBConnectionsActive
		dbConnectionsIdle = &load.DBConnectionsIdle
		dbConnectionsMax = &load.DBConnectionsMax
		redisConnectionsActive = &load.RedisConnectionsActive
		redisConnectionsIdle = &load.RedisConnectionsIdle
		redisConnectionsMax = &load.RedisConnectionsMax
		metricsCollectedAt = &load.CollectedAt
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO cluster_instances (
			runner_id, node_id, node_name, deployment_mode, worker_mode, worker_enabled,
			version, hostname, process_id, database_ok, redis_ok,
			started_at, last_seen_at, stopped_at,
			cpu_usage_percent, memory_used_bytes, memory_limit_bytes, memory_usage_percent,
			in_flight_requests, goroutine_count,
			db_connections_active, db_connections_idle, db_connections_max,
			redis_connections_active, redis_connections_idle, redis_connections_max,
			metrics_collected_at, updated_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,NULL,
			$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,NOW()
		)
		ON CONFLICT (runner_id) DO UPDATE SET
			node_id = EXCLUDED.node_id,
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
			cpu_usage_percent = EXCLUDED.cpu_usage_percent,
			memory_used_bytes = EXCLUDED.memory_used_bytes,
			memory_limit_bytes = EXCLUDED.memory_limit_bytes,
			memory_usage_percent = EXCLUDED.memory_usage_percent,
			in_flight_requests = EXCLUDED.in_flight_requests,
			goroutine_count = EXCLUDED.goroutine_count,
			db_connections_active = EXCLUDED.db_connections_active,
			db_connections_idle = EXCLUDED.db_connections_idle,
			db_connections_max = EXCLUDED.db_connections_max,
			redis_connections_active = EXCLUDED.redis_connections_active,
			redis_connections_idle = EXCLUDED.redis_connections_idle,
			redis_connections_max = EXCLUDED.redis_connections_max,
			metrics_collected_at = EXCLUDED.metrics_collected_at,
			updated_at = NOW()
	`, instance.RunnerID, instance.NodeID, instance.NodeName, instance.DeploymentMode, instance.WorkerMode,
		instance.WorkerEnabled, instance.Version, instance.Hostname, instance.ProcessID,
		instance.DatabaseOK, instance.RedisOK, instance.StartedAt, instance.LastSeenAt,
		cpuUsagePercent, memoryUsedBytes, memoryLimitBytes, memoryUsagePercent,
		inFlightRequests, goroutineCount, dbConnectionsActive, dbConnectionsIdle, dbConnectionsMax,
		redisConnectionsActive, redisConnectionsIdle, redisConnectionsMax, metricsCollectedAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (r *clusterRepository) RenameNode(ctx context.Context, nodeID, displayName string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE cluster_nodes
		SET display_name = $2, updated_at = NOW()
		WHERE node_id = $1
	`, nodeID, displayName)
	if err != nil {
		if isUniqueViolation(err) {
			return service.ErrClusterNodeNameConflict
		}
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return service.ErrClusterNodeNotFound
	}
	return nil
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
		SELECT node_id, runner_id, node_name, deployment_mode, worker_mode, worker_enabled,
			version, hostname, process_id, database_ok, redis_ok, started_at, last_seen_at, stopped_at,
			cpu_usage_percent, memory_used_bytes, memory_limit_bytes, memory_usage_percent,
			in_flight_requests, goroutine_count,
			db_connections_active, db_connections_idle, db_connections_max,
			redis_connections_active, redis_connections_idle, redis_connections_max,
			metrics_collected_at
		FROM (
			SELECT DISTINCT ON (instance.node_id)
				instance.node_id,
				instance.runner_id,
				node.display_name AS node_name,
				instance.deployment_mode,
				instance.worker_mode,
				instance.worker_enabled,
				instance.version,
				instance.hostname,
				instance.process_id,
				instance.database_ok,
				instance.redis_ok,
				instance.started_at,
				instance.last_seen_at,
				instance.stopped_at,
				instance.cpu_usage_percent,
				instance.memory_used_bytes,
				instance.memory_limit_bytes,
				instance.memory_usage_percent,
				instance.in_flight_requests,
				instance.goroutine_count,
				instance.db_connections_active,
				instance.db_connections_idle,
				instance.db_connections_max,
				instance.redis_connections_active,
				instance.redis_connections_idle,
				instance.redis_connections_max,
				instance.metrics_collected_at
			FROM cluster_instances AS instance
			JOIN cluster_nodes AS node ON node.node_id = instance.node_id
			ORDER BY instance.node_id,
				(instance.stopped_at IS NULL) DESC,
				instance.started_at DESC,
				instance.last_seen_at DESC
		) AS latest
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
		var cpuUsagePercent, memoryUsagePercent sql.NullFloat64
		var memoryUsedBytes, memoryLimitBytes, inFlightRequests sql.NullInt64
		var goroutineCount, dbConnectionsActive, dbConnectionsIdle, dbConnectionsMax sql.NullInt64
		var redisConnectionsActive, redisConnectionsIdle, redisConnectionsMax sql.NullInt64
		var metricsCollectedAt sql.NullTime
		if err := rows.Scan(
			&instance.NodeID, &instance.RunnerID, &instance.NodeName, &instance.DeploymentMode,
			&instance.WorkerMode, &instance.WorkerEnabled, &instance.Version,
			&instance.Hostname, &instance.ProcessID, &instance.DatabaseOK,
			&instance.RedisOK, &instance.StartedAt, &instance.LastSeenAt, &stoppedAt,
			&cpuUsagePercent, &memoryUsedBytes, &memoryLimitBytes, &memoryUsagePercent,
			&inFlightRequests, &goroutineCount,
			&dbConnectionsActive, &dbConnectionsIdle, &dbConnectionsMax,
			&redisConnectionsActive, &redisConnectionsIdle, &redisConnectionsMax,
			&metricsCollectedAt,
		); err != nil {
			return nil, err
		}
		if stoppedAt.Valid {
			value := stoppedAt.Time
			instance.StoppedAt = &value
		}
		if metricsCollectedAt.Valid {
			load := &service.ClusterInstanceLoad{
				InFlightRequests:       inFlightRequests.Int64,
				GoroutineCount:         int(goroutineCount.Int64),
				DBConnectionsActive:    int(dbConnectionsActive.Int64),
				DBConnectionsIdle:      int(dbConnectionsIdle.Int64),
				DBConnectionsMax:       int(dbConnectionsMax.Int64),
				RedisConnectionsActive: int(redisConnectionsActive.Int64),
				RedisConnectionsIdle:   int(redisConnectionsIdle.Int64),
				RedisConnectionsMax:    int(redisConnectionsMax.Int64),
				CollectedAt:            metricsCollectedAt.Time,
			}
			if cpuUsagePercent.Valid {
				value := cpuUsagePercent.Float64
				load.CPUUsagePercent = &value
			}
			if memoryUsedBytes.Valid {
				value := memoryUsedBytes.Int64
				load.MemoryUsedBytes = &value
			}
			if memoryLimitBytes.Valid {
				value := memoryLimitBytes.Int64
				load.MemoryLimitBytes = &value
			}
			if memoryUsagePercent.Valid {
				value := memoryUsagePercent.Float64
				load.MemoryUsagePercent = &value
			}
			instance.Load = load
		}
		instances = append(instances, instance)
	}
	return instances, rows.Err()
}

func (r *clusterRepository) ClusterConnectionStats() service.ClusterConnectionStats {
	if r == nil || r.db == nil {
		return service.ClusterConnectionStats{}
	}
	stats := r.db.Stats()
	return service.ClusterConnectionStats{
		Active: stats.InUse,
		Idle:   stats.Idle,
		Max:    stats.MaxOpenConnections,
	}
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
		WHERE COALESCE(stopped_at, last_seen_at) < $1
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

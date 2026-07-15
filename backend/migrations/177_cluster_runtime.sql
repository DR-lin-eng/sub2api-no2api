-- Multi-instance node inventory and renewable task leases.

CREATE TABLE IF NOT EXISTS cluster_instances (
    runner_id VARCHAR(192) PRIMARY KEY,
    node_name VARCHAR(128) NOT NULL,
    deployment_mode VARCHAR(32) NOT NULL,
    worker_mode VARCHAR(16) NOT NULL,
    worker_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    version VARCHAR(64) NOT NULL DEFAULT '',
    hostname VARCHAR(255) NOT NULL DEFAULT '',
    process_id INTEGER NOT NULL DEFAULT 0,
    database_ok BOOLEAN NOT NULL DEFAULT TRUE,
    redis_ok BOOLEAN NOT NULL DEFAULT FALSE,
    started_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL,
    stopped_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cluster_instances_last_seen
    ON cluster_instances (last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_cluster_instances_node_name
    ON cluster_instances (node_name, last_seen_at DESC);
CREATE INDEX IF NOT EXISTS idx_cluster_instances_stopped_at
    ON cluster_instances (stopped_at)
    WHERE stopped_at IS NOT NULL;

CREATE TABLE IF NOT EXISTS cluster_task_runs (
    id BIGSERIAL PRIMARY KEY,
    run_id VARCHAR(64) NOT NULL UNIQUE,
    task_key VARCHAR(255) NOT NULL,
    active_key VARCHAR(255),
    status VARCHAR(24) NOT NULL,
    node_name VARCHAR(128) NOT NULL,
    runner_id VARCHAR(192) NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    result JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_message TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL,
    heartbeat_at TIMESTAMPTZ NOT NULL,
    lease_until TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_cluster_task_runs_active_key
    ON cluster_task_runs (active_key)
    WHERE active_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_cluster_task_runs_started_at
    ON cluster_task_runs (started_at DESC);
CREATE INDEX IF NOT EXISTS idx_cluster_task_runs_status_lease
    ON cluster_task_runs (status, lease_until);
CREATE INDEX IF NOT EXISTS idx_cluster_task_runs_finished_at
    ON cluster_task_runs (finished_at)
    WHERE finished_at IS NOT NULL;

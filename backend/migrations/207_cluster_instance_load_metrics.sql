-- Per-node runtime load snapshots carried by the existing cluster heartbeat.

ALTER TABLE cluster_instances
    ADD COLUMN IF NOT EXISTS cpu_usage_percent DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS memory_used_bytes BIGINT,
    ADD COLUMN IF NOT EXISTS memory_limit_bytes BIGINT,
    ADD COLUMN IF NOT EXISTS memory_usage_percent DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS in_flight_requests BIGINT,
    ADD COLUMN IF NOT EXISTS goroutine_count INTEGER,
    ADD COLUMN IF NOT EXISTS db_connections_active INTEGER,
    ADD COLUMN IF NOT EXISTS db_connections_idle INTEGER,
    ADD COLUMN IF NOT EXISTS db_connections_max INTEGER,
    ADD COLUMN IF NOT EXISTS redis_connections_active INTEGER,
    ADD COLUMN IF NOT EXISTS redis_connections_idle INTEGER,
    ADD COLUMN IF NOT EXISTS redis_connections_max INTEGER,
    ADD COLUMN IF NOT EXISTS metrics_collected_at TIMESTAMPTZ;

-- Durable desired-version state and per-node rolling release orchestration.

CREATE TABLE IF NOT EXISTS cluster_nodes (
    node_id VARCHAR(64) PRIMARY KEY,
    display_name VARCHAR(128) NOT NULL,
    configured_name VARCHAR(128) NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_cluster_nodes_display_name
    ON cluster_nodes (display_name);

ALTER TABLE cluster_instances
    ADD COLUMN IF NOT EXISTS node_id VARCHAR(64) NOT NULL DEFAULT '';

INSERT INTO cluster_nodes (node_id, display_name, configured_name, last_seen_at)
SELECT
    'legacy-' || MD5(node_name),
    node_name,
    node_name,
    MAX(last_seen_at)
FROM cluster_instances
WHERE node_id = ''
GROUP BY node_name
ON CONFLICT DO NOTHING;

UPDATE cluster_instances
SET node_id = 'legacy-' || MD5(node_name)
WHERE node_id = '';

CREATE INDEX IF NOT EXISTS idx_cluster_instances_node_id_last_seen
    ON cluster_instances (node_id, last_seen_at DESC);

ALTER TABLE cluster_instances
    DROP CONSTRAINT IF EXISTS cluster_instances_node_id_fkey;

ALTER TABLE cluster_instances
    ADD CONSTRAINT cluster_instances_node_id_fkey
    FOREIGN KEY (node_id) REFERENCES cluster_nodes(node_id)
    ON UPDATE CASCADE ON DELETE RESTRICT;

CREATE TABLE IF NOT EXISTS cluster_release_state (
    singleton BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),
    desired_version VARCHAR(64) NOT NULL DEFAULT '',
    active_rollout_id VARCHAR(64),
    generation BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO cluster_release_state (singleton)
VALUES (TRUE)
ON CONFLICT (singleton) DO NOTHING;

CREATE TABLE IF NOT EXISTS cluster_release_rollouts (
    id VARCHAR(64) PRIMARY KEY,
    source_version VARCHAR(64) NOT NULL DEFAULT '',
    target_version VARCHAR(64) NOT NULL,
    status VARCHAR(24) NOT NULL,
    strategy VARCHAR(24) NOT NULL DEFAULT 'rolling',
    max_unavailable INTEGER NOT NULL DEFAULT 1 CHECK (max_unavailable = 1),
    created_by BIGINT NOT NULL DEFAULT 0,
    error_message TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_cluster_release_rollouts_active
    ON cluster_release_rollouts ((TRUE))
    WHERE status IN ('running', 'paused');

CREATE INDEX IF NOT EXISTS idx_cluster_release_rollouts_created_at
    ON cluster_release_rollouts (created_at DESC);

CREATE TABLE IF NOT EXISTS cluster_release_targets (
    rollout_id VARCHAR(64) NOT NULL REFERENCES cluster_release_rollouts(id) ON DELETE CASCADE,
    node_id VARCHAR(64) NOT NULL REFERENCES cluster_nodes(node_id) ON UPDATE CASCADE ON DELETE RESTRICT,
    node_name VARCHAR(128) NOT NULL,
    ordinal INTEGER NOT NULL,
    source_version VARCHAR(64) NOT NULL,
    target_version VARCHAR(64) NOT NULL,
    status VARCHAR(24) NOT NULL DEFAULT 'pending',
    attempt INTEGER NOT NULL DEFAULT 0,
    lease_owner VARCHAR(192) NOT NULL DEFAULT '',
    lease_until TIMESTAMPTZ,
    source_runner_id VARCHAR(192) NOT NULL DEFAULT '',
    observed_runner_id VARCHAR(192) NOT NULL DEFAULT '',
    verification_count INTEGER NOT NULL DEFAULT 0,
    last_verified_heartbeat TIMESTAMPTZ,
    error_message TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (rollout_id, node_id),
    UNIQUE (rollout_id, ordinal)
);

CREATE INDEX IF NOT EXISTS idx_cluster_release_targets_status
    ON cluster_release_targets (rollout_id, status, ordinal);

CREATE INDEX IF NOT EXISTS idx_cluster_release_targets_lease
    ON cluster_release_targets (status, lease_until)
    WHERE lease_until IS NOT NULL;

ALTER TABLE cluster_release_state
    DROP CONSTRAINT IF EXISTS cluster_release_state_active_rollout_id_fkey;

ALTER TABLE cluster_release_state
    ADD CONSTRAINT cluster_release_state_active_rollout_id_fkey
    FOREIGN KEY (active_rollout_id) REFERENCES cluster_release_rollouts(id)
    ON DELETE SET NULL;

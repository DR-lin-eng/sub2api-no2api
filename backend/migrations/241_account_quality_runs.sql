-- Account-level quality monitor artifacts. Raw HTML is intentionally absent:
-- only the rendered PNG/WebP and classifier decision are retained.
CREATE TABLE IF NOT EXISTS account_quality_runs (
    id            UUID PRIMARY KEY,
    account_id    BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    model         VARCHAR(200) NOT NULL DEFAULT '',
    effort        VARCHAR(20) NOT NULL DEFAULT 'medium',
    status        VARCHAR(20) NOT NULL,
    label         VARCHAR(32) NOT NULL DEFAULT '',
    confidence    DOUBLE PRECISION NOT NULL DEFAULT 0,
    started_at    TIMESTAMPTZ NOT NULL,
    finished_at   TIMESTAMPTZ NOT NULL,
    latency_ms    BIGINT NOT NULL DEFAULT 0,
    model_version VARCHAR(64) NOT NULL DEFAULT '',
    png           BYTEA,
    webp          BYTEA,
    error_message TEXT NOT NULL DEFAULT '',
    CONSTRAINT account_quality_runs_status_check CHECK (status IN ('ready', 'wrong', 'uncertain', 'error')),
    CONSTRAINT account_quality_runs_confidence_check CHECK (confidence >= 0 AND confidence <= 1)
);
CREATE INDEX IF NOT EXISTS idx_account_quality_runs_finished ON account_quality_runs (finished_at DESC);
CREATE INDEX IF NOT EXISTS idx_account_quality_runs_account_finished ON account_quality_runs (account_id, finished_at DESC);

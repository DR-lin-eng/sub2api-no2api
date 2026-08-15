-- Durable rescue and reconciliation state for clustered usage billing.
-- Existing binaries ignore these additive columns, so rolling upgrades remain
-- compatible while new binaries split unsettled and overlay-cleanup work.

ALTER TABLE usage_billing_jobs
    ADD COLUMN IF NOT EXISTS cleanup_attempts INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_error_class VARCHAR(64),
    ADD COLUMN IF NOT EXISTS last_attempt_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_claimed_by VARCHAR(255),
    ADD COLUMN IF NOT EXISTS reconcile_required_at TIMESTAMPTZ;

ALTER TABLE usage_billing_dead_letters
    ADD COLUMN IF NOT EXISTS replay_count INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_replayed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_replayed_by BIGINT,
    ADD COLUMN IF NOT EXISTS replay_reason TEXT;

CREATE TABLE IF NOT EXISTS usage_billing_admin_actions (
    id BIGSERIAL PRIMARY KEY,
    operator_id BIGINT NOT NULL,
    action VARCHAR(64) NOT NULL,
    source_job_id BIGINT,
    source_dead_letter_id BIGINT,
    request_id VARCHAR(255) NOT NULL,
    api_key_id BIGINT NOT NULL,
    reason TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON COLUMN usage_billing_jobs.reconcile_required_at IS
    'Set when retry count or age crosses the operator attention threshold; the job remains durable.';
COMMENT ON TABLE usage_billing_admin_actions IS
    'Transactional audit trail for manual usage-billing retry and dead-letter replay actions.';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_billing_jobs_unsettled_ready
    ON usage_billing_jobs (available_at, id)
    WHERE settled_at IS NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_billing_jobs_cleanup_ready
    ON usage_billing_jobs (available_at, id)
    WHERE settled_at IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_billing_jobs_reconcile
    ON usage_billing_jobs (reconcile_required_at, id)
    WHERE reconcile_required_at IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_billing_admin_actions_created_at
    ON usage_billing_admin_actions (created_at DESC, id DESC);

-- Keep admin usage model filters on an ordered index scan after switching the
-- displayed-model dimension to requested_model with a legacy model fallback.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_requested_model_filter_created_id
    ON usage_logs (
        (COALESCE(NULLIF(TRIM(requested_model), ''), model)),
        created_at DESC,
        id DESC
    );

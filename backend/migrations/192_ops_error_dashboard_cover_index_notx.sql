-- Keep high-volume dashboard counts and minute buckets off the wide error-log heap.
-- The predicate matches all ops dashboard aggregations and excludes count_tokens rows.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ops_error_logs_dashboard_time_cover
    ON ops_error_logs (created_at DESC)
    INCLUDE (status_code, is_business_limited, error_owner, upstream_status_code, platform, group_id)
    WHERE is_count_tokens = FALSE;

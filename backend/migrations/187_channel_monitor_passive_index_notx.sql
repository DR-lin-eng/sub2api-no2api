-- Keep frequent passive-monitor windows on an indexed channel/time range scan.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_channel_created_monitor
    ON usage_logs (channel_id, created_at DESC)
    INCLUDE (group_id, requested_model, model, duration_ms, actual_cost)
    WHERE channel_id IS NOT NULL;

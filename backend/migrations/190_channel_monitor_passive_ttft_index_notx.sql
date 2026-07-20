-- Cover passive-monitor conversation TTFT aggregation without heap lookups.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_channel_created_monitor_ttft
    ON usage_logs (channel_id, created_at DESC)
    INCLUDE (group_id, requested_model, model, first_token_ms, image_count, video_count, actual_cost)
    WHERE channel_id IS NOT NULL;

DROP INDEX CONCURRENTLY IF EXISTS idx_usage_logs_channel_created_monitor;

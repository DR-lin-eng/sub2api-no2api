-- Partial covering index for the optional Ops image-generation panel.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_image_generation_created_at
    ON usage_logs (created_at DESC)
    INCLUDE (duration_ms, image_count, image_size, image_output_size, group_id, account_id)
    WHERE image_count > 0 AND COALESCE(video_count, 0) = 0;

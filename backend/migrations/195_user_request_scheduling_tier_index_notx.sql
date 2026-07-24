CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_request_scheduling_tier_active
    ON users (request_scheduling_tier)
    WHERE deleted_at IS NULL;

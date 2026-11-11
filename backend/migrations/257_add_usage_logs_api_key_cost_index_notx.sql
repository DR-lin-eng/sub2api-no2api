-- Cover the API-key list's recent/total actual-cost aggregation with an
-- index-only scan. The INCLUDE columns avoid fetching every usage_logs row
-- when a user opens the key list.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_api_key_created_cost
    ON usage_logs (api_key_id, created_at)
    INCLUDE (actual_cost, input_tokens, output_tokens, cache_creation_tokens, cache_read_tokens);

-- Preserve the account/channel usage statistics shown in the admin UI after
-- raw request logs are removed. The row grain keeps every dimension needed by
-- the 30-day dialog while remaining substantially smaller than usage_logs.

CREATE TABLE IF NOT EXISTS ops_account_usage_daily (
    id BIGSERIAL PRIMARY KEY,
    bucket_date DATE NOT NULL,
    account_id BIGINT NOT NULL,
    model VARCHAR(100) NOT NULL,
    inbound_endpoint VARCHAR(128) NOT NULL,
    upstream_endpoint VARCHAR(128) NOT NULL,
    request_count BIGINT NOT NULL DEFAULT 0,
    input_tokens BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_creation_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read_tokens BIGINT NOT NULL DEFAULT 0,
    standard_cost DECIMAL(30, 10) NOT NULL DEFAULT 0,
    account_cost DECIMAL(30, 10) NOT NULL DEFAULT 0,
    user_cost DECIMAL(30, 10) NOT NULL DEFAULT 0,
    duration_sum_ms BIGINT NOT NULL DEFAULT 0,
    duration_sample_count BIGINT NOT NULL DEFAULT 0,
    ttft_sum_ms BIGINT NOT NULL DEFAULT 0,
    ttft_sample_count BIGINT NOT NULL DEFAULT 0,
    last_usage_log_id BIGINT NOT NULL DEFAULT 0,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT ops_account_usage_daily_dimension_key UNIQUE (
        bucket_date,
        account_id,
        model,
        inbound_endpoint,
        upstream_endpoint
    )
);

CREATE INDEX IF NOT EXISTS idx_ops_account_usage_daily_account_date
    ON ops_account_usage_daily (account_id, bucket_date DESC);

CREATE INDEX IF NOT EXISTS idx_ops_account_usage_daily_bucket_date
    ON ops_account_usage_daily (bucket_date);

COMMENT ON TABLE ops_account_usage_daily IS
    'Ops-owned daily account/channel usage aggregates retained independently from raw usage logs.';
COMMENT ON COLUMN ops_account_usage_daily.last_usage_log_id IS
    'Highest raw usage_logs.id included in this dimension row; later rows are merged incrementally.';

-- Seed the exact 30-day display window during upgrade. Subsequent rows are
-- maintained incrementally by DashboardAggregationService before log cleanup.
INSERT INTO ops_account_usage_daily (
    bucket_date,
    account_id,
    model,
    inbound_endpoint,
    upstream_endpoint,
    request_count,
    input_tokens,
    output_tokens,
    cache_creation_tokens,
    cache_read_tokens,
    standard_cost,
    account_cost,
    user_cost,
    duration_sum_ms,
    duration_sample_count,
    ttft_sum_ms,
    ttft_sample_count,
    last_usage_log_id,
    computed_at
)
SELECT
    (created_at AT TIME ZONE CURRENT_SETTING('TimeZone'))::date AS bucket_date,
    account_id,
    COALESCE(NULLIF(TRIM(requested_model), ''), model) AS model,
    COALESCE(NULLIF(TRIM(inbound_endpoint), ''), 'unknown') AS inbound_endpoint,
    COALESCE(NULLIF(TRIM(upstream_endpoint), ''), 'unknown') AS upstream_endpoint,
    COUNT(*) AS request_count,
    COALESCE(SUM(input_tokens), 0) AS input_tokens,
    COALESCE(SUM(output_tokens), 0) AS output_tokens,
    COALESCE(SUM(cache_creation_tokens), 0) AS cache_creation_tokens,
    COALESCE(SUM(cache_read_tokens), 0) AS cache_read_tokens,
    COALESCE(SUM(total_cost), 0) AS standard_cost,
    COALESCE(SUM(COALESCE(account_stats_cost, total_cost) * COALESCE(account_rate_multiplier, 1)), 0) AS account_cost,
    COALESCE(SUM(actual_cost), 0) AS user_cost,
    COALESCE(SUM(duration_ms), 0) AS duration_sum_ms,
    COUNT(duration_ms) AS duration_sample_count,
    COALESCE(SUM(first_token_ms) FILTER (
        WHERE COALESCE(image_count, 0) = 0 AND COALESCE(video_count, 0) = 0
    ), 0) AS ttft_sum_ms,
    COUNT(first_token_ms) FILTER (
        WHERE COALESCE(image_count, 0) = 0 AND COALESCE(video_count, 0) = 0
    ) AS ttft_sample_count,
    MAX(id) AS last_usage_log_id,
    NOW() AS computed_at
FROM usage_logs
WHERE created_at >= (
    (CURRENT_DATE - 29)::timestamp AT TIME ZONE CURRENT_SETTING('TimeZone')
)
  AND created_at < (
    (CURRENT_DATE + 1)::timestamp AT TIME ZONE CURRENT_SETTING('TimeZone')
)
GROUP BY 1, 2, 3, 4, 5
ON CONFLICT (bucket_date, account_id, model, inbound_endpoint, upstream_endpoint)
DO NOTHING;

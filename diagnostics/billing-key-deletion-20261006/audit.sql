-- Read-only triage. Zero actual cost alone does not prove a billing bypass.
-- Exclude intentionally free multipliers and join both live and archived ledgers.
SELECT
    ul.id, ul.created_at, ul.user_id, ul.api_key_id, ul.model,
    ul.input_tokens, ul.output_tokens, ul.total_cost, ul.actual_cost,
    ak.deleted_at AS key_deleted_at,
    CASE
        WHEN d.id IS NOT NULL OR a.request_id IS NOT NULL THEN 'settled'
        WHEN j.id IS NOT NULL THEN 'pending'
        WHEN dl.id IS NOT NULL THEN 'dead_letter'
        ELSE 'no_settlement_record'
    END AS settlement_state,
    dl.reason AS dead_letter_reason
FROM usage_logs ul
LEFT JOIN api_keys ak ON ak.id = ul.api_key_id
LEFT JOIN usage_billing_dedup d
    ON d.request_id = ul.request_id AND d.api_key_id = ul.api_key_id
LEFT JOIN usage_billing_dedup_archive a
    ON a.request_id = ul.request_id AND a.api_key_id = ul.api_key_id
LEFT JOIN usage_billing_jobs j
    ON j.request_id = ul.request_id AND j.api_key_id = ul.api_key_id
LEFT JOIN usage_billing_dead_letters dl
    ON dl.request_id = ul.request_id AND dl.api_key_id = ul.api_key_id
WHERE ul.created_at >= NOW() - INTERVAL '7 days'
    AND ul.actual_cost = 0 AND ul.total_cost > 0 AND ul.rate_multiplier > 0
    AND ul.input_tokens + ul.output_tokens > 0
ORDER BY ul.created_at DESC
LIMIT 500;

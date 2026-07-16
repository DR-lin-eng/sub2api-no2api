-- Convert existing valid probe schedules once so the minute-level scheduler
-- does not repeatedly parse every legacy future timestamp until it becomes due.

WITH legacy AS MATERIALIZED (
    SELECT
        id,
        jsonb_path_query_first_tz(
            jsonb_build_object(
                'value',
                replace(
                    regexp_replace(extra #>> '{upstream_billing_probe,next_probe_at}', 'Z$', '+00:00'),
                    'T',
                    ' '
                )
            ),
            '$.value.datetime()',
            '{}'::jsonb,
            true
        ) #>> '{}' AS parsed_next_probe_at
    FROM accounts
    WHERE deleted_at IS NULL
      AND status = 'active'
      AND platform = 'openai'
      AND type = 'apikey'
      AND extra @> '{"upstream_billing_probe_enabled": true}'::jsonb
      AND extra #>> '{upstream_billing_probe,status}' IN ('ok', 'unsupported', 'failed')
      AND (
          jsonb_typeof(extra #> '{upstream_billing_probe,next_probe_unix}') = 'number'
          AND extra #>> '{upstream_billing_probe,next_probe_unix}' ~ '^[0-9]{1,19}$'
      ) IS NOT TRUE
      AND extra #>> '{upstream_billing_probe,next_probe_at}'
          ~ '^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(\.[0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$'
), parsed AS (
    SELECT id, parsed_next_probe_at::timestamptz AS next_probe_at
    FROM legacy
    WHERE parsed_next_probe_at IS NOT NULL
)
UPDATE accounts AS account
SET extra = jsonb_set(
    account.extra,
    '{upstream_billing_probe,next_probe_unix}',
    to_jsonb(EXTRACT(EPOCH FROM parsed.next_probe_at)::bigint),
    true
)
FROM parsed
WHERE account.id = parsed.id;

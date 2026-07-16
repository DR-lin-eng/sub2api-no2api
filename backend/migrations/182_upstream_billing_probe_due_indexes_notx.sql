-- Keep the minute-level probe scheduler off full-table JSON parsing. The first
-- index drains malformed snapshots, while the second serves steady-state due probes
-- in next-run order. Both remain small because probing is opt-in.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_upstream_billing_probe_legacy
    ON accounts (id)
    WHERE deleted_at IS NULL
      AND status = 'active'
      AND platform = 'openai'
      AND type = 'apikey'
      AND extra @> '{"upstream_billing_probe_enabled": true}'::jsonb
      AND (
          jsonb_typeof(extra #> '{upstream_billing_probe,next_probe_unix}') = 'number'
          AND extra #>> '{upstream_billing_probe,next_probe_unix}' ~ '^[0-9]{1,19}$'
          AND extra #>> '{upstream_billing_probe,status}' IN ('ok', 'unsupported', 'failed')
      ) IS NOT TRUE;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_upstream_billing_probe_due
    ON accounts (((extra #>> '{upstream_billing_probe,next_probe_unix}')::numeric), id)
    WHERE deleted_at IS NULL
      AND status = 'active'
      AND platform = 'openai'
      AND type = 'apikey'
      AND extra @> '{"upstream_billing_probe_enabled": true}'::jsonb
      AND jsonb_typeof(extra #> '{upstream_billing_probe,next_probe_unix}') = 'number'
      AND extra #>> '{upstream_billing_probe,next_probe_unix}' ~ '^[0-9]{1,19}$'
      AND extra #>> '{upstream_billing_probe,status}' IN ('ok', 'unsupported', 'failed');

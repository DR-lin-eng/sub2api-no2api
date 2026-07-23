-- Keep Ollama Cloud group resolution and minute-level refresh scheduling on
-- small opt-in indexes. Legacy rows stay isolated until the next refresh adds
-- the numeric schedule field.

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_ollama_cloud_usage_group
    ON accounts ((credentials ->> 'api_key'), updated_at DESC, id)
    WHERE deleted_at IS NULL
      AND platform IN ('openai', 'anthropic')
      AND type = 'apikey'
      AND btrim(credentials ->> 'base_url') ~ '^[hH][tT][tT][pP][sS]://([wW][wW][wW]\.)?[oO][lL][lL][aA][mM][aA]\.[cC][oO][mM](:443)?(/v1)?$'
      AND jsonb_typeof(credentials -> 'api_key') = 'string'
      AND credentials ->> 'api_key' <> ''
      AND jsonb_typeof(extra -> 'ollama_cloud_usage_session') = 'string';

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_ollama_cloud_usage_legacy
    ON accounts (id)
    WHERE deleted_at IS NULL
      AND status = 'active'
      AND platform IN ('openai', 'anthropic')
      AND type = 'apikey'
      AND btrim(credentials ->> 'base_url') ~ '^[hH][tT][tT][pP][sS]://([wW][wW][wW]\.)?[oO][lL][lL][aA][mM][aA]\.[cC][oO][mM](:443)?(/v1)?$'
      AND jsonb_typeof(credentials -> 'api_key') = 'string'
      AND credentials ->> 'api_key' <> ''
      AND jsonb_typeof(extra -> 'ollama_cloud_usage_session') = 'string'
      AND extra @> '{"ollama_cloud_usage_auto_refresh": true}'::jsonb
      AND (
          jsonb_typeof(extra #> '{ollama_cloud_usage_snapshot,next_refresh_unix}') = 'number'
          AND (extra #>> '{ollama_cloud_usage_snapshot,next_refresh_unix}')::numeric =
              trunc((extra #>> '{ollama_cloud_usage_snapshot,next_refresh_unix}')::numeric)
          AND (extra #>> '{ollama_cloud_usage_snapshot,next_refresh_unix}')::numeric
              BETWEEN 0 AND 9223372036854775807
          AND extra #>> '{ollama_cloud_usage_snapshot,status}' IN ('ok', 'unauthorized', 'failed')
      ) IS NOT TRUE;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_ollama_cloud_usage_due
    ON accounts (((extra #>> '{ollama_cloud_usage_snapshot,next_refresh_unix}')::numeric), id, (credentials ->> 'api_key'))
    WHERE deleted_at IS NULL
      AND status = 'active'
      AND platform IN ('openai', 'anthropic')
      AND type = 'apikey'
      AND btrim(credentials ->> 'base_url') ~ '^[hH][tT][tT][pP][sS]://([wW][wW][wW]\.)?[oO][lL][lL][aA][mM][aA]\.[cC][oO][mM](:443)?(/v1)?$'
      AND jsonb_typeof(credentials -> 'api_key') = 'string'
      AND credentials ->> 'api_key' <> ''
      AND jsonb_typeof(extra -> 'ollama_cloud_usage_session') = 'string'
      AND extra @> '{"ollama_cloud_usage_auto_refresh": true}'::jsonb
      AND jsonb_typeof(extra #> '{ollama_cloud_usage_snapshot,next_refresh_unix}') = 'number'
      AND (extra #>> '{ollama_cloud_usage_snapshot,next_refresh_unix}')::numeric =
          trunc((extra #>> '{ollama_cloud_usage_snapshot,next_refresh_unix}')::numeric)
      AND (extra #>> '{ollama_cloud_usage_snapshot,next_refresh_unix}')::numeric
          BETWEEN 0 AND 9223372036854775807
      AND extra #>> '{ollama_cloud_usage_snapshot,status}' IN ('ok', 'unauthorized', 'failed');

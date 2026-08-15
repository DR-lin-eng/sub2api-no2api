-- Add ordered, per-key group routing while keeping api_keys.group_id as the
-- compatibility mirror of the first candidate.

ALTER TABLE api_keys
    ADD COLUMN IF NOT EXISTS group_bindings JSONB NOT NULL DEFAULT '[]'::jsonb;

UPDATE api_keys
SET group_bindings = jsonb_build_array(jsonb_build_object('group_id', group_id))
WHERE group_id IS NOT NULL
  AND (group_bindings IS NULL OR group_bindings = '[]'::jsonb);

COMMENT ON COLUMN api_keys.group_bindings IS
    'Ordered API key group candidates. Each item contains group_id and an optional max_rate_multiplier; group_id mirrors the first item.';

CREATE INDEX IF NOT EXISTS idx_api_keys_group_bindings_gin
    ON api_keys USING GIN (group_bindings jsonb_path_ops);

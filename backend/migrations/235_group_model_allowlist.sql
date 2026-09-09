-- Add a durable group model allowlist while preserving the existing display-only
-- models_list_config for rolling upgrades and older clients.
ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS model_allowlist JSONB NOT NULL DEFAULT '{}'::jsonb;

UPDATE groups
SET model_allowlist = models_list_config
WHERE model_allowlist = '{}'::jsonb
  AND models_list_config <> '{}'::jsonb;

COMMENT ON COLUMN groups.model_allowlist IS
    'Group model allowlist: constrains both model listing responses and request admission';

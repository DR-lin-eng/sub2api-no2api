-- Keep every existing token valid while converging managed Media Studio keys to
-- a new canonical name per user and group. Every pre-upgrade key remains usable
-- under a deterministic legacy display name and cannot gain managed-key powers.
DROP INDEX IF EXISTS idx_api_keys_media_studio_user_group;

UPDATE api_keys AS target
SET name = left('Media Studio Legacy ' || target.id::text, 100),
	updated_at = CURRENT_TIMESTAMP
WHERE target.deleted_at IS NULL
	AND target.group_id IS NOT NULL
	AND lower(btrim(target.name)) = 'media studio';

CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_media_studio_user_group
    ON api_keys (user_id, group_id, lower(name))
    WHERE deleted_at IS NULL
      AND group_id IS NOT NULL
      AND lower(name) = 'media studio';

-- Preserve existing Media Studio tokens while reserving the canonical managed name.
UPDATE api_keys AS target
SET name = left('Media Studio Legacy ' || target.id::text, 100),
    updated_at = CURRENT_TIMESTAMP
WHERE target.deleted_at IS NULL
  AND target.group_id IS NOT NULL
  AND lower(btrim(target.name)) = 'media studio';

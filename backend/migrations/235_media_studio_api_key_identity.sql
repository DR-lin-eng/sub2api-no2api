-- 媒体工坊 Key 使用固定名称，并以 user_id + group_id 区分。
-- 只调整索引，不修改或清理用户已有的 API Key。
DROP INDEX IF EXISTS idx_api_keys_media_studio_user_group;

CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_media_studio_user_group
    ON api_keys (user_id, group_id, lower(name))
    WHERE deleted_at IS NULL
      AND group_id IS NOT NULL
      AND lower(name) = 'media studio';

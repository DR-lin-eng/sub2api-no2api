-- 234_dedupe_media_studio_api_keys.sql
-- 媒体工坊 API Key 的唯一身份是 user_id + group_id，不区分媒体类型。
-- 所有媒体工坊 Key 使用同一个显示名称；普通 Key 不参与该约束。
CREATE UNIQUE INDEX IF NOT EXISTS idx_api_keys_media_studio_user_group
    ON api_keys (user_id, group_id, lower(name))
    WHERE deleted_at IS NULL
      AND group_id IS NOT NULL
      AND lower(name) = 'media studio';

-- 模型级媒体限制：媒体工坊按模型限制上传开关、允许的文件类型、各类型数量/大小与视频时长区间。
-- 空对象表示不做任何限制，行为与改造前一致。
ALTER TABLE custom_model_configs
    ADD COLUMN IF NOT EXISTS media_limits JSONB NOT NULL DEFAULT '{}'::jsonb;

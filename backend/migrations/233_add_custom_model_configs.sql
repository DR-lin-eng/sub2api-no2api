-- 233_add_custom_model_configs.sql
-- 添加自定义模型配置表，允许管理员手动配置模型的多模态能力

CREATE TABLE IF NOT EXISTS custom_model_configs (
    id BIGSERIAL PRIMARY KEY,
    model_name VARCHAR(255) NOT NULL,
    prefix_match BOOLEAN NOT NULL DEFAULT FALSE,
    capabilities JSONB NOT NULL DEFAULT '[]'::jsonb,
    video_api_type VARCHAR(255) NOT NULL DEFAULT '',
    template_id BIGINT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_custom_model_configs_model_name
    ON custom_model_configs (model_name);

COMMENT ON TABLE custom_model_configs IS '自定义模型配置表';
COMMENT ON COLUMN custom_model_configs.model_name IS '模型名称（唯一）';
COMMENT ON COLUMN custom_model_configs.prefix_match IS '是否按模型名前缀匹配';
COMMENT ON COLUMN custom_model_configs.capabilities IS '模型能力列表，如 ["image", "video", "audio"]';
COMMENT ON COLUMN custom_model_configs.video_api_type IS '视频 API 类型（grok_video, agnes_video 等），仅当 capabilities 包含 video 时有效';
COMMENT ON COLUMN custom_model_configs.template_id IS '可复用的请求适配模板 ID';

-- 添加系统设置字段：是否启用自定义模型配置
INSERT INTO settings (key, value)
VALUES (
    'custom_model_config_enabled',
    'false'
)
ON CONFLICT (key) DO NOTHING;

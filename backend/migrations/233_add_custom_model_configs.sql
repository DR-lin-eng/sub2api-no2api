-- 233_add_custom_model_configs.sql
-- 添加自定义模型配置表，允许管理员手动配置模型的多模态能力

CREATE TABLE IF NOT EXISTS custom_model_configs (
    id BIGSERIAL PRIMARY KEY,
    model_name VARCHAR(255) NOT NULL,
    capabilities JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_custom_model_configs_model_name
    ON custom_model_configs (model_name);

COMMENT ON TABLE custom_model_configs IS '自定义模型配置表';
COMMENT ON COLUMN custom_model_configs.model_name IS '模型名称（唯一）';
COMMENT ON COLUMN custom_model_configs.capabilities IS '模型能力列表，如 ["image", "video", "audio"]';

-- 添加系统设置字段：是否启用自定义模型配置
INSERT INTO settings (key, value)
VALUES (
    'custom_model_config_enabled',
    'false'
)
ON CONFLICT (key) DO NOTHING;

-- 236_add_custom_model_config_prefix_match.sql
-- 允许自定义模型能力配置按模型名前缀匹配。

ALTER TABLE custom_model_configs
    ADD COLUMN IF NOT EXISTS prefix_match BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN custom_model_configs.prefix_match IS '是否按模型名前缀匹配';

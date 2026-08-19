-- 添加可复用的自定义模型请求适配模板，并允许模型配置引用模板。

CREATE TABLE IF NOT EXISTS custom_model_request_templates (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description VARCHAR(500) NOT NULL DEFAULT '',
    request_adapter JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_custom_model_request_templates_name
    ON custom_model_request_templates (name);

ALTER TABLE custom_model_configs
    ADD COLUMN IF NOT EXISTS template_id BIGINT NULL
    REFERENCES custom_model_request_templates(id)
    ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_custom_model_configs_template_id
    ON custom_model_configs (template_id);

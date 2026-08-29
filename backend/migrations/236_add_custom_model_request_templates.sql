CREATE TABLE IF NOT EXISTS custom_model_request_templates (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    description VARCHAR(500) NOT NULL DEFAULT '',
    request_adapter JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_custom_model_request_templates_name_ci
    ON custom_model_request_templates (lower(name));

ALTER TABLE custom_model_configs
    ADD COLUMN IF NOT EXISTS template_id BIGINT NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'custom_model_configs_template_id_fkey'
          AND conrelid = 'custom_model_configs'::regclass
    ) THEN
        ALTER TABLE custom_model_configs
            ADD CONSTRAINT custom_model_configs_template_id_fkey
            FOREIGN KEY (template_id)
            REFERENCES custom_model_request_templates(id)
            ON DELETE SET NULL;
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_custom_model_configs_template_id
    ON custom_model_configs (template_id);

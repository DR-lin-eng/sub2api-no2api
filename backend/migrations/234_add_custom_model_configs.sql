-- Add administrator-defined multimodal capabilities without changing existing routing by default.
CREATE TABLE IF NOT EXISTS custom_model_configs (
    id BIGSERIAL PRIMARY KEY,
    model_name VARCHAR(255) NOT NULL,
    prefix_match BOOLEAN NOT NULL DEFAULT FALSE,
    capabilities JSONB NOT NULL DEFAULT '[]'::jsonb,
    video_api_type VARCHAR(255) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_custom_model_configs_model_name_ci
    ON custom_model_configs (lower(model_name));

INSERT INTO settings (key, value)
VALUES ('custom_model_config_enabled', 'false')
ON CONFLICT (key) DO NOTHING;

ALTER TABLE groups
    ADD COLUMN IF NOT EXISTS long_context_pricing_enabled BOOLEAN DEFAULT TRUE,
    ADD COLUMN IF NOT EXISTS model_pricing JSONB;

UPDATE groups
SET long_context_pricing_enabled = TRUE
WHERE long_context_pricing_enabled IS NULL;

ALTER TABLE groups
    ALTER COLUMN long_context_pricing_enabled SET DEFAULT TRUE,
    ALTER COLUMN long_context_pricing_enabled SET NOT NULL;

COMMENT ON COLUMN groups.long_context_pricing_enabled IS
    'Whether official and channel long-context pricing tiers are enabled; defaults to true to preserve existing billing';
COMMENT ON COLUMN groups.model_pricing IS
    'Per-model group pricing overrides channel and built-in model pricing';

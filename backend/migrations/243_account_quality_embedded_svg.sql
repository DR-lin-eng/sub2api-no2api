-- Embedded SVG previews are additive; existing PNG/WebP artifacts remain valid.
ALTER TABLE account_quality_runs
    ADD COLUMN IF NOT EXISTS preview_svg BYTEA,
    ADD COLUMN IF NOT EXISTS preview_format VARCHAR(16) NOT NULL DEFAULT '';

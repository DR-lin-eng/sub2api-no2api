-- Opt-in per-image billing; false preserves existing installations.
ALTER TABLE groups ADD COLUMN IF NOT EXISTS image_count_billing BOOLEAN NOT NULL DEFAULT false;
COMMENT ON COLUMN groups.image_count_billing IS '专用图片端点是否按实际出图张数计费';

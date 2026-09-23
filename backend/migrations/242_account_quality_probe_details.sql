-- Additive migration: old rows have no dialogue metadata and remain displayable.
ALTER TABLE account_quality_runs
    ADD COLUMN IF NOT EXISTS probe_details JSONB NOT NULL DEFAULT '{}'::jsonb;

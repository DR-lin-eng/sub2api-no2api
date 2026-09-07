-- Preserve local duration_ms / first_token_ms for operations and scheduling.
-- Optional allowlisted OpenAI telemetry is projected separately for display.
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS openai_timing JSONB;

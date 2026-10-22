-- A reported zero is distinct from historical/missing upstream usage.
-- Reasoning is already included in output_tokens and is not billed again.
ALTER TABLE usage_logs ADD COLUMN IF NOT EXISTS reasoning_tokens INTEGER;
COMMENT ON COLUMN usage_logs.reasoning_tokens IS 'Upstream reasoning output token count; NULL when not reported';

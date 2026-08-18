-- Add optional recurring time-based multipliers to channel model pricing.
-- The column is nullable so existing rows retain exactly the previous 1x cost.
ALTER TABLE channel_model_pricing
    ADD COLUMN IF NOT EXISTS time_pricing JSONB NULL;

COMMENT ON COLUMN channel_model_pricing.time_pricing IS
    'Optional IANA timezone and recurring daily multiplier periods for channel token pricing';

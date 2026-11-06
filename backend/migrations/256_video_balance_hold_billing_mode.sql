-- Freeze the video billing mode so per-request settlement never re-prices by duration.
ALTER TABLE video_balance_holds
    ADD COLUMN IF NOT EXISTS billing_mode VARCHAR(16) NOT NULL DEFAULT '';

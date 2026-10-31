-- Monetary/ownership facts only. Media, prompts and gallery history stay in the browser.
ALTER TABLE video_balance_holds
    ADD COLUMN IF NOT EXISTS base_amount NUMERIC(20, 8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS rate_multiplier NUMERIC(20, 10) NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS settled_duration_seconds INTEGER NOT NULL DEFAULT 0;

-- Earlier preview builds did not persist a creation-time price snapshot.
UPDATE video_balance_holds
SET base_amount = hold_amount
WHERE base_amount = 0 AND hold_amount > 0;

CREATE INDEX IF NOT EXISTS idx_video_balance_holds_unbound_expiry
    ON video_balance_holds (expires_at, hold_key)
    WHERE status = 'pending' AND upstream_request_id = '';

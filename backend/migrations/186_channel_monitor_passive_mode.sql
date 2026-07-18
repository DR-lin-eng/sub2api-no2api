-- Add passive channel monitoring. Active remains the compatibility default.
-- Passive monitors bind to a pricing channel and derive health from real
-- usage_logs / ops_error_logs instead of sending synthetic upstream requests.

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS monitor_mode VARCHAR(16) NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS channel_id BIGINT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'channel_monitors_monitor_mode_check'
          AND conrelid = 'channel_monitors'::regclass
    ) THEN
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_monitor_mode_check
            CHECK (monitor_mode IN ('active', 'passive'));
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'channel_monitors_channel_id_fkey'
          AND conrelid = 'channel_monitors'::regclass
    ) THEN
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_channel_id_fkey
            FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE SET NULL;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_channel_monitors_mode_channel
    ON channel_monitors (monitor_mode, channel_id);

ALTER TABLE channel_monitor_histories
    DROP CONSTRAINT IF EXISTS channel_monitor_histories_status_check;

ALTER TABLE channel_monitor_histories
    ADD CONSTRAINT channel_monitor_histories_status_check
    CHECK (status IN ('operational', 'degraded', 'failed', 'error', 'unknown'));

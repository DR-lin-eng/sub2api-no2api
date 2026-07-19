-- Allow passive monitors to observe one routing group without requiring a
-- pricing channel. Existing channel-targeted monitors remain unchanged.

ALTER TABLE channel_monitors
    ADD COLUMN IF NOT EXISTS group_id BIGINT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'channel_monitors_group_id_fkey'
          AND conrelid = 'channel_monitors'::regclass
    ) THEN
        ALTER TABLE channel_monitors
            ADD CONSTRAINT channel_monitors_group_id_fkey
            FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE SET NULL;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_channel_monitors_mode_group
    ON channel_monitors (monitor_mode, group_id);

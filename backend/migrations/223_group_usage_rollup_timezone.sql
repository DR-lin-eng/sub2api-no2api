-- Keep group-usage rollups on the configured database-session timezone and
-- replace the upstream singleton-row insert lock with a short advisory barrier.
-- Normal current-day writes never read or lock usage_group_rollup_state.

ALTER TABLE usage_group_rollup_state
    ADD COLUMN IF NOT EXISTS timezone_name TEXT NOT NULL DEFAULT 'Asia/Shanghai';

COMMENT ON COLUMN usage_group_rollup_state.timezone_name IS 'IANA timezone used by the current rollup buckets.';
COMMENT ON COLUMN usage_group_rollup_state.closed_before IS 'Exclusive upper local-date bound of fully published buckets.';
COMMENT ON COLUMN usage_group_daily_rollups.bucket_date IS 'Local calendar date in usage_group_rollup_state.timezone_name.';

-- These two int32 keys spell SUB2/GRPU. All current-day usage-log mutations
-- take the shared transaction lock. The publisher takes the exclusive lock in
-- a separate, short transaction before it snapshots closed days, so it waits
-- for cross-midnight writers without blocking new writes during the backfill.
CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    affected_at TIMESTAMPTZ;
    affected_date DATE;
    configured_timezone TEXT := current_setting('TimeZone');
    current_bucket_start TIMESTAMPTZ :=
        date_trunc('day', clock_timestamp() AT TIME ZONE configured_timezone)
        AT TIME ZONE configured_timezone;
BEGIN
    IF TG_OP = 'DELETE' THEN
        affected_at := OLD.created_at;
    ELSIF OLD.group_id IS NULL THEN
        affected_at := NEW.created_at;
    ELSIF NEW.group_id IS NULL THEN
        affected_at := OLD.created_at;
    ELSE
        affected_at := LEAST(OLD.created_at, NEW.created_at);
    END IF;

    IF affected_at >= current_bucket_start THEN
        PERFORM pg_advisory_xact_lock_shared(1398096434, 1196576853);
    ELSE
        affected_date := (affected_at AT TIME ZONE configured_timezone)::date;
        UPDATE usage_group_rollup_state
        SET closed_before = LEAST(closed_before, affected_date),
            updated_at = NOW()
        WHERE id = 1
          AND closed_before > affected_date;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state_after_insert()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    affected_at TIMESTAMPTZ;
    affected_date DATE;
    configured_timezone TEXT := current_setting('TimeZone');
    current_bucket_start TIMESTAMPTZ :=
        date_trunc('day', clock_timestamp() AT TIME ZONE configured_timezone)
        AT TIME ZONE configured_timezone;
BEGIN
    SELECT MIN(created_at)
    INTO affected_at
    FROM inserted_usage_logs
    WHERE group_id IS NOT NULL;

    IF affected_at IS NULL THEN
        RETURN NULL;
    END IF;

    IF affected_at >= current_bucket_start THEN
        PERFORM pg_advisory_xact_lock_shared(1398096434, 1196576853);
        RETURN NULL;
    END IF;

    affected_date := (affected_at AT TIME ZONE configured_timezone)::date;
    UPDATE usage_group_rollup_state
    SET closed_before = LEAST(closed_before, affected_date),
        updated_at = NOW()
    WHERE id = 1
      AND closed_before > affected_date;

    RETURN NULL;
END;
$$;

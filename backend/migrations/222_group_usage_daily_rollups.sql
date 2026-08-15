-- /admin/groups group-usage daily rollups.
-- Historical rows are backfilled by the dashboard aggregation worker after startup.

CREATE TABLE IF NOT EXISTS usage_group_daily_rollups (
    bucket_date DATE NOT NULL,
    group_id BIGINT NOT NULL,
    actual_cost DECIMAL(20, 10) NOT NULL DEFAULT 0,
    computed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (bucket_date, group_id)
);

COMMENT ON TABLE usage_group_daily_rollups IS 'Daily group actual-cost rollups.';

CREATE TABLE IF NOT EXISTS usage_group_rollup_state (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    closed_before DATE NOT NULL DEFAULT DATE '1970-01-01',
    retained_from TIMESTAMPTZ NOT NULL DEFAULT TIMESTAMPTZ '1970-01-01 00:00:00+00',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE usage_group_rollup_state IS 'Singleton publication watermark for group daily rollups.';
COMMENT ON COLUMN usage_group_rollup_state.closed_before IS 'Exclusive upper date bound of fully published buckets.';

INSERT INTO usage_group_rollup_state (id, closed_before, retained_from)
VALUES (1, DATE '1970-01-01', TIMESTAMPTZ '1970-01-01 00:00:00+00')
ON CONFLICT (id) DO NOTHING;

CREATE OR REPLACE FUNCTION invalidate_group_usage_rollup_state()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    affected_at TIMESTAMPTZ;
    affected_date DATE;
    current_bucket_start TIMESTAMPTZ :=
        date_trunc('day', clock_timestamp() AT TIME ZONE 'Asia/Shanghai')
        AT TIME ZONE 'Asia/Shanghai';
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
        affected_date := (affected_at AT TIME ZONE 'Asia/Shanghai')::date;
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
    current_bucket_start TIMESTAMPTZ :=
        date_trunc('day', clock_timestamp() AT TIME ZONE 'Asia/Shanghai')
        AT TIME ZONE 'Asia/Shanghai';
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

    affected_date := (affected_at AT TIME ZONE 'Asia/Shanghai')::date;
    UPDATE usage_group_rollup_state
    SET closed_before = LEAST(closed_before, affected_date),
        updated_at = NOW()
    WHERE id = 1
      AND closed_before > affected_date;

    RETURN NULL;
END;
$$;

DROP TRIGGER IF EXISTS usage_logs_group_rollup_invalidate_insert ON usage_logs;
CREATE TRIGGER usage_logs_group_rollup_invalidate_insert
AFTER INSERT ON usage_logs
REFERENCING NEW TABLE AS inserted_usage_logs
FOR EACH STATEMENT
EXECUTE FUNCTION invalidate_group_usage_rollup_state_after_insert();

DROP TRIGGER IF EXISTS usage_logs_group_rollup_invalidate_delete ON usage_logs;
CREATE TRIGGER usage_logs_group_rollup_invalidate_delete
AFTER DELETE ON usage_logs
FOR EACH ROW
WHEN (OLD.group_id IS NOT NULL)
EXECUTE FUNCTION invalidate_group_usage_rollup_state();

DROP TRIGGER IF EXISTS usage_logs_group_rollup_invalidate_update ON usage_logs;
CREATE TRIGGER usage_logs_group_rollup_invalidate_update
AFTER UPDATE OF created_at, group_id, actual_cost ON usage_logs
FOR EACH ROW
WHEN (
    (
        OLD.created_at IS DISTINCT FROM NEW.created_at
        OR OLD.group_id IS DISTINCT FROM NEW.group_id
        OR OLD.actual_cost IS DISTINCT FROM NEW.actual_cost
    )
    AND (OLD.group_id IS NOT NULL OR NEW.group_id IS NOT NULL)
)
EXECUTE FUNCTION invalidate_group_usage_rollup_state();

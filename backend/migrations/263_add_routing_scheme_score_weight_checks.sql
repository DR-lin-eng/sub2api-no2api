-- Keep score weights inside 0..100 even when rows are written outside the API
-- (manual SQL, data imports). The API validates as well, but the database is the
-- last line of defense for the scheduler's tuning knobs.
--
-- Clamp first: a constraint added on top of out-of-range historical rows would
-- fail the migration and block startup.

UPDATE group_routing_schemes
SET ttft_weight = LEAST(100, GREATEST(0, ttft_weight))
WHERE ttft_weight < 0 OR ttft_weight > 100;

UPDATE group_routing_schemes
SET error_weight = LEAST(100, GREATEST(0, error_weight))
WHERE error_weight < 0 OR error_weight > 100;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'group_routing_schemes_ttft_weight_check'
    ) THEN
        ALTER TABLE group_routing_schemes
            ADD CONSTRAINT group_routing_schemes_ttft_weight_check
            CHECK (ttft_weight BETWEEN 0 AND 100);
    END IF;
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'group_routing_schemes_error_weight_check'
    ) THEN
        ALTER TABLE group_routing_schemes
            ADD CONSTRAINT group_routing_schemes_error_weight_check
            CHECK (error_weight BETWEEN 0 AND 100);
    END IF;
END
$$;

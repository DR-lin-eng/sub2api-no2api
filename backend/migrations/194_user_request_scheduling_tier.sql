-- Three-level user request admission priority. PostgreSQL 11+ installs a
-- constant default without rewriting existing rows; existing users become normal.
ALTER TABLE users
    ADD COLUMN IF NOT EXISTS request_scheduling_tier SMALLINT NOT NULL DEFAULT 1;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'users_request_scheduling_tier_check'
          AND conrelid = 'users'::regclass
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT users_request_scheduling_tier_check
            CHECK (request_scheduling_tier IN (0, 1, 2)) NOT VALID;
    END IF;
END
$$;

ALTER TABLE users
    VALIDATE CONSTRAINT users_request_scheduling_tier_check;

-- Keep scheduling-tier changes in the same durable auth-cache invalidation
-- path as other user fields embedded in API-key authentication snapshots.
CREATE OR REPLACE FUNCTION enqueue_user_auth_cache_invalidation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    target_user_id BIGINT;
BEGIN
    target_user_id := OLD.id;
    IF TG_OP = 'UPDATE'
       AND OLD.status IS NOT DISTINCT FROM NEW.status
       AND OLD.role IS NOT DISTINCT FROM NEW.role
       AND OLD.request_scheduling_tier IS NOT DISTINCT FROM NEW.request_scheduling_tier
       AND OLD.deleted_at IS NOT DISTINCT FROM NEW.deleted_at THEN
        RETURN NEW;
    END IF;

    INSERT INTO auth_cache_invalidation_outbox (cache_key)
    SELECT encode(sha256(convert_to(k.key, 'UTF8')), 'hex')
    FROM api_keys AS k
    WHERE k.user_id = target_user_id
      AND k.deleted_at IS NULL
      AND k.key <> '';
    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

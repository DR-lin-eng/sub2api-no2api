-- Keep prompt-audit admission O(1) as the durable queue grows. The singleton
-- row is updated in the same transaction as every active-state transition.
CREATE TABLE IF NOT EXISTS prompt_audit_queue_state (
    id           SMALLINT PRIMARY KEY,
    active_count INTEGER NOT NULL DEFAULT 0,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_prompt_audit_queue_state_singleton CHECK (id = 1),
    CONSTRAINT chk_prompt_audit_queue_state_nonnegative CHECK (active_count >= 0)
);

INSERT INTO prompt_audit_queue_state (id, active_count)
SELECT 1, COUNT(*)::INTEGER
FROM prompt_audit_jobs
WHERE status IN ('staging', 'queued', 'processing', 'retry')
ON CONFLICT (id) DO NOTHING;

CREATE OR REPLACE FUNCTION public.sync_prompt_audit_queue_state()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
DECLARE
    delta INTEGER := 0;
    old_active BOOLEAN := FALSE;
    new_active BOOLEAN := FALSE;
BEGIN
    IF TG_OP <> 'INSERT' THEN
        old_active := OLD.status IN ('staging', 'queued', 'processing', 'retry');
    END IF;
    IF TG_OP <> 'DELETE' THEN
        new_active := NEW.status IN ('staging', 'queued', 'processing', 'retry');
    END IF;

    IF new_active AND NOT old_active THEN
        delta := 1;
    ELSIF old_active AND NOT new_active THEN
        delta := -1;
    END IF;

    IF delta <> 0 THEN
        UPDATE prompt_audit_queue_state
        SET active_count=GREATEST(active_count+delta, 0), updated_at=NOW()
        WHERE id=1;
    END IF;

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS prompt_audit_jobs_sync_queue_state ON prompt_audit_jobs;
CREATE TRIGGER prompt_audit_jobs_sync_queue_state
AFTER INSERT OR DELETE OR UPDATE OF status ON prompt_audit_jobs
FOR EACH ROW EXECUTE FUNCTION public.sync_prompt_audit_queue_state();

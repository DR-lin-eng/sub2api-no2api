-- Older nodes may replace accounts.extra from a stale cached account during an
-- ordinary edit. Keep the discovery-owned snapshot across those writes. The
-- guard uses only OLD/NEW row values and performs no additional table query.
CREATE OR REPLACE FUNCTION preserve_account_model_metadata()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    previous_snapshot jsonb := OLD.extra -> 'upstream_model_metadata';
    incoming_snapshot jsonb := NEW.extra -> 'upstream_model_metadata';
    preserve_previous boolean := false;
BEGIN
    -- A capability snapshot belongs to an exact upstream account identity.
    IF NEW.platform IS DISTINCT FROM OLD.platform
        OR NEW.type IS DISTINCT FROM OLD.type
        OR NEW.credentials IS DISTINCT FROM OLD.credentials
        OR NEW.proxy_id IS DISTINCT FROM OLD.proxy_id THEN
        NEW.extra := COALESCE(NEW.extra, '{}'::jsonb) - 'upstream_model_metadata';
        RETURN NEW;
    END IF;

    IF incoming_snapshot IS NOT DISTINCT FROM previous_snapshot THEN
        RETURN NEW;
    END IF;
    IF incoming_snapshot IS NULL OR incoming_snapshot = 'null'::jsonb THEN
        preserve_previous := true;
    ELSIF previous_snapshot ->> 'version' IS DISTINCT FROM '1' THEN
        -- This version must not overwrite a future owner's opaque snapshot.
        preserve_previous := incoming_snapshot ->> 'version' = '1';
    ELSIF incoming_snapshot ->> 'version' = '1'
        AND jsonb_typeof(previous_snapshot -> 'generation') = 'number' THEN
        IF jsonb_typeof(incoming_snapshot -> 'generation') = 'number' THEN
            preserve_previous := (incoming_snapshot ->> 'generation')::numeric
                <= (previous_snapshot ->> 'generation')::numeric;
        ELSE
            preserve_previous := true;
        END IF;
    END IF;

    IF preserve_previous THEN
        NEW.extra := COALESCE(NEW.extra, '{}'::jsonb)
            || jsonb_build_object('upstream_model_metadata', previous_snapshot);
    END IF;
    RETURN NEW;
END;
$$;

DROP TRIGGER IF EXISTS accounts_preserve_model_metadata ON accounts;
CREATE TRIGGER accounts_preserve_model_metadata
BEFORE UPDATE OF extra, credentials, platform, type, proxy_id ON accounts
FOR EACH ROW
WHEN (OLD.extra ? 'upstream_model_metadata')
EXECUTE FUNCTION preserve_account_model_metadata();

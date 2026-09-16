ALTER TABLE proxies
    ADD COLUMN IF NOT EXISTS health_status VARCHAR(20) NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS health_consecutive_failures INTEGER NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS last_health_check_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_health_error TEXT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_proxies_health_status'
    ) THEN
        ALTER TABLE proxies
            ADD CONSTRAINT chk_proxies_health_status
            CHECK (health_status IN ('unknown', 'healthy', 'degraded', 'unhealthy')) NOT VALID;
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_proxies_health_failures_nonnegative'
    ) THEN
        ALTER TABLE proxies
            ADD CONSTRAINT chk_proxies_health_failures_nonnegative
            CHECK (health_consecutive_failures >= 0) NOT VALID;
    END IF;
END $$;

ALTER TABLE proxies VALIDATE CONSTRAINT chk_proxies_health_status;
ALTER TABLE proxies VALIDATE CONSTRAINT chk_proxies_health_failures_nonnegative;

CREATE INDEX IF NOT EXISTS proxy_status_last_health_check_at
    ON proxies (status, last_health_check_at)
    WHERE deleted_at IS NULL;

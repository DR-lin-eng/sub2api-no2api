-- Keep an applied billing job until its Redis pending overlay is cleared.
-- This makes post-commit overlay cleanup retryable without reapplying billing.

ALTER TABLE usage_billing_jobs
    ADD COLUMN IF NOT EXISTS settled_at TIMESTAMPTZ;

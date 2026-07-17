-- Match account list ordering for both ASC NULLS FIRST and its reverse scan.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_last_used_at_id
    ON accounts (last_used_at ASC NULLS FIRST, id ASC);

DROP INDEX CONCURRENTLY IF EXISTS idx_accounts_last_used_at;

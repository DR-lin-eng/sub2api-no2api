-- Support bounded retention scans without walking the whole message table.
-- Financial balance-transfer receipts are intentionally excluded from cleanup.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chat_messages_retention_created_id
    ON chat_messages (created_at, id)
    WHERE kind <> 'balance_transfer';

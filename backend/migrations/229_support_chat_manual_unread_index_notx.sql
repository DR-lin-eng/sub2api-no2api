DROP INDEX CONCURRENTLY IF EXISTS idx_chat_conversations_unread_by_admin_active;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chat_conversations_unread_by_admin_active
    ON chat_conversations (unread_by_admin, manually_unread_by_admin)
    WHERE unread_by_admin > 0 OR manually_unread_by_admin;

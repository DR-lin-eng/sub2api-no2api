CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chat_messages_reply_to_id
    ON chat_messages (reply_to_id);

CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_chat_messages_sender_idempotency
    ON chat_messages (sender_type, sender_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chat_messages_conversation_created_id
    ON chat_messages (conversation_id, created_at, id);

DROP INDEX CONCURRENTLY IF EXISTS idx_chat_messages_conversation_id_created_at;

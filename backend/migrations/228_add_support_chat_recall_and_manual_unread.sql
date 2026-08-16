-- Keep manual inbox reminders separate from the real unread message count,
-- and preserve recalled message rows for audit while hiding their payloads
-- from delivery APIs.
ALTER TABLE chat_conversations
    ADD COLUMN IF NOT EXISTS manually_unread_by_admin BOOLEAN NOT NULL DEFAULT FALSE;

ALTER TABLE chat_messages
    ADD COLUMN IF NOT EXISTS recalled_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS recalled_by BIGINT;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chat_messages_recall_state_check'
    ) THEN
        ALTER TABLE chat_messages
            ADD CONSTRAINT chat_messages_recall_state_check
            CHECK (
                (recalled_at IS NULL AND recalled_by IS NULL)
                OR (
                    recalled_at IS NOT NULL
                    AND recalled_by IS NOT NULL
                    AND sender_type = 'admin'
                    AND kind <> 'balance_transfer'
                )
            ) NOT VALID;
    END IF;
END $$;

ALTER TABLE chat_messages VALIDATE CONSTRAINT chat_messages_recall_state_check;

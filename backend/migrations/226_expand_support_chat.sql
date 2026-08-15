-- Complete support chat with persistent read receipts, structured messages,
-- reusable quick replies, and authorization-aware image assets.
ALTER TABLE chat_conversations
    ADD COLUMN IF NOT EXISTS last_read_by_user_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS last_read_by_admin_at TIMESTAMPTZ;

ALTER TABLE chat_messages
    ADD COLUMN IF NOT EXISTS kind VARCHAR(32) NOT NULL DEFAULT 'text',
    ADD COLUMN IF NOT EXISTS reply_to_id BIGINT,
    ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(128);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chat_messages_kind_check'
    ) THEN
        ALTER TABLE chat_messages
            ADD CONSTRAINT chat_messages_kind_check
            CHECK (kind IN ('text', 'image', 'sticker', 'balance_transfer')) NOT VALID;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chat_messages_reply_to_id_fkey'
    ) THEN
        ALTER TABLE chat_messages
            ADD CONSTRAINT chat_messages_reply_to_id_fkey
            FOREIGN KEY (reply_to_id) REFERENCES chat_messages(id) ON DELETE SET NULL NOT VALID;
    END IF;
END $$;

ALTER TABLE chat_messages VALIDATE CONSTRAINT chat_messages_kind_check;
ALTER TABLE chat_messages VALIDATE CONSTRAINT chat_messages_reply_to_id_fkey;

CREATE TABLE IF NOT EXISTS chat_quick_replies (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    admin_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title VARCHAR(100) NOT NULL,
    content VARCHAR(10000) NOT NULL,
    sort_order INT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_chat_quick_replies_admin_sort
    ON chat_quick_replies (admin_id, sort_order, id);

CREATE TABLE IF NOT EXISTS chat_assets (
    id BIGSERIAL PRIMARY KEY,
    scope VARCHAR(16) NOT NULL,
    conversation_id BIGINT REFERENCES chat_conversations(id) ON DELETE CASCADE,
    uploaded_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    name VARCHAR(255) NOT NULL,
    mime_type VARCHAR(64) NOT NULL,
    size INT NOT NULL,
    data BYTEA NOT NULL,
    collection VARCHAR(100) NOT NULL DEFAULT '',
    catalog_visible BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT chat_assets_scope_check CHECK (scope IN ('message', 'library', 'sticker')),
    CONSTRAINT chat_assets_owner_check CHECK (
        (scope = 'message' AND conversation_id IS NOT NULL)
        OR (scope IN ('library', 'sticker') AND conversation_id IS NULL)
    ),
    CONSTRAINT chat_assets_mime_check CHECK (mime_type IN ('image/png', 'image/jpeg')),
    CONSTRAINT chat_assets_size_check CHECK (
        size > 0 AND size <= 5242880 AND size = octet_length(data)
    )
);

CREATE INDEX IF NOT EXISTS idx_chat_assets_catalog
    ON chat_assets (scope, catalog_visible, created_at, id);
CREATE INDEX IF NOT EXISTS idx_chat_assets_conversation_created
    ON chat_assets (conversation_id, created_at);

CREATE TABLE IF NOT EXISTS chat_message_assets (
    message_id BIGINT NOT NULL REFERENCES chat_messages(id) ON DELETE CASCADE,
    asset_id BIGINT NOT NULL REFERENCES chat_assets(id) ON DELETE RESTRICT,
    sort_order INT NOT NULL DEFAULT 0,
    PRIMARY KEY (message_id, asset_id)
);

CREATE INDEX IF NOT EXISTS idx_chat_message_assets_asset_id
    ON chat_message_assets (asset_id);

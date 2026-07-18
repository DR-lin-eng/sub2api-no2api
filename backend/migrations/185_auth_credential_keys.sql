-- Durable RSA keys for browser credential encryption. PostgreSQL is the
-- authority; Redis only caches the immutable private_key value by key_id.

CREATE TABLE IF NOT EXISTS auth_credential_keys (
    key_id             VARCHAR(32) PRIMARY KEY,
    private_key        TEXT NOT NULL CHECK (private_key <> ''),
    slot_started_at    TIMESTAMPTZ NOT NULL,
    public_expires_at  TIMESTAMPTZ NOT NULL,
    decrypt_expires_at TIMESTAMPTZ NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT auth_credential_keys_public_window
        CHECK (public_expires_at = slot_started_at + INTERVAL '12 hours'),
    CONSTRAINT auth_credential_keys_decrypt_window
        CHECK (decrypt_expires_at = public_expires_at + INTERVAL '30 minutes')
);

CREATE INDEX IF NOT EXISTS idx_auth_credential_keys_decrypt_expires_at
    ON auth_credential_keys (decrypt_expires_at);

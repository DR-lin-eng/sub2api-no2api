-- Shared replay protection across nodes, with bounded lifetime.
CREATE TABLE IF NOT EXISTS site_migration_http_nonces (
    nonce bytea PRIMARY KEY,
    expires_at timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS site_migration_http_nonces_expiry ON site_migration_http_nonces(expires_at);

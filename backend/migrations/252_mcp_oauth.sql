-- OAuth clients use public-client authorization code + S256 PKCE. No client
-- secrets, authorization codes, access tokens, or refresh tokens are stored raw.
-- Client registration writes nothing: the client ID is signed and carries its
-- own metadata. Rows are created only when a user approves a client, and each
-- grant keeps the client name shown at consent.
CREATE TABLE IF NOT EXISTS mcp_oauth_grants (
    id VARCHAR(43) PRIMARY KEY,
    client_id TEXT NOT NULL,
    client_name VARCHAR(100) NOT NULL,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_version BIGINT NOT NULL,
    resource TEXT NOT NULL,
    scope VARCHAR(16) NOT NULL CHECK (scope IN ('user', 'admin')),
    read_only BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ,
    revoked_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_mcp_oauth_grants_user_created ON mcp_oauth_grants(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_mcp_oauth_grants_expiry ON mcp_oauth_grants(expires_at);

CREATE TABLE IF NOT EXISTS mcp_oauth_codes (
    code_hash VARCHAR(64) PRIMARY KEY,
    grant_id VARCHAR(43) NOT NULL REFERENCES mcp_oauth_grants(id) ON DELETE CASCADE,
    client_id TEXT NOT NULL,
    redirect_uri TEXT NOT NULL,
    resource TEXT NOT NULL,
    code_challenge VARCHAR(43) NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_mcp_oauth_codes_grant ON mcp_oauth_codes(grant_id);

-- Consumed refresh digests remain until the grant is pruned, allowing a replay
-- to revoke the entire family, including access tokens issued after rotation.
CREATE TABLE IF NOT EXISTS mcp_oauth_credentials (
    key_hash VARCHAR(64) PRIMARY KEY,
    grant_id VARCHAR(43) NOT NULL REFERENCES mcp_oauth_grants(id) ON DELETE CASCADE,
    kind VARCHAR(8) NOT NULL CHECK (kind IN ('access', 'refresh')),
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_mcp_oauth_credentials_grant ON mcp_oauth_credentials(grant_id);

-- All binaries, including pre-MCP nodes during rolling upgrade/rollback, revoke
-- grants when an owner loses authority. Authorization creation locks the same
-- user row and revalidates the snapshot before inserting a grant.
CREATE OR REPLACE FUNCTION revoke_mcp_grants_on_user_change() RETURNS TRIGGER AS $$
BEGIN
 IF NEW.status IS DISTINCT FROM OLD.status AND NEW.status <> 'active'
    OR NEW.deleted_at IS DISTINCT FROM OLD.deleted_at AND NEW.deleted_at IS NOT NULL
    OR NEW.password_hash IS DISTINCT FROM OLD.password_hash
    OR NEW.email IS DISTINCT FROM OLD.email THEN
  UPDATE mcp_oauth_grants SET revoked_at=COALESCE(revoked_at,CURRENT_TIMESTAMP)
  WHERE user_id=NEW.id AND revoked_at IS NULL;
 ELSIF NEW.role IS DISTINCT FROM OLD.role AND NEW.role <> 'admin' THEN
  UPDATE mcp_oauth_grants SET revoked_at=COALESCE(revoked_at,CURRENT_TIMESTAMP)
  WHERE user_id=NEW.id AND scope='admin' AND revoked_at IS NULL;
 ELSIF OLD.totp_enabled AND NOT NEW.totp_enabled THEN
  UPDATE mcp_oauth_grants SET revoked_at=COALESCE(revoked_at,CURRENT_TIMESTAMP)
  WHERE user_id=NEW.id AND scope='admin' AND NOT read_only AND revoked_at IS NULL;
 END IF;
 RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER trg_revoke_mcp_grants_on_user_change
 AFTER UPDATE OF status,role,password_hash,email,deleted_at,totp_enabled ON users
 FOR EACH ROW EXECUTE FUNCTION revoke_mcp_grants_on_user_change();

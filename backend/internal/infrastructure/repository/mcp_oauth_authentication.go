package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

func (r *mcpOAuthRepository) GetAuthentication(ctx context.Context, hash string) (*service.MCPAuthentication, error) {
	state := &service.MCPAuthentication{Credential: &service.MCPOAuthCredential{}, Grant: &service.MCPOAuthGrant{}, User: &service.User{}}
	c, g, u := state.Credential, state.Grant, state.User
	err := r.db.QueryRowContext(ctx, `SELECT c.key_hash,c.grant_id,c.kind,c.expires_at,c.used_at,
 g.id,g.client_id,g.client_name,g.user_id,g.token_version,g.resource,g.scope,g.read_only,g.created_at,g.expires_at,g.last_used_at,g.revoked_at,
 u.id,u.email,u.password_hash,u.role,u.status,u.concurrency,u.deleted_at,u.totp_enabled
 FROM mcp_oauth_credentials c JOIN mcp_oauth_grants g ON g.id=c.grant_id JOIN users u ON u.id=g.user_id
 WHERE c.key_hash=$1`, hash).Scan(
		&c.Hash, &c.GrantID, &c.Kind, &c.ExpiresAt, &c.UsedAt,
		&g.ID, &g.ClientID, &g.ClientName, &g.UserID, &g.TokenVersion, &g.Resource, &g.Scope, &g.ReadOnly, &g.CreatedAt, &g.ExpiresAt, &g.LastUsedAt, &g.RevokedAt,
		&u.ID, &u.Email, &u.PasswordHash, &u.Role, &u.Status, &u.Concurrency, &u.DeletedAt, &u.TotpEnabled)
	if err != nil {
		return nil, mcpOAuthSQLError(err)
	}
	return state, nil
}

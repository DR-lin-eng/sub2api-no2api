//go:build integration

package repository

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestMCPRollingOldUserWriterCannotReviveGrants(t *testing.T) {
	for _, mutation := range []string{"status", "email", "password_hash"} {
		t.Run(mutation, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			f := newMCPOAuthIntegrationFixture(t, ctx)
			tokens, err := f.oauth.Token(ctx, f.input, mcpIntegrationResource)
			require.NoError(t, err)
			// SQL models an old binary with no knowledge of MCP tables or revocation.
			switch mutation {
			case "status":
				_, err = integrationDB.ExecContext(ctx, `UPDATE users SET status='disabled' WHERE id=$1`, f.userID)
				require.NoError(t, err)
				_, err = integrationDB.ExecContext(ctx, `UPDATE users SET status='active' WHERE id=$1`, f.userID)
			case "email":
				_, err = integrationDB.ExecContext(ctx, `UPDATE users SET email='changed-'||email WHERE id=$1`, f.userID)
				require.NoError(t, err)
				_, err = integrationDB.ExecContext(ctx, `UPDATE users SET email=substring(email from 9) WHERE id=$1`, f.userID)
			case "password_hash":
				_, err = integrationDB.ExecContext(ctx, `UPDATE users SET password_hash='changed-'||password_hash WHERE id=$1`, f.userID)
				require.NoError(t, err)
				_, err = integrationDB.ExecContext(ctx, `UPDATE users SET password_hash=substring(password_hash from 9) WHERE id=$1`, f.userID)
			}
			require.NoError(t, err)
			grant, err := f.repo.GetGrant(ctx, f.grantID)
			require.NoError(t, err)
			require.NotNil(t, grant.RevokedAt)
			_, err = f.oauth.Authenticate(ctx, tokens.AccessToken)
			require.ErrorIs(t, err, service.ErrMCPOAuthInvalidToken)
		})
	}
}

func TestMCPAuthorizationRevalidatesLockedUser(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	f := newMCPOAuthIntegrationFixture(t, ctx)
	users := NewUserRepository(integrationEntClient, integrationDB)
	owner, err := users.GetByID(ctx, f.userID)
	require.NoError(t, err)
	grant, err := f.repo.GetGrant(ctx, f.grantID)
	require.NoError(t, err)
	grant.ID = "race-" + strings.Repeat("g", 25)
	code := &service.MCPOAuthCode{Hash: mcpIntegrationDigest(grant.ID), GrantID: grant.ID, ClientID: grant.ClientID, RedirectURI: f.input.RedirectURI, Resource: grant.Resource, CodeChallenge: strings.Repeat("c", 43), ExpiresAt: time.Now().Add(time.Minute)}
	_, err = integrationDB.ExecContext(ctx, `UPDATE users SET status='disabled' WHERE id=$1`, owner.ID)
	require.NoError(t, err)
	require.ErrorIs(t, f.repo.CreateAuthorization(ctx, grant, code, 20), service.ErrMCPOAuthInvalidToken)
	_, err = integrationDB.ExecContext(ctx, `UPDATE users SET status='active',password_hash='changed' WHERE id=$1`, owner.ID)
	require.NoError(t, err)
	require.ErrorIs(t, f.repo.CreateAuthorization(ctx, grant, code, 20), service.ErrMCPOAuthInvalidToken)
	var count int
	require.NoError(t, integrationDB.QueryRowContext(ctx, `SELECT COUNT(*) FROM mcp_oauth_grants WHERE id=$1`, grant.ID).Scan(&count))
	require.Zero(t, count)
}

func TestMCPAuthenticationJoinedReadAndTiming(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	f := newMCPOAuthIntegrationFixture(t, ctx)
	tokens, err := f.oauth.Token(ctx, f.input, mcpIntegrationResource)
	require.NoError(t, err)
	hash := mcpIntegrationDigest(tokens.AccessToken)
	reader, ok := f.repo.(*mcpOAuthRepository)
	require.True(t, ok)
	joined, err := reader.GetAuthentication(ctx, hash)
	require.NoError(t, err)
	require.Equal(t, f.userID, joined.User.ID)
	require.Equal(t, f.grantID, joined.Grant.ID)
	users := NewUserRepository(integrationEntClient, integrationDB)
	const n = 100
	for run := 0; run < 3; run++ {
		start := time.Now()
		for i := 0; i < n; i++ {
			_, err = reader.GetAuthentication(ctx, hash)
			require.NoError(t, err)
		}
		joinedTime := time.Since(start)
		start = time.Now()
		for i := 0; i < n; i++ {
			credential, err := reader.GetCredential(ctx, hash)
			require.NoError(t, err)
			grant, err := reader.GetGrant(ctx, credential.GrantID)
			require.NoError(t, err)
			_, err = users.GetByID(ctx, grant.UserID)
			require.NoError(t, err)
		}
		t.Logf("run=%d joined_one_sql=%s upstream_style_four_sql=%s calls=%d", run, joinedTime, time.Since(start), n)
	}
}

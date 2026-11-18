//go:build integration

package repository

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func (s *AccountRepoSuite) TestRevokeOpenAIWorkspaceAccountsIncludesAllGroupsAndShadows() {
	owner := mustCreateAccount(s.T(), s.client, &service.Account{Name: "workspace-owner", Platform: service.PlatformOpenAI, Credentials: map[string]any{"chatgpt_account_id": "workspace-a"}})
	peer := mustCreateAccount(s.T(), s.client, &service.Account{Name: "workspace-peer", Platform: service.PlatformOpenAI, Credentials: map[string]any{"chatgpt_account_id": "workspace-a"}})
	shadow := mustCreateAccount(s.T(), s.client, &service.Account{Name: "workspace-shadow", Platform: service.PlatformOpenAI, ParentAccountID: &peer.ID, QuotaDimension: service.QuotaDimensionSpark})
	other := mustCreateAccount(s.T(), s.client, &service.Account{Name: "other-workspace", Platform: service.PlatformOpenAI, Credentials: map[string]any{"chatgpt_account_id": "workspace-b"}})
	apiKey := mustCreateAccount(s.T(), s.client, &service.Account{Name: "workspace-api-key", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"chatgpt_account_id": "workspace-a"}})
	anthropic := mustCreateAccount(s.T(), s.client, &service.Account{Name: "workspace-other-platform", Platform: service.PlatformAnthropic, Credentials: map[string]any{"chatgpt_account_id": "workspace-a"}})
	missingID := mustCreateAccount(s.T(), s.client, &service.Account{Name: "missing-workspace", Platform: service.PlatformOpenAI})
	for index, accountID := range []int64{owner.ID, peer.ID} {
		group := mustCreateGroup(s.T(), s.client, &service.Group{Name: []string{"workspace-group-a", "workspace-group-b"}[index], Platform: service.PlatformOpenAI})
		mustBindAccountToGroup(s.T(), s.client, accountID, group.ID, 1)
	}
	cache := &schedulerCacheRecorder{}
	s.repo.schedulerCache = cache
	_, err := s.repo.sql.ExecContext(s.ctx, "DELETE FROM scheduler_outbox WHERE account_id IN ($1, $2, $3)", owner.ID, peer.ID, shadow.ID)
	s.Require().NoError(err)

	ids, err := s.repo.RevokeOpenAIWorkspaceAccounts(s.ctx, " workspace-a ", "Workspace deactivated (402)")

	s.Require().NoError(err)
	s.Require().ElementsMatch([]int64{owner.ID, peer.ID, shadow.ID}, ids)
	s.Require().Len(cache.setAccounts, 3)
	for _, id := range ids {
		account, err := s.repo.GetByID(s.ctx, id)
		s.Require().NoError(err)
		s.Require().Equal(service.StatusError, account.Status)
		s.Require().False(account.Schedulable)
		s.Require().Equal("Workspace deactivated (402)", account.ErrorMessage)
		var count int
		s.Require().NoError(scanSingleRow(s.ctx, s.repo.sql, "SELECT COUNT(*) FROM scheduler_outbox WHERE account_id = $1 AND event_type = $2", []any{id, service.SchedulerOutboxEventAccountChanged}, &count))
		s.Require().Equal(1, count)
	}
	for _, id := range []int64{other.ID, apiKey.ID, anthropic.ID, missingID.ID} {
		account, err := s.repo.GetByID(s.ctx, id)
		s.Require().NoError(err)
		s.Require().Equal(service.StatusActive, account.Status)
		s.Require().True(account.Schedulable)
	}
}

func TestRevokeOpenAIWorkspaceAccountsPreservesConcurrentWorkspaceChange(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := testEntClient(t)
	account := mustCreateAccount(t, client, &service.Account{Name: "workspace-revocation-concurrent", Platform: service.PlatformOpenAI, Credentials: map[string]any{"chatgpt_account_id": "workspace-old"}})
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM accounts WHERE id = $1", account.ID)
	})
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `UPDATE accounts SET credentials = '{"chatgpt_account_id":"workspace-new"}'::jsonb WHERE id = $1`, account.ID)
	require.NoError(t, err)
	repo := newAccountRepositoryWithSQL(client, integrationDB, nil)
	type result struct {
		ids []int64
		err error
	}
	done := make(chan result, 1)
	go func() {
		ids, err := repo.RevokeOpenAIWorkspaceAccounts(ctx, "workspace-old", "workspace revoked")
		done <- result{ids, err}
	}()
	require.Eventually(t, func() bool {
		var waiting bool
		err := integrationDB.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_stat_activity WHERE pid <> pg_backend_pid()
			AND wait_event_type = 'Lock' AND query LIKE '%WITH workspace_owners AS MATERIALIZED%'
		)`).Scan(&waiting)
		return err == nil && waiting
	}, 5*time.Second, 20*time.Millisecond, "revocation must wait for the concurrent credential update")
	require.NoError(t, tx.Commit())
	completed := <-done
	require.NoError(t, completed.err)
	require.Empty(t, completed.ids, "the workspace predicate must be rechecked after the row lock")
	got, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, "workspace-new", got.GetChatGPTAccountID())
	require.Equal(t, service.StatusActive, got.Status)
	require.True(t, got.Schedulable)
}

type failWorkspaceOutboxExecutor struct{ sqlExecutor }

func (e *failWorkspaceOutboxExecutor) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	if strings.Contains(query, "INSERT INTO scheduler_outbox") && len(args) > 0 {
		args = append([]any(nil), args...)
		args[len(args)-1] = nil
	}
	return e.sqlExecutor.QueryContext(ctx, query, args...)
}

func TestRevokeOpenAIWorkspaceAccountsRollsBackWhenOutboxFails(t *testing.T) {
	client := testEntClient(t)
	account := mustCreateAccount(t, client, &service.Account{Name: "workspace-revocation-atomic", Platform: service.PlatformOpenAI, Credentials: map[string]any{"chatgpt_account_id": "workspace-atomic"}})
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM scheduler_outbox WHERE account_id = $1", account.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM accounts WHERE id = $1", account.ID)
	})
	repo := newAccountRepositoryWithSQL(client, &failWorkspaceOutboxExecutor{sqlExecutor: integrationDB}, nil)
	ids, err := repo.RevokeOpenAIWorkspaceAccounts(t.Context(), "workspace-atomic", "workspace revoked")
	require.Error(t, err)
	require.Empty(t, ids)
	got, err := repo.GetByID(t.Context(), account.ID)
	require.NoError(t, err)
	require.Equal(t, service.StatusActive, got.Status)
	require.True(t, got.Schedulable)
}

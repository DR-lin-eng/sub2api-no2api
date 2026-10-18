//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestMergeUpstreamModelMappingPreservesCurrentMappingAndCredentials(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "model-sync", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "fixture-key", "base_url": "https://upstream.example/v1", "model_mapping": map[string]any{"existing": "custom-target"}},
		Extra:       map[string]any{"keep": "value"},
	})
	// A mapping edit made after the probe must take precedence over its result.
	_, err := tx.ExecContext(ctx, `UPDATE accounts SET credentials = jsonb_set(credentials, '{model_mapping}', credentials->'model_mapping' || '{"new-model":"edited-target","manual":"manual-target"}'::jsonb) WHERE id = $1`, account.ID)
	require.NoError(t, err)
	added, err := repo.MergeUpstreamModelMapping(ctx, account, []string{"existing", "new-model", "added-model"})
	require.NoError(t, err)
	require.Equal(t, 1, added)
	updated, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, map[string]string{"existing": "custom-target", "new-model": "edited-target", "manual": "manual-target", "added-model": "added-model"}, updated.GetModelMapping())
	require.Equal(t, "fixture-key", updated.GetCredential("api_key"))
	require.Equal(t, "https://upstream.example/v1", updated.GetCredential("base_url"))
	require.Equal(t, "value", updated.Extra["keep"])
	added, err = repo.MergeUpstreamModelMapping(ctx, updated, []string{"added-model", "new-model"})
	require.NoError(t, err)
	require.Zero(t, added)
	rows, err := tx.QueryContext(ctx, `SELECT COUNT(*) FROM scheduler_outbox WHERE account_id = $1 AND event_type = $2`, account.ID, service.SchedulerOutboxEventAccountChanged)
	require.NoError(t, err)
	defer rows.Close()
	require.True(t, rows.Next())
	var count int
	require.NoError(t, rows.Scan(&count))
	require.GreaterOrEqual(t, count, 2)
}

func TestMergeUpstreamModelMappingRejectsChangedCredentials(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "model-sync-stale", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Credentials: map[string]any{"access_token": "old-token", "model_mapping": map[string]any{"original": "original"}},
	})
	_, err := tx.ExecContext(ctx, `UPDATE accounts SET credentials = jsonb_set(credentials, '{access_token}', '"refreshed-token"'::jsonb) WHERE id = $1`, account.ID)
	require.NoError(t, err)
	_, err = repo.MergeUpstreamModelMapping(ctx, account, []string{"stale-result"})
	require.ErrorIs(t, err, service.ErrUpstreamModelSyncAccountChanged)
	updated, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Equal(t, "refreshed-token", updated.GetCredential("access_token"))
	require.Equal(t, map[string]string{"original": "original"}, updated.GetModelMapping())
}

func TestMergeUpstreamModelMappingOutboxFailureRollsBackModels(t *testing.T) {
	ctx := context.Background()
	tx := testEntTx(t)
	repo := newAccountRepositoryWithSQL(tx.Client(), tx, nil)
	account := mustCreateAccount(t, tx.Client(), &service.Account{
		Name: "model-sync-outbox", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "key"},
	})
	_, err := tx.ExecContext(ctx, `CREATE FUNCTION pg_temp.reject_model_sync_outbox() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'fixture outbox failure'; END $$`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `CREATE TRIGGER reject_model_sync_outbox BEFORE INSERT ON scheduler_outbox FOR EACH ROW EXECUTE FUNCTION pg_temp.reject_model_sync_outbox()`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `SAVEPOINT model_sync_savepoint`)
	require.NoError(t, err)
	_, err = repo.MergeUpstreamModelMapping(ctx, account, []string{"not-committed"})
	require.Error(t, err)
	_, err = tx.ExecContext(ctx, `ROLLBACK TO SAVEPOINT model_sync_savepoint`)
	require.NoError(t, err)
	updated, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotContains(t, updated.GetModelMapping(), "not-committed")
	require.Equal(t, "key", updated.GetCredential("api_key"))
}

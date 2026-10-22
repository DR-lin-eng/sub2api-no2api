//go:build integration

package repository

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/modules/modelcatalog"
	"github.com/stretchr/testify/require"
)

func TestModelCatalogPostgresRefreshAndLegacyWrites(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newAccountRepositoryWithSQL(client, nil, nil)
	account := &service.Account{Name: "catalog-generation-fixture", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth, Status: service.StatusActive, Credentials: map[string]any{"access_token": "fixture-only"}, Extra: map[string]any{"other": "preserve"}, Concurrency: 1, Priority: 50, Schedulable: true}
	require.NoError(t, repo.Create(ctx, account))
	t.Cleanup(func() { _ = repo.Delete(ctx, account.ID) })
	staleEdit, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	base := time.Now().Add(-time.Hour)
	snapshot := func(at time.Time, id string) *modelcatalog.Snapshot {
		c := &modelcatalog.Catalog{Models: []string{id}, Metadata: map[string]modelcatalog.Descriptor{id: {"context_window": json.RawMessage(`777777`)}}}
		s, err := c.Snapshot("fixture", at)
		require.NoError(t, err)
		return s
	}
	old, newer := snapshot(base, "old"), snapshot(base.Add(time.Second), "new")
	require.NoError(t, repo.UpdateUpstreamModelCatalog(ctx, account, old))
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, s := range []*modelcatalog.Snapshot{newer, old} {
		wg.Add(1)
		go func(s *modelcatalog.Snapshot) {
			defer wg.Done()
			errs <- repo.UpdateUpstreamModelCatalog(ctx, account, s)
		}(s)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			require.ErrorIs(t, err, service.ErrUpstreamModelCatalogChanged)
		}
	}
	loaded, err := repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Contains(t, modelcatalog.ReadSnapshot(loaded.Extra).Models, "new")
	require.Equal(t, "preserve", loaded.Extra["other"])
	// Simulate the old binary replacing extra from a stale cache. This is a
	// raw SQL replacement: it deliberately bypasses the new repository guard.
	_, err = client.ExecContext(ctx, `UPDATE accounts SET extra = '{"legacy_edit":true}'::jsonb WHERE id=$1`, account.ID)
	require.NoError(t, err)
	loaded, err = repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Contains(t, modelcatalog.ReadSnapshot(loaded.Extra).Models, "new")
	require.Equal(t, true, loaded.Extra["legacy_edit"])
	staleJSON, err := json.Marshal(map[string]any{modelcatalog.ExtraKey: old})
	require.NoError(t, err)
	_, err = client.ExecContext(ctx, `UPDATE accounts SET extra=$2::jsonb WHERE id=$1`, account.ID, string(staleJSON))
	require.NoError(t, err)
	loaded, err = repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Contains(t, modelcatalog.ReadSnapshot(loaded.Extra).Models, "new")
	// The old background writer knows only the ID list and merges its fields.
	require.NoError(t, repo.UpdateExtra(ctx, account.ID, map[string]any{service.OAuthSupportedModelsExtraKey: []string{"legacy-id"}}))
	staleEdit.Name = "ordinary stale edit"
	require.NoError(t, repo.Update(ctx, staleEdit))
	loaded, err = repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.Contains(t, modelcatalog.ReadSnapshot(loaded.Extra).Models, "new")
	// A future snapshot is opaque to this version and survives ordinary edits.
	future := map[string]any{"version": 99, "generation": 9999999999999999, "models": map[string]any{}}
	require.NoError(t, repo.UpdateExtra(ctx, account.ID, map[string]any{modelcatalog.ExtraKey: future}))
	require.ErrorIs(t, repo.UpdateUpstreamModelCatalog(ctx, account, snapshot(time.Now(), "replacement")), service.ErrUpstreamModelCatalogChanged)
	_, err = client.ExecContext(ctx, `UPDATE accounts SET extra=$2::jsonb WHERE id=$1`, account.ID, string(staleJSON))
	require.NoError(t, err)
	require.NoError(t, repo.Update(ctx, staleEdit))
	loaded, err = repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.EqualValues(t, 99, loaded.Extra[modelcatalog.ExtraKey].(map[string]any)["version"])
	loaded.Credentials = map[string]any{"access_token": "different-fixture"}
	require.NoError(t, repo.Update(ctx, loaded))
	loaded, err = repo.GetByID(ctx, account.ID)
	require.NoError(t, err)
	require.NotContains(t, loaded.Extra, modelcatalog.ExtraKey)
	require.ErrorIs(t, repo.UpdateUpstreamModelCatalog(ctx, account, snapshot(time.Now(), "stale-identity")), service.ErrUpstreamModelCatalogChanged)
}

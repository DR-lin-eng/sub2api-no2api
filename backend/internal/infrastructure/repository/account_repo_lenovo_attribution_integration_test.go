//go:build integration

package repository

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/modules/lenovoattribution"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestLenovoAttributionPostgresCASRestartAndStaleEdits(t *testing.T) {
	ctx := t.Context()
	client := testEntClient(t)
	first := newAccountRepositoryWithSQL(client, integrationDB, nil)
	second := newAccountRepositoryWithSQL(client, integrationDB, nil)
	a := &service.Account{Name: "lenovo-fixture", Platform: service.PlatformOpenAI, Type: service.AccountTypeOAuth,
		Status: service.StatusActive, Credentials: map[string]any{"access_token": "synthetic-token"},
		Extra: map[string]any{"keep": true}, Concurrency: 1, Schedulable: true}
	require.NoError(t, first.Create(ctx, a))
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		require.NoError(t, first.Delete(cleanupCtx, a.ID))
	})
	stale, err := first.GetByID(ctx, a.ID)
	require.NoError(t, err)
	var wg sync.WaitGroup
	winners := make(chan *lenovoattribution.State, 16)
	for range 16 {
		wg.Go(func() {
			s := &lenovoattribution.State{Version: 1, Binding: "principal", InstallationID: uuid.NewString(), LeaseID: uuid.NewString(), LeaseUntil: time.Now().Add(time.Minute).UTC()}
			won, e := second.CompareAndSwapOpenAILenovoAttribution(ctx, stale, nil, s)
			if e != nil {
				t.Error(e)
			}
			if won {
				winners <- s
			}
		})
	}
	wg.Wait()
	close(winners)
	require.Len(t, winners, 1, "only one replica may reserve the installation/request")
	winner := <-winners
	current, err := first.GetByID(ctx, a.ID)
	require.NoError(t, err)
	stored, err := lenovoattribution.Read(current.Extra)
	require.NoError(t, err)
	require.Equal(t, winner.InstallationID, stored.InstallationID)
	completed := stored.Complete(204, nil, false, time.Now())
	_, err = client.ExecContext(ctx, `UPDATE accounts SET extra=extra || '{"concurrent_other":true}'::jsonb WHERE id=$1`, a.ID)
	require.NoError(t, err)
	won, err := first.CompareAndSwapOpenAILenovoAttribution(ctx, current, stored, &completed)
	require.NoError(t, err)
	require.True(t, won)
	won, err = second.CompareAndSwapOpenAILenovoAttribution(ctx, current, stored, winner)
	require.NoError(t, err)
	require.False(t, won, "late worker must not replace the final state")
	stale.Name = "edited from stale form"
	require.NoError(t, second.Update(ctx, stale))
	// Generic patches cannot import another account's generated runtime state.
	require.NoError(t, first.UpdateExtra(ctx, a.ID, map[string]any{lenovoattribution.ExtraKey: winner, "unrelated": true}))
	restarted := newAccountRepositoryWithSQL(client, integrationDB, nil)
	loaded, err := restarted.GetByID(ctx, a.ID)
	require.NoError(t, err)
	state, err := lenovoattribution.Read(loaded.Extra)
	require.NoError(t, err)
	require.Equal(t, completed.InstallationID, state.InstallationID)
	require.True(t, state.Terminal)
	require.Equal(t, 204, state.StatusCode)
	require.Equal(t, true, loaded.Extra["unrelated"])
	_, err = client.ExecContext(ctx, `UPDATE accounts SET credentials='{"access_token":"changed"}'::jsonb WHERE id=$1`, a.ID)
	require.NoError(t, err)
	won, err = first.CompareAndSwapOpenAILenovoAttribution(ctx, current, &completed, winner)
	require.NoError(t, err)
	require.False(t, won, "credential change rejects stale receipts")
}

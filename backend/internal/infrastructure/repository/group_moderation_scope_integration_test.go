//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestContentModerationConfigWriterRechecksGroupAfterConcurrentDelete(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	repo := newGroupRepositoryWithSQL(client, integrationDB)
	settings := &settingRepository{client: client}
	group := &service.Group{
		Name: "moderation-concurrent-delete",
		Platform: service.PlatformAnthropic,
		RateMultiplier: 1,
		Status: service.StatusActive,
		SubscriptionType: service.SubscriptionTypeStandard,
	}
	require.NoError(t, repo.Create(ctx, group))
	t.Cleanup(func() { _, _ = integrationDB.ExecContext(ctx, "DELETE FROM groups WHERE id=$1", group.ID) })

	key := service.SettingKeyContentModerationConfig
	previous, previousErr := settings.GetValue(ctx, key)
	t.Cleanup(func() {
		if previousErr == nil {
			_ = settings.Set(ctx, key, previous)
		} else {
			_ = settings.Delete(ctx, key)
		}
	})
	require.NoError(t, settings.Set(ctx, key, `{"all_groups":false,"group_ids":[]}`))

	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	var lockedID int64
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT id FROM groups WHERE id=$1 FOR UPDATE", group.ID).Scan(&lockedID))
	finished := make(chan error, 1)
	go func() {
		finished <- settings.SetContentModerationConfig(ctx,
			fmt.Sprintf(`{"all_groups":false,"group_ids":[%d]}`, group.ID), []int64{group.ID})
	}()
	select {
	case err := <-finished:
		t.Fatalf("writer passed the deleter lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	_, err = tx.ExecContext(ctx, "UPDATE groups SET deleted_at=NOW() WHERE id=$1", group.ID)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())
	select {
	case err := <-finished:
		require.ErrorIs(t, err, service.ErrGroupNotFound)
	case <-time.After(5 * time.Second):
		t.Fatal("writer did not finish after delete commit")
	}
	stored, err := settings.GetValue(ctx, key)
	require.NoError(t, err)
	require.NotContains(t, stored, fmt.Sprintf("%d", lockedID))
}

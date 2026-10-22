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
		Name:             "moderation-concurrent-delete",
		Platform:         service.PlatformAnthropic,
		RateMultiplier:   1,
		Status:           service.StatusActive,
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
	defer func() { _ = tx.Rollback() }()
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

func TestGroupDeleteConcurrentCallsUseWriteLock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := testEntClient(t)
	repo := newGroupRepositoryWithSQL(client, integrationDB)
	group := &service.Group{Name: "delete-concurrent", Platform: service.PlatformAnthropic,
		RateMultiplier: 1, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard}
	require.NoError(t, repo.Create(ctx, group))
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM groups WHERE id=$1", group.ID)
	})
	// Hold both calls behind a write lock so they contend for the same row.
	tx, err := integrationDB.BeginTx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	var id int64
	require.NoError(t, tx.QueryRowContext(ctx, "SELECT id FROM groups WHERE id=$1 FOR UPDATE", group.ID).Scan(&id))
	results := make(chan error, 2)
	started := make(chan struct{}, 2)
	for range 2 {
		go func() { started <- struct{}{}; results <- repo.Delete(ctx, group.ID) }()
	}
	<-started
	<-started
	require.NoError(t, tx.Commit())
	first, second := <-results, <-results
	if first != nil {
		first, second = second, first
	}
	require.NoError(t, first)
	require.ErrorIs(t, second, service.ErrGroupNotFound)
}

func (s *GroupRepoSuite) TestI18nPreservedAndCleared() {
	group := &service.Group{Name: "i18n-persist", Platform: service.PlatformAnthropic,
		RateMultiplier: 1, Status: service.StatusActive, SubscriptionType: service.SubscriptionTypeStandard,
		I18n: service.GroupI18n{"en": {Name: "English", Description: "English description"}}}
	s.Require().NoError(s.repo.Create(s.ctx, group))
	stored, err := s.repo.GetByID(s.ctx, group.ID)
	s.Require().NoError(err)
	s.Require().Equal(group.I18n, stored.I18n)
	stored.Name = "old-client-default-edit"
	s.Require().NoError(s.repo.Update(s.ctx, stored))
	stored, err = s.repo.GetByID(s.ctx, group.ID)
	s.Require().NoError(err)
	s.Require().Equal("English", stored.I18n["en"].Name)
	stored.I18n = service.GroupI18n{}
	s.Require().NoError(s.repo.Update(s.ctx, stored))
	stored, err = s.repo.GetByID(s.ctx, group.ID)
	s.Require().NoError(err)
	s.Require().Empty(stored.I18n)
}

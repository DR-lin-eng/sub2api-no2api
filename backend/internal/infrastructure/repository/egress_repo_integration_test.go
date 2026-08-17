//go:build integration

package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	moduleegress "github.com/Wei-Shaw/sub2api/internal/modules/egress"
	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
	"github.com/stretchr/testify/require"
)

func TestEgressRepositoryPersistsRotationAndReleasesSoftDeletedAccount(t *testing.T) {
	ctx := context.Background()
	accountA := createEgressIntegrationAccount(t, ctx, "ipv6-egress-a")
	accountB := createEgressIntegrationAccount(t, ctx, "ipv6-egress-b")
	poolCIDR := fmt.Sprintf("fd42:%x:%x::/64", accountA.ID&0xffff, accountB.ID&0xffff)

	store := NewEgressStore(integrationEntClient, integrationDB, nil)
	pool, err := store.CreatePool(ctx, moduleegress.CreatePoolInput{
		Name: fmt.Sprintf("ipv6-egress-%d-%d", accountA.ID, accountB.ID),
		CIDR: poolCIDR,
	})
	require.NoError(t, err)
	require.Equal(t, "18446744073709551615", pool.Capacity)
	_, err = store.CreatePool(ctx, moduleegress.CreatePoolInput{
		Name: fmt.Sprintf("ipv6-egress-overlap-%d-%d", accountA.ID, accountB.ID),
		CIDR: strings.TrimSuffix(poolCIDR, "/64") + "/80",
	})
	require.ErrorIs(t, err, moduleegress.ErrPoolOverlap)

	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM account_egress_bindings WHERE account_id IN ($1, $2)", accountA.ID, accountB.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM ipv6_egress_pools WHERE id = $1", pool.ID)
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM accounts WHERE id IN ($1, $2)", accountA.ID, accountB.ID)
	})

	addressA := fmt.Sprintf("fd42:%x:%x::10", accountA.ID&0xffff, accountB.ID&0xffff)
	binding, err := store.UpsertBinding(ctx, moduleegress.Binding{
		AccountID:  accountA.ID,
		PoolID:     pool.ID,
		SourceIPv6: addressA,
		Status:     moduleegress.BindingStatusActive,
		Version:    1,
	}, nil)
	require.NoError(t, err)
	require.Equal(t, int64(1), binding.Version)
	require.Equal(t, addressA, binding.SourceIPv6)

	_, err = store.UpsertBinding(ctx, moduleegress.Binding{
		AccountID:  accountB.ID,
		PoolID:     pool.ID,
		SourceIPv6: addressA,
		Status:     moduleegress.BindingStatusActive,
		Version:    1,
	}, nil)
	require.ErrorIs(t, err, moduleegress.ErrAddressConflict)

	expectedVersion := int64(1)
	addressB := fmt.Sprintf("fd42:%x:%x::11", accountA.ID&0xffff, accountB.ID&0xffff)
	rotated, err := store.UpsertBinding(ctx, moduleegress.Binding{
		AccountID:  accountA.ID,
		PoolID:     pool.ID,
		SourceIPv6: addressB,
		Status:     moduleegress.BindingStatusActive,
		Version:    2,
	}, &expectedVersion)
	require.NoError(t, err)
	require.Equal(t, int64(2), rotated.Version)
	require.Equal(t, addressB, rotated.SourceIPv6)

	_, err = store.UpsertBinding(ctx, moduleegress.Binding{
		AccountID:  accountA.ID,
		PoolID:     pool.ID,
		SourceIPv6: addressA,
		Status:     moduleegress.BindingStatusActive,
		Version:    2,
	}, &expectedVersion)
	require.ErrorIs(t, err, moduleegress.ErrBindingChanged)

	require.NoError(t, store.SetAccountMode(ctx, accountA.ID, platformegress.ModeIPv6Pool))
	accountRepo := newAccountRepositoryWithSQL(integrationEntClient, integrationDB, nil)
	require.NoError(t, accountRepo.Delete(ctx, accountA.ID))
	_, err = store.GetBinding(ctx, accountA.ID)
	require.ErrorIs(t, err, moduleegress.ErrBindingNotFound)
	require.NoError(t, store.DeletePool(ctx, pool.ID))
}

func createEgressIntegrationAccount(t *testing.T, ctx context.Context, name string) *dbent.Account {
	t.Helper()
	account, err := integrationEntClient.Account.Create().
		SetName(name).
		SetPlatform("openai").
		SetType("api_key").
		SetCredentials(map[string]any{"api_key": "test"}).
		SetExtra(map[string]any{}).
		Save(ctx)
	require.NoError(t, err)
	return account
}

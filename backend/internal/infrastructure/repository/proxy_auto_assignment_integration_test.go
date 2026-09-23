//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

type proxyAutoAssignmentIntegrationFixture struct {
	ctx  context.Context
	tx   *dbent.Tx
	repo *proxyRepository
}

func newProxyAutoAssignmentIntegrationFixture(t *testing.T) *proxyAutoAssignmentIntegrationFixture {
	t.Helper()
	tx := testEntTx(t)
	return &proxyAutoAssignmentIntegrationFixture{
		ctx:  context.Background(),
		tx:   tx,
		repo: newProxyRepositoryWithSQL(tx.Client(), tx),
	}
}

func (f *proxyAutoAssignmentIntegrationFixture) createProxy(t *testing.T, name string) int64 {
	t.Helper()
	p := &service.Proxy{
		Name:           name,
		Protocol:       "http",
		Host:           "127.0.0.1",
		Port:           8000,
		Status:         service.StatusActive,
		FallbackMode:   service.FallbackModeNone,
		ExpiryWarnDays: 7,
	}
	require.NoError(t, f.repo.Create(f.ctx, p))
	return p.ID
}

func (f *proxyAutoAssignmentIntegrationFixture) createParentAccount(t *testing.T, name string, proxyID *int64) int64 {
	t.Helper()
	var id int64
	err := scanSingleRow(f.ctx, f.tx, `
		INSERT INTO accounts (name, platform, type, credentials, extra, status, proxy_id, created_at, updated_at)
		VALUES ($1, 'openai', 'oauth', '{}', '{}', 'active', $2, NOW(), NOW())
		RETURNING id
	`, []any{name, proxyID}, &id)
	require.NoError(t, err)
	return id
}

func (f *proxyAutoAssignmentIntegrationFixture) createShadow(t *testing.T, parentID int64, proxyID *int64) int64 {
	t.Helper()
	var id int64
	err := scanSingleRow(f.ctx, f.tx, `
		INSERT INTO accounts (
			name, platform, type, credentials, extra, status, proxy_id,
			parent_account_id, quota_dimension, created_at, updated_at
		)
		VALUES ($1, 'openai', 'oauth', '{}', '{}', 'active', $2, $3, 'spark', NOW(), NOW())
		RETURNING id
	`, []any{"shadow", proxyID, parentID}, &id)
	require.NoError(t, err)
	return id
}

func TestProxyAutoAssignmentRebalancesParentsAndKeepsShadowsTogether(t *testing.T) {
	f := newProxyAutoAssignmentIntegrationFixture(t)
	proxyIDs := []int64{
		f.createProxy(t, "pool-a"),
		f.createProxy(t, "pool-b"),
		f.createProxy(t, "pool-c"),
	}
	parents := make([]int64, 0, 8)
	for i := 0; i < 8; i++ {
		parents = append(parents, f.createParentAccount(t, "parent-"+time.Now().Add(time.Duration(i)*time.Nanosecond).Format("150405.000000000"), nil))
	}
	wrongProxyID := proxyIDs[2]
	shadowID := f.createShadow(t, parents[0], &wrongProxyID)

	changed, err := f.repo.RebalanceActiveProxyAccounts(f.ctx, time.Now())
	require.NoError(t, err)
	require.EqualValues(t, 9, changed)

	counts := make([]int64, 0, len(proxyIDs))
	for _, proxyID := range proxyIDs {
		var count int64
		require.NoError(t, scanSingleRow(f.ctx, f.tx, `
			SELECT COUNT(*) FROM accounts
			WHERE proxy_id = $1 AND parent_account_id IS NULL AND deleted_at IS NULL
		`, []any{proxyID}, &count))
		counts = append(counts, count)
	}
	require.ElementsMatch(t, []int64{2, 3, 3}, counts)

	var parentProxyID, shadowProxyID int64
	require.NoError(t, scanSingleRow(f.ctx, f.tx, `SELECT proxy_id FROM accounts WHERE id = $1`, []any{parents[0]}, &parentProxyID))
	require.NoError(t, scanSingleRow(f.ctx, f.tx, `SELECT proxy_id FROM accounts WHERE id = $1`, []any{shadowID}, &shadowProxyID))
	require.Equal(t, parentProxyID, shadowProxyID)

	wantRotation := []int64{proxyIDs[2], proxyIDs[0], proxyIDs[1], proxyIDs[2]}
	for _, wantProxyID := range wantRotation {
		selected, err := f.repo.SelectLeastLoadedActiveProxy(f.ctx, time.Now())
		require.NoError(t, err)
		require.Equal(t, wantProxyID, *selected)
	}
}

func TestProxyHealthThresholdDeactivatesAndMigrates(t *testing.T) {
	f := newProxyAutoAssignmentIntegrationFixture(t)
	failedProxyID := f.createProxy(t, "failed")
	healthyProxyID := f.createProxy(t, "healthy")
	for i := 0; i < 4; i++ {
		proxyID := failedProxyID
		if i == 3 {
			proxyID = healthyProxyID
		}
		f.createParentAccount(t, "health-parent-"+time.Now().Add(time.Duration(i)*time.Nanosecond).Format("150405.000000000"), &proxyID)
	}

	deactivated, err := f.repo.RecordProxyHealthCheck(f.ctx, failedProxyID, time.Now(), false, "probe_failed", 2)
	require.NoError(t, err)
	require.False(t, deactivated)
	first, err := f.repo.GetByID(f.ctx, failedProxyID)
	require.NoError(t, err)
	require.Equal(t, service.ProxyHealthDegraded, first.HealthStatus)
	require.Equal(t, 1, first.HealthConsecutiveFailures)
	require.Equal(t, service.StatusActive, first.Status)

	deactivated, err = f.repo.RecordProxyHealthCheck(f.ctx, failedProxyID, time.Now(), false, "probe_failed", 2)
	require.NoError(t, err)
	require.True(t, deactivated)
	second, err := f.repo.GetByID(f.ctx, failedProxyID)
	require.NoError(t, err)
	require.Equal(t, service.ProxyHealthUnhealthy, second.HealthStatus)
	require.Equal(t, service.ProxyStatusInactive, second.Status)

	changed, err := f.repo.RebalanceActiveProxyAccounts(f.ctx, time.Now())
	require.NoError(t, err)
	require.EqualValues(t, 3, changed)
	var failedAssignments, healthyAssignments int64
	require.NoError(t, scanSingleRow(f.ctx, f.tx, `SELECT COUNT(*) FROM accounts WHERE proxy_id = $1`, []any{failedProxyID}, &failedAssignments))
	require.NoError(t, scanSingleRow(f.ctx, f.tx, `SELECT COUNT(*) FROM accounts WHERE proxy_id = $1`, []any{healthyProxyID}, &healthyAssignments))
	require.Zero(t, failedAssignments)
	require.EqualValues(t, 4, healthyAssignments)
}

func TestProxyHealthSuccessResetsFailureState(t *testing.T) {
	f := newProxyAutoAssignmentIntegrationFixture(t)
	proxyID := f.createProxy(t, "recovering")
	deactivated, err := f.repo.RecordProxyHealthCheck(f.ctx, proxyID, time.Now(), false, "probe_failed", 3)
	require.NoError(t, err)
	require.False(t, deactivated)

	deactivated, err = f.repo.RecordProxyHealthCheck(f.ctx, proxyID, time.Now(), true, "", 3)
	require.NoError(t, err)
	require.False(t, deactivated)
	proxy, err := f.repo.GetByID(f.ctx, proxyID)
	require.NoError(t, err)
	require.Equal(t, service.ProxyHealthHealthy, proxy.HealthStatus)
	require.Zero(t, proxy.HealthConsecutiveFailures)
	require.Empty(t, proxy.LastHealthError)
	require.NotNil(t, proxy.LastHealthCheckAt)
}

func TestProxyAutoAssignmentRejectsEmptyPool(t *testing.T) {
	f := newProxyAutoAssignmentIntegrationFixture(t)
	_, err := f.repo.SelectLeastLoadedActiveProxy(f.ctx, time.Now())
	require.True(t, errors.Is(err, service.ErrProxyAutoAssignmentNoActiveProxy))
	_, err = f.repo.RebalanceActiveProxyAccounts(f.ctx, time.Now())
	require.True(t, errors.Is(err, service.ErrProxyAutoAssignmentNoActiveProxy))
}

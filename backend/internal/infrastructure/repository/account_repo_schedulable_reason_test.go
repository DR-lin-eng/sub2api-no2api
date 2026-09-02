package repository

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestAccountRepositorySetSchedulableWithReasonIsAtomic(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	err := repo.SetSchedulableWithReason(context.Background(), 42, false, "consecutive 429/502 threshold reached")

	require.NoError(t, err)
	require.Len(t, exec.execQueries, 1)
	normalized := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, normalized, "WITH updated AS")
	require.Contains(t, normalized, "schedulable = $1")
	require.Contains(t, normalized, "jsonb_set")
	require.Contains(t, normalized, "INSERT INTO scheduler_outbox")
	require.Contains(t, normalized, "FROM updated")
	require.NotContains(t, normalized, "a.schedulable IS TRUE", "stale instances must be able to repeat the pause idempotently")
	require.Equal(t, false, exec.execArgs[0][0])
	require.Equal(t, "consecutive 429/502 threshold reached", exec.execArgs[0][1])
	require.Equal(t, service.AccountSchedulingDisabledReasonExtraKey, exec.execArgs[0][2])
}

func TestAccountRepositorySetSchedulableWithReasonReturnsNotFoundForMissingAccount(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(0)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	err := repo.SetSchedulableWithReason(context.Background(), 404, false, "threshold reached")

	require.ErrorIs(t, err, service.ErrAccountNotFound)
}

func TestAccountRepositorySetSchedulableWithReasonClearsReasonOnRecovery(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	require.NoError(t, repo.SetSchedulableWithReason(context.Background(), 42, true, ""))
	normalized := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, normalized, "COALESCE(a.extra, '{}'::jsonb) - $3")
	require.Equal(t, true, exec.execArgs[0][0])
	require.Equal(t, "", exec.execArgs[0][1])
}

func TestAccountRepositoryBulkEnableClearsSchedulingDisabledReason(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)
	enabled := true

	rows, err := repo.BulkUpdate(context.Background(), []int64{42}, service.AccountBulkUpdate{Schedulable: &enabled})

	require.NoError(t, err)
	require.Equal(t, int64(1), rows)
	require.NotEmpty(t, exec.execQueries)
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[0]), "- '"+service.AccountSchedulingDisabledReasonExtraKey+"'")
}

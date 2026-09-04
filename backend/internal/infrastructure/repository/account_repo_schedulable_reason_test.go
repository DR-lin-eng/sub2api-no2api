package repository

import (
	"context"
	"testing"
	"time"

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
	require.Equal(t, service.AccountAutoEnableSourceExtraKey, exec.execArgs[0][3])
	require.Equal(t, service.AccountAutoEnableAtExtraKey, exec.execArgs[0][4])
	require.Equal(t, "", exec.execArgs[0][5])
	require.Equal(t, int64(0), exec.execArgs[0][6])
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
	require.Contains(t, normalized, "- $4")
	require.Contains(t, normalized, "- $5")
	require.Equal(t, true, exec.execArgs[0][0])
	require.Equal(t, "", exec.execArgs[0][1])
}

func TestAccountRepositorySetSchedulableWithReasonAndAutoEnablePersistsOAuthMarker(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)
	resetAt := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

	require.NoError(t, repo.SetSchedulableWithReasonAndAutoEnable(
		context.Background(),
		42,
		false,
		"consecutive 429/502 threshold reached",
		service.AccountAutoEnableSourceOpenAIOAuthFailure,
		&resetAt,
	))

	require.Equal(t, service.AccountAutoEnableSourceOpenAIOAuthFailure, exec.execArgs[0][5])
	require.Equal(t, resetAt.Unix(), exec.execArgs[0][6])
}

func TestAccountRepositoryAutoEnableOpenAIOAuthAccountIfMarkedIsScopedAndAtomic(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)

	enabled, err := repo.AutoEnableOpenAIOAuthAccountIfMarked(context.Background(), 42)

	require.NoError(t, err)
	require.True(t, enabled)
	require.Len(t, exec.execQueries, 1)
	normalized := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, normalized, "a.platform = $5")
	require.Contains(t, normalized, "a.type = $6")
	require.Contains(t, normalized, "a.schedulable = FALSE")
	require.Contains(t, normalized, "a.extra ->> $2 = $8")
	require.Contains(t, normalized, "rate_limited_at = NULL")
	require.Contains(t, normalized, "INSERT INTO scheduler_outbox")
	require.Equal(t, service.PlatformOpenAI, exec.execArgs[0][4])
	require.Equal(t, service.AccountTypeOAuth, exec.execArgs[0][5])
	require.Equal(t, service.AccountAutoEnableSourceOpenAIOAuthFailure, exec.execArgs[0][7])
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
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[0]), "- '"+service.AccountAutoEnableSourceExtraKey+"'")
	require.Contains(t, normalizeSQLWhitespace(exec.execQueries[0]), "- '"+service.AccountAutoEnableAtExtraKey+"'")
}

func TestAccountRepositoryBulkManualDisableClearsAutoEnableMarker(t *testing.T) {
	exec := &recordingSQLExecutor{result: rowsAffectedResult(1)}
	repo := newAccountRepositoryWithSQL(nil, exec, nil)
	disabled := false

	_, err := repo.BulkUpdate(context.Background(), []int64{42}, service.AccountBulkUpdate{Schedulable: &disabled})

	require.NoError(t, err)
	normalized := normalizeSQLWhitespace(exec.execQueries[0])
	require.Contains(t, normalized, "- '"+service.AccountAutoEnableSourceExtraKey+"'")
	require.Contains(t, normalized, "- '"+service.AccountAutoEnableAtExtraKey+"'")
	require.NotContains(t, normalized, "- '"+service.AccountSchedulingDisabledReasonExtraKey+"'", "manual disable keeps the existing reason visible")
}

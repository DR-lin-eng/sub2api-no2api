package repository

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestResetQuotaUsedClearsAccountRateLimitAtomically(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE accounts SET extra = (
			COALESCE(extra, '{}'::jsonb)
			|| '{"quota_used": 0, "quota_daily_used": 0, "quota_weekly_used": 0}'::jsonb
		) - 'quota_daily_start' - 'quota_weekly_start' - 'quota_daily_reset_at' - 'quota_weekly_reset_at',
		rate_limited_at = NULL, rate_limit_reset_at = NULL, updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`)).
		WithArgs(int64(42)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO scheduler_outbox").
		WithArgs(service.SchedulerOutboxEventAccountChanged, sqlmock.AnyArg(), nil, nil, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(0, 1))

	require.NoError(t, repo.ResetQuotaUsed(context.Background(), 42))
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResetQuotaUsedReportsMissingAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	repo := newAccountRepositoryWithSQL(nil, db, nil)
	mock.ExpectExec("UPDATE accounts SET extra").
		WithArgs(int64(404)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err = repo.ResetQuotaUsed(context.Background(), 404)
	require.ErrorIs(t, err, service.ErrAccountNotFound)
	require.NoError(t, mock.ExpectationsWereMet())
}

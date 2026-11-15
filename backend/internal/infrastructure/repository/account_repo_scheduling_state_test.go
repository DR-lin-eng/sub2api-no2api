package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/DR-lin-eng/sub2api-no2api/ent"
	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestGetAccountSchedulingStateUsesSingleProjectionQuery(t *testing.T) {
	var capturedSQL string
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(captureEntQueryMatcher{actual: &capturedSQL}))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() { _ = client.Close() })
	repo := newAccountRepositoryWithSQL(client, db, nil)

	mock.ExpectQuery("account scheduling state projection").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "platform", "type", "status", "schedulable", "expires_at",
			"auto_pause_on_expired", "overload_until", "rate_limit_reset_at",
			"temp_unschedulable_until", "temp_unschedulable_reason", "extra",
		}).AddRow(
			int64(91), service.PlatformOpenAI, service.AccountTypeAPIKey,
			service.StatusActive, true, nil, true, nil, nil, nil, nil, []byte(`{}`),
		))

	state, err := repo.GetAccountSchedulingState(context.Background(), 91)
	require.NoError(t, err)
	require.Equal(t, service.AccountSchedulingState{Exists: true, Schedulable: true}, state)
	require.NoError(t, mock.ExpectationsWereMet(), "projection path must execute exactly one query")

	normalized := normalizeSQLWhitespace(capturedSQL)
	selectClause, _, found := strings.Cut(normalized, " FROM ")
	require.True(t, found, "unexpected projection SQL: %s", normalized)
	require.Equal(t, 11, strings.Count(selectClause, ","), "projection must select exactly twelve columns: %s", selectClause)
	for _, field := range []string{
		"id", "platform", "type", "status", "schedulable", "expires_at",
		"auto_pause_on_expired", "overload_until", "rate_limit_reset_at",
		"temp_unschedulable_until", "temp_unschedulable_reason", "extra",
	} {
		require.Contains(t, selectClause, `"`+field+`"`)
	}
	require.NotContains(t, selectClause, "credentials")
	require.NotContains(t, selectClause, "proxy_id")
	require.NotContains(t, normalized, "account_groups")
	require.NotContains(t, normalized, "proxies")
}

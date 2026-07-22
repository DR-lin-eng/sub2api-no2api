package repository

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestAuditLogListDoesNotSelectRequestBody(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)
	defer func() { _ = db.Close() }()

	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM audit_logs`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?s)SELECT.*'' AS request_body.*FROM audit_logs`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "actor_user_id", "actor_email", "actor_role", "auth_method",
			"credential_masked", "action", "method", "path", "request_id", "client_ip",
			"user_agent", "request_body", "status_code", "latency_ms", "extra",
		}).AddRow(
			int64(1), time.Now().UTC(), nil, "admin@example.com", "admin", "jwt",
			"Bearer sk-****test", "admin.settings.update", "PUT", "/api/v1/admin/settings",
			"req-1", "127.0.0.1", "test-agent", "", 200, int64(5), "{}",
		))

	repo := &auditLogRepository{db: db}
	result, err := repo.List(context.Background(), &service.AuditLogFilter{Page: 1, PageSize: 50})
	require.NoError(t, err)
	require.Len(t, result.Logs, 1)
	require.Empty(t, result.Logs[0].RequestBody)
	require.NoError(t, mock.ExpectationsWereMet())
}

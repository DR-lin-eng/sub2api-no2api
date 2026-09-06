package repository

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func originalActivityMigration(t *testing.T) string {
	t.Helper()
	b, err := migrations.FS.ReadFile(activityCenterMigration)
	require.NoError(t, err)
	content := strings.TrimSpace(string(b))
	require.Equal(t, activityCenterMigrationChecksum, migrationChecksum(content), "published 237 must remain immutable")
	return content
}

func TestMigrationSQLForExecution_ActivityCenter(t *testing.T) {
	original := originalActivityMigration(t)
	got, err := migrationSQLForExecution(activityCenterMigration, migrationChecksum(original), original)
	require.NoError(t, err)
	require.NotEqual(t, original, got)
	for _, column := range []string{"type", "campaign_type"} {
		require.Equal(t, 3, strings.Count(got, "CHECK ("+column+" IN ("+activityCenterCompatibleTypes+"))"))
	}
	// No statements, indexes, data backfills or non-type constraints may change.
	normalize := regexp.MustCompile(`CHECK \((?:type|campaign_type) IN \([^\n]*\)\)`)
	require.Equal(t, normalize.ReplaceAllString(original, "TYPE_CHECK"), normalize.ReplaceAllString(got, "TYPE_CHECK"))
	forward, err := migrations.FS.ReadFile("238_preserve_legacy_activity_campaign_types.sql")
	require.NoError(t, err)
	require.Equal(t, 2, strings.Count(string(forward), activityCenterCompatibleTypes))
}

func TestMigrationSQLForExecution_RejectsUnknownActivitySQL(t *testing.T) {
	original := originalActivityMigration(t)
	changed := original + "\nSELECT 1;"
	_, err := migrationSQLForExecution(activityCenterMigration, migrationChecksum(changed), changed)
	require.ErrorContains(t, err, "requires original migration")
	_, err = migrationSQLForExecution(activityCenterMigration, activityCenterMigrationChecksum, "SELECT 1;")
	require.ErrorContains(t, err, "unexpected type constraint")
	got, err := migrationSQLForExecution("999_other.sql", migrationChecksum(changed), changed)
	require.NoError(t, err)
	require.Equal(t, changed, got, "unrelated migrations must execute unchanged")
}

func TestApplyMigrationsFS_ActivityChecksumAndTransaction(t *testing.T) {
	original := originalActivityMigration(t)
	execution, err := migrationSQLForExecution(activityCenterMigration, activityCenterMigrationChecksum, original)
	require.NoError(t, err)
	for _, mode := range []string{"pending", "already_applied", "sql_failure", "record_failure", "checksum_mismatch", "unknown_file"} {
		t.Run(mode, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			db.SetMaxOpenConns(1)
			prepareMigrationsBootstrapExpectations(mock)
			query := mock.ExpectQuery("SELECT checksum FROM schema_migrations WHERE filename = \\$1").WithArgs(activityCenterMigration)
			switch mode {
			case "already_applied":
				query.WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow(activityCenterMigrationChecksum))
			case "checksum_mismatch":
				query.WillReturnRows(sqlmock.NewRows([]string{"checksum"}).AddRow("unknown"))
			default:
				query.WillReturnError(sql.ErrNoRows)
			}
			if mode == "pending" || mode == "sql_failure" || mode == "record_failure" {
				mock.ExpectBegin()
				exec := mock.ExpectExec(regexp.QuoteMeta(execution))
				if mode == "sql_failure" {
					exec.WillReturnError(errors.New("fixture SQL failure"))
					mock.ExpectRollback()
				} else {
					exec.WillReturnResult(sqlmock.NewResult(0, 0))
					record := mock.ExpectExec("INSERT INTO schema_migrations").WithArgs(activityCenterMigration, activityCenterMigrationChecksum)
					if mode == "record_failure" {
						record.WillReturnError(errors.New("fixture record failure"))
						mock.ExpectRollback()
					} else {
						record.WillReturnResult(sqlmock.NewResult(1, 1))
						mock.ExpectCommit()
					}
				}
			}
			mock.ExpectExec("SELECT pg_advisory_unlock\\(\\$1\\)").WithArgs(migrationsAdvisoryLockID).WillReturnResult(sqlmock.NewResult(0, 1))
			content := original
			if mode == "unknown_file" {
				content += "\nSELECT 1;"
			}
			err = applyMigrationsFS(context.Background(), db, fstest.MapFS{activityCenterMigration: &fstest.MapFile{Data: []byte(content)}})
			if mode == "pending" || mode == "already_applied" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

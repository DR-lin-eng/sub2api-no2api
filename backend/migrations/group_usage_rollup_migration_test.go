package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGroupUsageRollupMigrationsCreateBucketsAndInvalidation(t *testing.T) {
	base, err := FS.ReadFile("222_group_usage_daily_rollups.sql")
	require.NoError(t, err)
	baseSQL := string(base)
	require.Contains(t, baseSQL, "CREATE TABLE IF NOT EXISTS usage_group_daily_rollups")
	require.Contains(t, baseSQL, "PRIMARY KEY (bucket_date, group_id)")
	require.Contains(t, baseSQL, "CREATE TABLE IF NOT EXISTS usage_group_rollup_state")
	require.Contains(t, baseSQL, "REFERENCING NEW TABLE AS inserted_usage_logs")
	require.Contains(t, baseSQL, "AFTER UPDATE OF created_at, group_id, actual_cost")
	require.Contains(t, baseSQL, "pg_advisory_xact_lock_shared(1398096434, 1196576853)")
	require.NotContains(t, baseSQL, "FOR KEY SHARE")

	timezoneMigration, err := FS.ReadFile("223_group_usage_rollup_timezone.sql")
	require.NoError(t, err)
	timezoneSQL := string(timezoneMigration)
	require.Contains(t, timezoneSQL, "ADD COLUMN IF NOT EXISTS timezone_name TEXT")
	require.Contains(t, timezoneSQL, "current_setting('TimeZone')")
	require.Contains(t, timezoneSQL, "clock_timestamp()")
	require.Contains(t, timezoneSQL, "pg_advisory_xact_lock_shared(1398096434, 1196576853)")
	require.NotContains(t, timezoneSQL, "FOR KEY SHARE")
}

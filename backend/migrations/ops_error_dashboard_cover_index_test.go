package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpsErrorDashboardCoverIndexMigration(t *testing.T) {
	content, err := FS.ReadFile("192_ops_error_dashboard_cover_index_notx.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_ops_error_logs_dashboard_time_cover")
	require.Contains(t, sql, "ON ops_error_logs (created_at DESC)")
	require.Contains(t, sql, "INCLUDE (status_code, is_business_limited, error_owner, upstream_status_code, platform, group_id)")
	require.Contains(t, sql, "WHERE is_count_tokens = FALSE")
}

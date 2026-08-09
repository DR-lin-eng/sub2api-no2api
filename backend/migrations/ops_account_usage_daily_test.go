package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpsAccountUsageDailyMigrationPreservesDisplayDimensions(t *testing.T) {
	content, err := FS.ReadFile("204_ops_account_usage_daily.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS ops_account_usage_daily")
	require.Contains(t, sql, "CURRENT_DATE - 29")
	require.Contains(t, sql, "model, inbound_endpoint, upstream_endpoint")
	require.Contains(t, sql, "COALESCE(image_count, 0) = 0 AND COALESCE(video_count, 0) = 0")
	require.Contains(t, sql, "last_usage_log_id")
	require.Contains(t, sql, "ON CONFLICT (bucket_date, account_id, model, inbound_endpoint, upstream_endpoint) DO NOTHING")
}

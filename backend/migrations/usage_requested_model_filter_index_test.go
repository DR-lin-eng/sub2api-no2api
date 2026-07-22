package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageRequestedModelFilterIndexMigration(t *testing.T) {
	content, err := FS.ReadFile("191_usage_requested_model_filter_index_notx.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_requested_model_filter_created_id")
	require.Contains(t, sql, "(COALESCE(NULLIF(TRIM(requested_model), ''), model))")
	require.Contains(t, sql, "created_at DESC, id DESC")
}

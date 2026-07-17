package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpsImageGenerationStatsIndexMigration(t *testing.T) {
	content, err := FS.ReadFile("183_ops_image_generation_stats_index_notx.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_logs_image_generation_created_at")
	require.Contains(t, sql, "ON usage_logs (created_at DESC)")
	require.Contains(t, sql, "WHERE image_count > 0 AND COALESCE(video_count, 0) = 0")
}

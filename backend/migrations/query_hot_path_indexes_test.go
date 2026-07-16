package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestQueryHotPathIndexesMigration(t *testing.T) {
	content, err := FS.ReadFile("178_query_hot_path_indexes_notx.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_api_keys_active_user_id ON api_keys (user_id, id DESC) WHERE deleted_at IS NULL")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_batch_image_jobs_owner_created_active ON batch_image_jobs (user_id, api_key_id, created_at DESC, id DESC) WHERE user_deleted_at IS NULL")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_batch_image_jobs_stale_unsubmitted ON batch_image_jobs (updated_at, id) WHERE status IN ('created', 'uploading') AND provider_job_name IS NULL AND COALESCE(hold_amount, estimated_cost, 0) > 0")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_batch_image_jobs_input_cleanup_due ON batch_image_jobs (id) WHERE input_deleted_at IS NULL AND provider_input_ref IS NOT NULL AND status IN ('completed', 'failed', 'cancelled', 'output_deleted')")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_batch_image_jobs_output_cleanup_due ON batch_image_jobs (output_expires_at, id) WHERE output_deleted_at IS NULL AND provider_output_ref IS NOT NULL AND status = 'completed' AND output_expires_at IS NOT NULL")
	require.Contains(t, sql, "DROP INDEX CONCURRENTLY IF EXISTS batch_image_jobs_output_expires_at_idx")
}

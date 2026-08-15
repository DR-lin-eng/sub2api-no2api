package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUsageBillingQueueRescueMigrationsAreRollingUpgradeSafe(t *testing.T) {
	base, err := FS.ReadFile("224_usage_billing_queue_rescue.sql")
	require.NoError(t, err)
	baseSQL := string(base)
	for _, fragment := range []string{
		"ADD COLUMN IF NOT EXISTS cleanup_attempts",
		"ADD COLUMN IF NOT EXISTS last_error_class",
		"ADD COLUMN IF NOT EXISTS last_claimed_by",
		"ADD COLUMN IF NOT EXISTS reconcile_required_at",
		"CREATE TABLE IF NOT EXISTS usage_billing_admin_actions",
		"replay_count",
	} {
		require.Contains(t, baseSQL, fragment)
	}

	indexes, err := FS.ReadFile("225_usage_billing_queue_rescue_indexes_notx.sql")
	require.NoError(t, err)
	indexSQL := string(indexes)
	require.Contains(t, indexSQL, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_billing_jobs_unsettled_ready")
	require.Contains(t, indexSQL, "WHERE settled_at IS NULL")
	require.Contains(t, indexSQL, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_billing_jobs_cleanup_ready")
	require.Contains(t, indexSQL, "WHERE settled_at IS NOT NULL")
	require.Contains(t, indexSQL, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_usage_billing_jobs_reconcile")
}

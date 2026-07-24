package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUserRequestSchedulingTierMigrations(t *testing.T) {
	columnContent, err := FS.ReadFile("194_user_request_scheduling_tier.sql")
	require.NoError(t, err)
	columnSQL := strings.Join(strings.Fields(string(columnContent)), " ")
	require.Contains(t, columnSQL, "request_scheduling_tier SMALLINT NOT NULL DEFAULT 1")
	require.Contains(t, columnSQL, "CHECK (request_scheduling_tier IN (0, 1, 2))")
	require.Contains(t, columnSQL, "OLD.request_scheduling_tier IS NOT DISTINCT FROM NEW.request_scheduling_tier")
	require.Contains(t, columnSQL, "INSERT INTO auth_cache_invalidation_outbox (cache_key)")

	indexContent, err := FS.ReadFile("195_user_request_scheduling_tier_index_notx.sql")
	require.NoError(t, err)
	indexSQL := strings.Join(strings.Fields(string(indexContent)), " ")
	require.Contains(t, indexSQL, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_users_request_scheduling_tier_active")
	require.Contains(t, indexSQL, "ON users (request_scheduling_tier) WHERE deleted_at IS NULL")
}

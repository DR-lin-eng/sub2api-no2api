package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelModelTimePricingMigrationIsRollingUpgradeSafe(t *testing.T) {
	contents, err := FS.ReadFile("233_channel_model_time_pricing.sql")
	require.NoError(t, err)
	sql := string(contents)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS time_pricing JSONB")
	require.Contains(t, sql, "COMMENT ON COLUMN channel_model_pricing.time_pricing")
	require.False(t, strings.Contains(sql, "CREATE INDEX CONCURRENTLY"), "metadata-only column migration must remain transactional")
}

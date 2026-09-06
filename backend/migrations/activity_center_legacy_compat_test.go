package migrations

import (
	"io/fs"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestActivityCenterLegacyCompatMigrationPrecedes237AndIsFreshDatabaseSafe(t *testing.T) {
	content, err := FS.ReadFile("236a_repair_activity_center_campaign_config.sql")
	require.NoError(t, err)
	sql := string(content)

	require.Contains(t, sql, "to_regclass('public.act_campaigns') IS NOT NULL")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS config_json TEXT NOT NULL DEFAULT '{}'")
	require.True(t, strings.HasPrefix(strings.TrimSpace(sql), "-- Repair legacy activity-center"))

	migrationNames, err := fs.Glob(FS, "*.sql")
	require.NoError(t, err)
	var has236a, has237 bool
	for _, name := range migrationNames {
		has236a = has236a || name == "236a_repair_activity_center_campaign_config.sql"
		has237 = has237 || name == "237_add_activity_center.sql"
	}
	require.True(t, has236a)
	require.True(t, has237)
	require.Less(t, "236a_repair_activity_center_campaign_config.sql", "237_add_activity_center.sql")
}

func TestActivityCenterLegacyTypesConvergeAfter237(t *testing.T) {
	content, err := FS.ReadFile("238_preserve_legacy_activity_campaign_types.sql")
	require.NoError(t, err)
	for _, column := range []string{"type", "campaign_type"} {
		require.Contains(t, string(content), "CHECK ("+column+" IN ('lottery', 'inflate', 'redeem', 'custom', 'checkin', 'external_link', 'announcement'))")
	}
	require.NotContains(t, strings.ToUpper(string(content)), "UPDATE ")
	require.NotContains(t, strings.ToUpper(string(content)), "DELETE ")
}

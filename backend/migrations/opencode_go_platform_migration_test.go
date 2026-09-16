package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpenCodeGoPlatformMigrationOnlyWidensChecks(t *testing.T) {
	content, err := FS.ReadFile("244_add_opencode_go_platform.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")

	for _, constraint := range []string{
		"user_platform_quotas_platform_check",
		"composite_model_routes_target_platform_check",
		"channel_monitors_provider_check",
		"channel_monitor_request_templates_provider_check",
	} {
		require.Contains(t, sql, constraint)
	}
	require.Contains(t, sql, "'opencode_go'")
	require.NotContains(t, strings.ToUpper(sql), "DROP TABLE")
	require.NotContains(t, strings.ToUpper(sql), "DROP COLUMN")
	require.NotContains(t, strings.ToUpper(sql), "UPDATE ")
}

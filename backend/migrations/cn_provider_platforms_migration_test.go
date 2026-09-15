package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCNProviderPlatformsMigrationOnlyWidensChecks(t *testing.T) {
	content, err := FS.ReadFile("243_add_cn_provider_platforms.sql")
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
	for _, platform := range []string{"kimi", "zhipu", "deepseek", "minimax"} {
		require.Contains(t, sql, "'"+platform+"'")
	}
	require.NotContains(t, strings.ToUpper(sql), "DROP TABLE")
	require.NotContains(t, strings.ToUpper(sql), "DROP COLUMN")
	require.NotContains(t, strings.ToUpper(sql), "UPDATE ")
}

package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChannelMonitorPassiveModeMigration(t *testing.T) {
	content, err := FS.ReadFile("186_channel_monitor_passive_mode.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "monitor_mode VARCHAR(16) NOT NULL DEFAULT 'active'")
	require.Contains(t, sql, "channel_id BIGINT")
	require.Contains(t, sql, "FOREIGN KEY (channel_id) REFERENCES channels(id) ON DELETE SET NULL")
	require.Contains(t, sql, "CHECK (monitor_mode IN ('active', 'passive'))")
	require.Contains(t, sql, "'unknown'")
}

func TestChannelMonitorPassiveIndexMigration(t *testing.T) {
	content, err := FS.ReadFile("187_channel_monitor_passive_index_notx.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS")
	require.Contains(t, sql, "ON usage_logs (channel_id, created_at DESC)")
	require.Contains(t, sql, "WHERE channel_id IS NOT NULL")
}

func TestChannelMonitorGroupTargetMigration(t *testing.T) {
	content, err := FS.ReadFile("189_channel_monitor_group_target.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "group_id BIGINT")
	require.Contains(t, sql, "FOREIGN KEY (group_id) REFERENCES groups(id) ON DELETE SET NULL")
	require.Contains(t, sql, "ON channel_monitors (monitor_mode, group_id)")
}

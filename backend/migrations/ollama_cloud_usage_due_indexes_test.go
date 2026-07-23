package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOllamaCloudUsageDueIndexesMigration(t *testing.T) {
	content, err := FS.ReadFile("193_ollama_cloud_usage_due_indexes_notx.sql")
	require.NoError(t, err)
	for _, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			require.NotContains(t, line, ";", "non-transactional migration comments must not confuse the statement splitter")
		}
	}

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_ollama_cloud_usage_group ON accounts ((credentials ->> 'api_key'), updated_at DESC, id)")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_ollama_cloud_usage_legacy ON accounts (id)")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_ollama_cloud_usage_due ON accounts (((extra #>> '{ollama_cloud_usage_snapshot,next_refresh_unix}')::numeric), id, (credentials ->> 'api_key'))")
	require.Contains(t, sql, "platform IN ('openai', 'anthropic')")
	require.Contains(t, sql, "credentials ->> 'api_key' <> ''")
	require.Contains(t, sql, "jsonb_typeof(extra -> 'ollama_cloud_usage_session') = 'string'")
	require.Contains(t, sql, `extra @> '{"ollama_cloud_usage_auto_refresh": true}'::jsonb`)
	require.Contains(t, sql, "jsonb_typeof(extra #> '{ollama_cloud_usage_snapshot,next_refresh_unix}') = 'number'")
	require.Contains(t, sql, "trunc((extra #>> '{ollama_cloud_usage_snapshot,next_refresh_unix}')::numeric)")
	require.Contains(t, sql, "BETWEEN 0 AND 9223372036854775807")
	require.Contains(t, sql, "IS NOT TRUE")
}

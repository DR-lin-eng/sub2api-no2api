package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUpstreamBillingProbeDueIndexesMigration(t *testing.T) {
	backfillContent, err := FS.ReadFile("181_backfill_upstream_billing_probe_next_unix.sql")
	require.NoError(t, err)
	backfillSQL := strings.Join(strings.Fields(string(backfillContent)), " ")
	require.Contains(t, backfillSQL, "UPDATE accounts AS account SET extra = jsonb_set")
	require.Contains(t, backfillSQL, "jsonb_path_query_first_tz")
	require.Contains(t, backfillSQL, "EXTRACT(EPOCH FROM parsed.next_probe_at)::bigint")
	require.Contains(t, backfillSQL, "parsed_next_probe_at IS NOT NULL")

	content, err := FS.ReadFile("182_upstream_billing_probe_due_indexes_notx.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_upstream_billing_probe_legacy ON accounts (id)")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_upstream_billing_probe_due ON accounts (((extra #>> '{upstream_billing_probe,next_probe_unix}')::numeric), id)")
	require.Contains(t, sql, `extra @> '{"upstream_billing_probe_enabled": true}'::jsonb`)
	require.Contains(t, sql, "jsonb_typeof(extra #> '{upstream_billing_probe,next_probe_unix}') = 'number'")
	require.Contains(t, sql, "extra #>> '{upstream_billing_probe,next_probe_unix}' ~ '^[0-9]{1,19}$'")
	require.Contains(t, sql, "IS NOT TRUE")
}

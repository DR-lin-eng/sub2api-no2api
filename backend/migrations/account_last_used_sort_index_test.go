package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountLastUsedSortIndexMigration(t *testing.T) {
	content, err := FS.ReadFile("184_account_last_used_sort_index_notx.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_accounts_last_used_at_id")
	require.Contains(t, sql, "ON accounts (last_used_at ASC NULLS FIRST, id ASC)")
	require.Contains(t, sql, "DROP INDEX CONCURRENTLY IF EXISTS idx_accounts_last_used_at")
}

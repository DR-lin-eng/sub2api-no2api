package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAPIKeyGroupBindingsMigrationBackfillsLegacyGroup(t *testing.T) {
	content, err := FS.ReadFile("209_add_api_key_group_bindings.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS group_bindings JSONB NOT NULL DEFAULT '[]'::jsonb")
	require.Contains(t, sql, "jsonb_build_object('group_id', group_id)")
	require.Contains(t, sql, "USING GIN (group_bindings jsonb_path_ops)")
}

package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAccountQualityRunsMigrationStoresRenderedArtifactsOnly(t *testing.T) {
	content, err := FS.ReadFile("241_account_quality_runs.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS account_quality_runs")
	require.Contains(t, sql, "model_version VARCHAR(64)")
	require.Contains(t, sql, "png BYTEA")
	require.Contains(t, sql, "webp BYTEA")
}

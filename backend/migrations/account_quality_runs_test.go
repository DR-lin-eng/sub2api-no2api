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

func TestAccountQualityEmbeddedPreviewMigrationIsAdditive(t *testing.T) {
	content, err := FS.ReadFile("243_account_quality_embedded_svg.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS preview_svg BYTEA")
	require.Contains(t, sql, "preview_format VARCHAR(16)")
	require.NotContains(t, sql, "DROP")
}

func TestAccountQualityConversationMigrationIsAdditive(t *testing.T) {
	content, err := FS.ReadFile("242_account_quality_probe_details.sql")
	require.NoError(t, err)
	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS probe_details JSONB NOT NULL DEFAULT '{}'::jsonb")
	require.NotContains(t, sql, "DROP")
}

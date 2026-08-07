package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestUpstreamResponseModelMigrations(t *testing.T) {
	schemaSQL, err := migrations.FS.ReadFile("202_add_usage_log_upstream_response_model.sql")
	require.NoError(t, err)
	require.Contains(t, string(schemaSQL), "upstream_response_model VARCHAR(200)")
	require.Contains(t, string(schemaSQL), "upstream_model_mismatch BOOLEAN")

	indexSQL, err := migrations.FS.ReadFile(usageLogsUpstreamModelMismatchIndexMigration)
	require.NoError(t, err)
	nonTx, err := validateMigrationExecutionMode(usageLogsUpstreamModelMismatchIndexMigration, string(indexSQL))
	require.NoError(t, err)
	require.True(t, nonTx)
	require.Contains(t, string(indexSQL), "WHERE upstream_model_mismatch IS TRUE")
}

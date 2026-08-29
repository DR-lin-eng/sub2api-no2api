package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCustomModelMigrationsFollowCurrentSequenceAndStayOptIn(t *testing.T) {
	configMigration, err := FS.ReadFile("234_add_custom_model_configs.sql")
	require.NoError(t, err)
	sql := string(configMigration)
	require.Contains(t, sql, "idx_custom_model_configs_model_name_ci")
	require.Contains(t, sql, "lower(model_name)")
	require.Contains(t, sql, "VALUES ('custom_model_config_enabled', 'false')")
	require.NotContains(t, sql, "template_id")

	_, err = FS.ReadFile("233_add_custom_model_configs.sql")
	require.Error(t, err, "migration 233 is already owned by channel model time pricing")
}

func TestMediaStudioIdentityMigrationPreservesExistingTokens(t *testing.T) {
	content, err := FS.ReadFile("235_media_studio_api_key_identity.sql")
	require.NoError(t, err)
	sql := string(content)
	require.Contains(t, sql, "Media Studio Legacy")
	require.Contains(t, sql, "idx_api_keys_media_studio_user_group")
	require.NotContains(t, sql, "ROW_NUMBER()")
	require.NotContains(t, strings.ToUpper(sql), "DELETE FROM API_KEYS")
}

func TestCustomModelTemplateMigrationAddsExplicitForeignKey(t *testing.T) {
	content, err := FS.ReadFile("236_add_custom_model_request_templates.sql")
	require.NoError(t, err)
	sql := string(content)
	require.Contains(t, sql, "ADD COLUMN IF NOT EXISTS template_id BIGINT NULL")
	require.Contains(t, sql, "ADD CONSTRAINT custom_model_configs_template_id_fkey")
	require.Contains(t, sql, "ON DELETE SET NULL")
	require.Contains(t, sql, "idx_custom_model_request_templates_name_ci")
}

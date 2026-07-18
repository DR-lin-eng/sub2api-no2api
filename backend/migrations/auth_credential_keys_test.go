package migrations

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthCredentialKeysMigrationDefinesRotationAndDecryptWindows(t *testing.T) {
	content, err := FS.ReadFile("185_auth_credential_keys.sql")
	require.NoError(t, err)

	sql := strings.Join(strings.Fields(string(content)), " ")
	require.Contains(t, sql, "CREATE TABLE IF NOT EXISTS auth_credential_keys")
	require.Contains(t, sql, "key_id VARCHAR(32) PRIMARY KEY")
	require.Contains(t, sql, "public_expires_at = slot_started_at + INTERVAL '12 hours'")
	require.Contains(t, sql, "decrypt_expires_at = public_expires_at + INTERVAL '30 minutes'")
}

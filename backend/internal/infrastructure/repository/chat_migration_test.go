package repository

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/stretchr/testify/require"
)

func TestChatUnreadIndexMigrationIsPartialAndNonTransactional(t *testing.T) {
	migration, err := migrations.FS.ReadFile("201_add_chat_unread_index_notx.sql")
	require.NoError(t, err)
	content := strings.TrimSpace(string(migration))
	require.Contains(t, content, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chat_conversations_unread_by_admin_active")
	require.Contains(t, content, "WHERE unread_by_admin > 0")
	require.NotContains(t, content, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chat_conversations_unread_by_admin\n")
	nonTx, err := validateMigrationExecutionMode("201_add_chat_unread_index_notx.sql", content)
	require.NoError(t, err)
	require.True(t, nonTx)
}

func TestExpandedSupportChatUsesOnlineIndexesForExistingMessageTable(t *testing.T) {
	expand, err := migrations.FS.ReadFile("226_expand_support_chat.sql")
	require.NoError(t, err)
	expandSQL := string(expand)
	require.Contains(t, expandSQL, "ADD COLUMN IF NOT EXISTS kind")
	require.Contains(t, expandSQL, "NOT VALID")
	require.Contains(t, expandSQL, "size = octet_length(data)")
	require.NotContains(t, expandSQL, "CREATE INDEX CONCURRENTLY", "transactional migrations cannot contain concurrent indexes")

	indexes, err := migrations.FS.ReadFile("227_support_chat_indexes_notx.sql")
	require.NoError(t, err)
	indexSQL := strings.TrimSpace(string(indexes))
	require.Contains(t, indexSQL, "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS idx_chat_messages_sender_idempotency")
	require.Contains(t, indexSQL, "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_chat_messages_conversation_created_id")
	require.Contains(t, indexSQL, "DROP INDEX CONCURRENTLY IF EXISTS idx_chat_messages_conversation_id_created_at")
	nonTx, err := validateMigrationExecutionMode("227_support_chat_indexes_notx.sql", indexSQL)
	require.NoError(t, err)
	require.True(t, nonTx)
}

//go:build integration

package repository

import (
	"context"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
	"github.com/stretchr/testify/require"
)

func TestChatConversationListPrioritizesUnreadConversations(t *testing.T) {
	baseCtx := context.Background()
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(baseCtx, tx)
	client := tx.Client()

	readUser := createEntUser(t, ctx, client, uniqueSoftDeleteValue(t, "chat-order-read")+"@example.test")
	unreadUser := createEntUser(t, ctx, client, uniqueSoftDeleteValue(t, "chat-order-unread")+"@example.test")
	manualUser := createEntUser(t, ctx, client, uniqueSoftDeleteValue(t, "chat-order-manual")+"@example.test")

	conversationRepo := NewChatConversationRepository(client)
	messageRepo := NewChatMessageRepository(client)
	newest := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	old := newest.Add(-2 * time.Hour)
	older := old.Add(-time.Hour)

	readConversation, err := conversationRepo.GetOrCreateByUserID(ctx, readUser.ID)
	require.NoError(t, err)
	unreadMessage := &chat.Message{
		ConversationID: readConversation.ID, SenderType: chat.SenderTypeUser, SenderID: readUser.ID,
		Content: "newer but already read", Kind: chat.MessageKindText, Metadata: []byte(`{}`),
	}
	require.NoError(t, messageRepo.CreateAndTouch(ctx, unreadMessage, newest, chat.SenderTypeUser))
	_, err = client.ExecContext(ctx, "UPDATE chat_messages SET created_at = $1 WHERE id = $2", newest, unreadMessage.ID)
	require.NoError(t, err)
	_, _, err = conversationRepo.MarkRead(ctx, readConversation.ID, chat.SenderTypeAdmin)
	require.NoError(t, err)

	unreadConversation, err := conversationRepo.GetOrCreateByUserID(ctx, unreadUser.ID)
	require.NoError(t, err)
	unreadMessage = &chat.Message{
		ConversationID: unreadConversation.ID, SenderType: chat.SenderTypeUser, SenderID: unreadUser.ID,
		Content: "older unread", Kind: chat.MessageKindText, Metadata: []byte(`{}`),
	}
	require.NoError(t, messageRepo.CreateAndTouch(ctx, unreadMessage, old, chat.SenderTypeUser))
	_, err = client.ExecContext(ctx, "UPDATE chat_messages SET created_at = $1 WHERE id = $2", old, unreadMessage.ID)
	require.NoError(t, err)

	manualConversation, err := conversationRepo.GetOrCreateByUserID(ctx, manualUser.ID)
	require.NoError(t, err)
	manualMessage := &chat.Message{
		ConversationID: manualConversation.ID, SenderType: chat.SenderTypeUser, SenderID: manualUser.ID,
		Content: "old manual reminder", Kind: chat.MessageKindText, Metadata: []byte(`{}`),
	}
	require.NoError(t, messageRepo.CreateAndTouch(ctx, manualMessage, older, chat.SenderTypeUser))
	_, err = client.ExecContext(ctx, "UPDATE chat_messages SET created_at = $1 WHERE id = $2", older, manualMessage.ID)
	require.NoError(t, err)
	_, _, err = conversationRepo.MarkRead(ctx, manualConversation.ID, chat.SenderTypeAdmin)
	require.NoError(t, err)
	changed, err := conversationRepo.MarkUnreadByAdmin(ctx, manualConversation.ID)
	require.NoError(t, err)
	require.True(t, changed)

	items, page, err := conversationRepo.List(ctx, pagination.PaginationParams{Page: 1, PageSize: 10}, chat.ConversationListFilters{})
	require.NoError(t, err)
	require.EqualValues(t, 3, page.Total)
	require.Equal(t, []int64{unreadConversation.ID, manualConversation.ID, readConversation.ID}, []int64{
		items[0].ID, items[1].ID, items[2].ID,
	})
	require.Equal(t, 1, items[0].UnreadByAdmin)
	require.True(t, items[1].ManuallyUnreadByAdmin)
	require.Zero(t, items[2].UnreadByAdmin)
	require.Zero(t, items[2].ManuallyUnreadByAdmin)
}

func TestChatConversationListOrdersReadThreadsByLatestPersistedMessage(t *testing.T) {
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(context.Background(), tx)
	client := tx.Client()
	conversations := NewChatConversationRepository(client)
	messages := NewChatMessageRepository(client)
	start := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)

	createConversation := func(name string) *chat.Conversation {
		user := createEntUser(t, ctx, client, uniqueSoftDeleteValue(t, name)+"@example.test")
		conversation, err := conversations.GetOrCreateByUserID(ctx, user.ID)
		require.NoError(t, err)
		return conversation
	}
	first := createConversation("chat-latest-first")
	second := createConversation("chat-latest-second")
	var empty *chat.Conversation

	send := func(conversation *chat.Conversation, sender chat.SenderType, at time.Time, content string) {
		message := &chat.Message{
			ConversationID: conversation.ID, SenderType: sender, SenderID: conversation.UserID,
			Content: content, Kind: chat.MessageKindText, Metadata: []byte(`{}`),
		}
		require.NoError(t, messages.CreateAndTouch(ctx, message, at, sender))
		_, err := client.ExecContext(ctx, "UPDATE chat_messages SET created_at = $1 WHERE id = $2", at, message.ID)
		require.NoError(t, err)
	}
	send(first, chat.SenderTypeUser, start, "first contact")
	send(second, chat.SenderTypeUser, start.Add(time.Hour), "later contact")
	send(first, chat.SenderTypeAdmin, start.Add(2*time.Hour), "latest support reply")
	for _, conversation := range []*chat.Conversation{first, second} {
		_, _, err := conversations.MarkRead(ctx, conversation.ID, chat.SenderTypeAdmin)
		require.NoError(t, err)
		_, _, err = conversations.MarkRead(ctx, conversation.ID, chat.SenderTypeUser)
		require.NoError(t, err)
	}

	assertOrder := func(t *testing.T) {
		t.Helper()
		expected := []int64{first.ID, second.ID}
		if empty != nil {
			expected = append(expected, empty.ID)
		}
		var ids []int64
		for page := 1; page <= len(expected); page++ {
			items, result, err := conversations.List(ctx, pagination.PaginationParams{Page: page, PageSize: 1}, chat.ConversationListFilters{})
			require.NoError(t, err)
			require.EqualValues(t, len(expected), result.Total)
			require.Len(t, items, 1)
			ids = append(ids, items[0].ID)
			require.Zero(t, items[0].UnreadByAdmin)
			require.False(t, items[0].ManuallyUnreadByAdmin)
			if items[0].ID == first.ID {
				require.NotNil(t, items[0].LastMessageAt)
				require.WithinDuration(t, start.Add(2*time.Hour), *items[0].LastMessageAt, time.Microsecond)
			}
		}
		require.Equal(t, expected, ids)
	}
	t.Run("read conversations use the last message including support replies", assertOrder)
	empty = createConversation("chat-latest-empty")
	t.Run("empty conversations do not precede active read conversations", assertOrder)
	t.Run("an outdated activity summary cannot restore first-contact order", func(t *testing.T) {
		_, err := client.ChatConversation.UpdateOneID(first.ID).SetLastMessageAt(start).Save(ctx)
		require.NoError(t, err)
		assertOrder(t)
	})
}

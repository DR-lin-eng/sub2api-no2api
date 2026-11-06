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
	_, _, err = conversationRepo.MarkRead(ctx, readConversation.ID, chat.SenderTypeAdmin)
	require.NoError(t, err)

	unreadConversation, err := conversationRepo.GetOrCreateByUserID(ctx, unreadUser.ID)
	require.NoError(t, err)
	unreadMessage = &chat.Message{
		ConversationID: unreadConversation.ID, SenderType: chat.SenderTypeUser, SenderID: unreadUser.ID,
		Content: "older unread", Kind: chat.MessageKindText, Metadata: []byte(`{}`),
	}
	require.NoError(t, messageRepo.CreateAndTouch(ctx, unreadMessage, old, chat.SenderTypeUser))

	manualConversation, err := conversationRepo.GetOrCreateByUserID(ctx, manualUser.ID)
	require.NoError(t, err)
	manualMessage := &chat.Message{
		ConversationID: manualConversation.ID, SenderType: chat.SenderTypeUser, SenderID: manualUser.ID,
		Content: "old manual reminder", Kind: chat.MessageKindText, Metadata: []byte(`{}`),
	}
	require.NoError(t, messageRepo.CreateAndTouch(ctx, manualMessage, older, chat.SenderTypeUser))
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

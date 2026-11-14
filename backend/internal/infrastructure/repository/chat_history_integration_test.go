//go:build integration

package repository

import (
	"context"
	"fmt"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
	"github.com/stretchr/testify/require"
)

// Use committed writes and independent clients: transaction-scoped tests alone
// cannot prove that a later browser request or restarted process sees history.
func TestChatHistoryPersistsAcrossConnectionsAndReadReceipts(t *testing.T) {
	ctx := context.Background()
	writer, err := dbent.Open("postgres", integrationPostgresDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = writer.Close() })

	user := createEntUser(t, ctx, writer, uniqueSoftDeleteValue(t, "chat-history-user")+"@example.test")
	admin := createEntUser(t, ctx, writer, uniqueSoftDeleteValue(t, "chat-history-admin")+"@example.test")
	t.Cleanup(func() {
		_, cleanupErr := integrationDB.ExecContext(ctx, "DELETE FROM users WHERE id IN ($1, $2)", user.ID, admin.ID)
		require.NoError(t, cleanupErr)
	})

	conversations := NewChatConversationRepository(writer)
	assets := NewChatAssetRepository(writer)
	service := chat.NewService(conversations, NewChatMessageRepository(writer))
	conversation, err := service.GetOrCreateConversationForUser(ctx, user.ID)
	require.NoError(t, err)
	key := fmt.Sprintf("chat-history-%d", time.Now().UnixNano())
	input := chat.SendMessageInput{Content: "历史记录必须在已读和重新连接后保留", IdempotencyKey: key}
	first, err := service.PostRichMessageFromUser(ctx, user.ID, input)
	require.NoError(t, err)
	asset := &chat.Asset{
		Scope: chat.AssetScopeMessage, ConversationID: &conversation.ID, UploadedBy: &admin.ID,
		Name: "history.png", MIMEType: "image/png", Size: 3, Data: []byte("png"),
	}
	require.NoError(t, assets.Create(ctx, asset))
	second, err := service.PostRichMessageFromAdmin(ctx, conversation.ID, admin.ID, chat.SendMessageInput{
		Content: "历史图片", Kind: chat.MessageKindImage, AssetIDs: []int64{asset.ID},
	})
	require.NoError(t, err)
	third, err := service.PostRichMessageFromUser(ctx, user.ID, chat.SendMessageInput{
		Kind: chat.MessageKindSticker, Sticker: &chat.StickerMetadata{Name: "OK", Emoji: "👍"},
	})
	require.NoError(t, err)
	require.NoError(t, service.MarkReadByUser(ctx, user.ID))
	require.NoError(t, service.MarkReadByAdmin(ctx, conversation.ID))
	replayed, err := service.PostRichMessageFromUser(ctx, user.ID, input)
	require.NoError(t, err)
	require.Equal(t, first.ID, replayed.ID)
	require.NoError(t, writer.Close())

	reader, err := dbent.Open("postgres", integrationPostgresDSN)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })
	readerService := chat.NewService(NewChatConversationRepository(reader), NewChatMessageRepository(reader))
	reopened, err := readerService.GetOrCreateConversationForUser(ctx, user.ID)
	require.NoError(t, err)
	require.Equal(t, conversation.ID, reopened.ID)
	require.Zero(t, reopened.UnreadByUser)
	require.Zero(t, reopened.UnreadByAdmin)
	require.NotNil(t, reopened.LastReadByUserAt)
	require.NotNil(t, reopened.LastReadByAdminAt)

	for _, side := range []string{"user", "admin"} {
		t.Run(side, func(t *testing.T) {
			var history []chat.Message
			for page := 1; page <= 2; page++ {
				params := pagination.PaginationParams{Page: page, PageSize: 2}
				var items []chat.Message
				var result *pagination.PaginationResult
				if side == "user" {
					items, result, err = readerService.ListMessagesForUser(ctx, user.ID, params)
				} else {
					items, result, err = readerService.ListMessagesForAdmin(ctx, conversation.ID, params)
				}
				require.NoError(t, err)
				require.EqualValues(t, 3, result.Total)
				history = append(history, items...)
			}
			require.Len(t, history, 3)
			require.Equal(t, []int64{third.ID, second.ID, first.ID}, []int64{history[0].ID, history[1].ID, history[2].ID})
			require.Equal(t, input.Content, history[2].Content)
			require.Len(t, history[1].Assets, 1)
			require.Equal(t, asset.ID, history[1].Assets[0].ID)
			require.Equal(t, chat.MessageKindSticker, history[0].Kind)
		})
	}
}

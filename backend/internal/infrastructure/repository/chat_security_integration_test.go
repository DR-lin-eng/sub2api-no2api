//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/chatasset"
	"github.com/Wei-Shaw/sub2api/ent/chatmessage"
	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
	"github.com/stretchr/testify/require"
)

func TestChatAssetAndReplyAuthorizationBoundaries(t *testing.T) {
	baseCtx := context.Background()
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(baseCtx, tx)
	client := tx.Client()

	userA := createEntUser(t, ctx, client, uniqueSoftDeleteValue(t, "chat-user-a")+"@example.test")
	userB := createEntUser(t, ctx, client, uniqueSoftDeleteValue(t, "chat-user-b")+"@example.test")
	adminA := createEntUser(t, ctx, client, uniqueSoftDeleteValue(t, "chat-admin-a")+"@example.test")
	adminB := createEntUser(t, ctx, client, uniqueSoftDeleteValue(t, "chat-admin-b")+"@example.test")

	conversationRepo := NewChatConversationRepository(client)
	messageRepo := NewChatMessageRepository(client)
	assetRepo := NewChatAssetRepository(client)
	conversationA, err := conversationRepo.GetOrCreateByUserID(ctx, userA.ID)
	require.NoError(t, err)
	conversationB, err := conversationRepo.GetOrCreateByUserID(ctx, userB.ID)
	require.NoError(t, err)

	userAsset := &chat.Asset{
		Scope: chat.AssetScopeMessage, ConversationID: &conversationA.ID, UploadedBy: &userA.ID,
		Name: "image.png", MIMEType: "image/png", Size: 3, Data: []byte("png"),
	}
	require.NoError(t, assetRepo.Create(ctx, userAsset))
	_, err = assetRepo.GetForUser(ctx, userAsset.ID, userB.ID, conversationB.ID)
	require.ErrorIs(t, err, chat.ErrAssetNotFound, "a user must not read another conversation's pending asset")
	_, err = assetRepo.GetForUser(ctx, userAsset.ID, userA.ID, conversationA.ID)
	require.NoError(t, err, "the uploader may preview their own pending normalized asset")
	_, err = assetRepo.GetForAdmin(ctx, userAsset.ID, adminA.ID)
	require.ErrorIs(t, err, chat.ErrAssetNotFound, "administrators must not enumerate a user's unsent upload")

	adminAsset := &chat.Asset{
		Scope: chat.AssetScopeMessage, ConversationID: &conversationA.ID, UploadedBy: &adminA.ID,
		Name: "admin.png", MIMEType: "image/png", Size: 3, Data: []byte("png"),
	}
	require.NoError(t, assetRepo.Create(ctx, adminAsset))
	_, err = assetRepo.GetForAdmin(ctx, adminAsset.ID, adminA.ID)
	require.NoError(t, err, "an administrator may preview their own pending upload")
	_, err = assetRepo.GetForAdmin(ctx, adminAsset.ID, adminB.ID)
	require.ErrorIs(t, err, chat.ErrAssetNotFound, "pending admin uploads stay private to their uploader")
	err = messageRepo.CreateAndTouch(ctx, &chat.Message{
		ConversationID: conversationA.ID, SenderType: chat.SenderTypeAdmin, SenderID: adminB.ID,
		Content: "[image]", Kind: chat.MessageKindImage, Metadata: []byte(`{}`), AssetIDs: []int64{adminAsset.ID},
	}, time.Now(), chat.SenderTypeAdmin)
	require.ErrorIs(t, err, chat.ErrMessageAssetsInvalid, "an admin must not attach another admin's pending upload")

	foreignReply := &chat.Message{
		ConversationID: conversationB.ID, SenderType: chat.SenderTypeUser, SenderID: userB.ID,
		Content: "other conversation", Kind: chat.MessageKindText, Metadata: []byte(`{}`),
	}
	require.NoError(t, messageRepo.CreateAndTouch(ctx, foreignReply, time.Now(), chat.SenderTypeUser))
	err = messageRepo.CreateAndTouch(ctx, &chat.Message{
		ConversationID: conversationA.ID, SenderType: chat.SenderTypeAdmin, SenderID: adminA.ID,
		Content: "cross reply", Kind: chat.MessageKindText, Metadata: []byte(`{}`), ReplyToID: &foreignReply.ID,
	}, time.Now(), chat.SenderTypeAdmin)
	require.ErrorIs(t, err, chat.ErrMessageReplyInvalid, "reply targets must stay in the same conversation")

	catalog := &chat.Asset{
		Scope: chat.AssetScopeLibrary, UploadedBy: &adminA.ID, Name: "catalog.png",
		MIMEType: "image/png", Size: 3, Data: []byte("png"), CatalogVisible: true,
	}
	require.NoError(t, assetRepo.Create(ctx, catalog))
	_, err = assetRepo.GetForAdmin(ctx, catalog.ID, adminB.ID)
	require.NoError(t, err, "visible catalog content is shared by support agents")
	linked := &chat.Message{
		ConversationID: conversationA.ID, SenderType: chat.SenderTypeAdmin, SenderID: adminA.ID,
		Content: "[image]", Kind: chat.MessageKindImage, Metadata: []byte(`{}`), AssetIDs: []int64{catalog.ID},
	}
	require.NoError(t, messageRepo.CreateAndTouch(ctx, linked, time.Now(), chat.SenderTypeAdmin))
	require.NoError(t, assetRepo.HideCatalog(ctx, chat.AssetScopeLibrary, catalog.ID))
	_, err = assetRepo.GetForUser(ctx, catalog.ID, userA.ID, conversationA.ID)
	require.NoError(t, err, "hiding catalog entries must not break historical messages")
	_, err = assetRepo.GetForAdmin(ctx, catalog.ID, adminB.ID)
	require.NoError(t, err, "hiding catalog entries must not break admin access to historical messages")
	err = messageRepo.CreateAndTouch(ctx, &chat.Message{
		ConversationID: conversationA.ID, SenderType: chat.SenderTypeAdmin, SenderID: adminA.ID,
		Content: "[image]", Kind: chat.MessageKindImage, Metadata: []byte(`{}`), AssetIDs: []int64{catalog.ID},
	}, time.Now(), chat.SenderTypeAdmin)
	require.ErrorIs(t, err, chat.ErrMessageAssetsInvalid, "hidden catalog entries must not be newly attached")
}

func TestChatRecallAndManualUnreadStateAreDurableAndAuthorizationAware(t *testing.T) {
	baseCtx := context.Background()
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(baseCtx, tx)
	client := tx.Client()

	user := createEntUser(t, ctx, client, uniqueSoftDeleteValue(t, "chat-recall-user")+"@example.test")
	admin := createEntUser(t, ctx, client, uniqueSoftDeleteValue(t, "chat-recall-admin")+"@example.test")
	conversationRepo := NewChatConversationRepository(client)
	messageRepo := NewChatMessageRepository(client)
	assetRepo := NewChatAssetRepository(client)
	conversation, err := conversationRepo.GetOrCreateByUserID(ctx, user.ID)
	require.NoError(t, err)

	message := &chat.Message{
		ConversationID: conversation.ID, SenderType: chat.SenderTypeAdmin, SenderID: admin.ID,
		Content: "mistaken private reply", Kind: chat.MessageKindText, Metadata: []byte(`{"audit":true}`),
	}
	require.NoError(t, messageRepo.CreateAndTouch(ctx, message, time.Now().UTC(), chat.SenderTypeAdmin))
	conversation, err = conversationRepo.GetByID(ctx, conversation.ID)
	require.NoError(t, err)
	require.Equal(t, 1, conversation.UnreadByUser)

	recalled, changed, err := messageRepo.RecallByAdmin(
		ctx,
		conversation.ID,
		message.ID,
		admin.ID,
		time.Now().UTC(),
	)
	require.NoError(t, err)
	require.True(t, changed)
	require.NotNil(t, recalled.RecalledAt)
	require.Empty(t, recalled.Content)
	require.JSONEq(t, `{}`, string(recalled.Metadata))
	require.Empty(t, recalled.Assets)

	conversation, err = conversationRepo.GetByID(ctx, conversation.ID)
	require.NoError(t, err)
	require.Zero(t, conversation.UnreadByUser, "recalling an unread admin message must remove its unread count")
	persisted, err := client.ChatMessage.Get(ctx, message.ID)
	require.NoError(t, err)
	require.Equal(t, "mistaken private reply", persisted.Content, "the original is retained only for server-side audit")

	recalled, changed, err = messageRepo.RecallByAdmin(ctx, conversation.ID, message.ID, admin.ID, time.Now().UTC())
	require.NoError(t, err)
	require.False(t, changed)
	require.Empty(t, recalled.Content)

	listed, _, err := messageRepo.List(ctx, conversation.ID, pagination.PaginationParams{Page: 1, PageSize: 20})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Empty(t, listed[0].Content, "delivery lists must never expose recalled content")
	replyID := message.ID
	err = messageRepo.CreateAndTouch(ctx, &chat.Message{
		ConversationID: conversation.ID, SenderType: chat.SenderTypeAdmin, SenderID: admin.ID,
		Content: "reply", Kind: chat.MessageKindText, Metadata: []byte(`{}`), ReplyToID: &replyID,
	}, time.Now().UTC(), chat.SenderTypeAdmin)
	require.ErrorIs(t, err, chat.ErrMessageReplyInvalid)

	userMessage := &chat.Message{
		ConversationID: conversation.ID, SenderType: chat.SenderTypeUser, SenderID: user.ID,
		Content: "user message", Kind: chat.MessageKindText, Metadata: []byte(`{}`),
	}
	require.NoError(t, messageRepo.CreateAndTouch(ctx, userMessage, time.Now().UTC(), chat.SenderTypeUser))
	_, _, err = messageRepo.RecallByAdmin(ctx, conversation.ID, userMessage.ID, admin.ID, time.Now().UTC())
	require.ErrorIs(t, err, chat.ErrMessageRecallNotAllowed)

	for name, search := range map[string]string{
		"email":           user.Email,
		"user id":         fmt.Sprint(user.ID),
		"conversation id": fmt.Sprint(conversation.ID),
		"message content": "user message",
	} {
		t.Run("search by "+name, func(t *testing.T) {
			items, _, searchErr := conversationRepo.List(
				ctx,
				pagination.PaginationParams{Page: 1, PageSize: 20},
				chat.ConversationListFilters{Search: search},
			)
			require.NoError(t, searchErr)
			require.Len(t, items, 1)
			require.Equal(t, conversation.ID, items[0].ID)
		})
	}
	recalledMatches, _, err := conversationRepo.List(
		ctx,
		pagination.PaginationParams{Page: 1, PageSize: 20},
		chat.ConversationListFilters{Search: "mistaken private reply"},
	)
	require.NoError(t, err)
	require.Empty(t, recalledMatches, "recalled message payloads must not remain searchable")

	_, _, err = conversationRepo.MarkRead(ctx, conversation.ID, chat.SenderTypeAdmin)
	require.NoError(t, err)
	changed, err = conversationRepo.MarkUnreadByAdmin(ctx, conversation.ID)
	require.NoError(t, err)
	require.True(t, changed)
	count, err := conversationRepo.CountUnreadByAdmin(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, count)
	unread, _, err := conversationRepo.List(
		ctx,
		pagination.PaginationParams{Page: 1, PageSize: 20},
		chat.ConversationListFilters{UnreadOnly: true},
	)
	require.NoError(t, err)
	require.Len(t, unread, 1)
	require.True(t, unread[0].ManuallyUnreadByAdmin)
	_, _, err = conversationRepo.MarkRead(ctx, conversation.ID, chat.SenderTypeAdmin)
	require.NoError(t, err)
	count, err = conversationRepo.CountUnreadByAdmin(ctx)
	require.NoError(t, err)
	require.Zero(t, count)

	asset := &chat.Asset{
		Scope: chat.AssetScopeMessage, ConversationID: &conversation.ID, UploadedBy: &admin.ID,
		Name: "recall.png", MIMEType: "image/png", Size: 3, Data: []byte("png"),
	}
	require.NoError(t, assetRepo.Create(ctx, asset))
	imageMessage := &chat.Message{
		ConversationID: conversation.ID, SenderType: chat.SenderTypeAdmin, SenderID: admin.ID,
		Content: "[image]", Kind: chat.MessageKindImage, Metadata: []byte(`{}`), AssetIDs: []int64{asset.ID},
	}
	require.NoError(t, messageRepo.CreateAndTouch(ctx, imageMessage, time.Now().UTC(), chat.SenderTypeAdmin))
	_, err = assetRepo.GetForUser(ctx, asset.ID, user.ID, conversation.ID)
	require.NoError(t, err)
	_, _, err = messageRepo.RecallByAdmin(ctx, conversation.ID, imageMessage.ID, admin.ID, time.Now().UTC())
	require.NoError(t, err)
	_, err = assetRepo.GetForUser(ctx, asset.ID, user.ID, conversation.ID)
	require.ErrorIs(t, err, chat.ErrAssetNotFound, "recalled image payloads must no longer be downloadable by the user")
}

func TestChatRetentionCleanupDeletesOrdinaryHistoryAndReconcilesConversationState(t *testing.T) {
	baseCtx := context.Background()
	tx := testEntTx(t)
	ctx := dbent.NewTxContext(baseCtx, tx)
	client := tx.Client()

	user := createEntUser(t, ctx, client, uniqueSoftDeleteValue(t, "chat-retention-user")+"@example.test")
	emptyUser := createEntUser(t, ctx, client, uniqueSoftDeleteValue(t, "chat-retention-empty")+"@example.test")
	admin := createEntUser(t, ctx, client, uniqueSoftDeleteValue(t, "chat-retention-admin")+"@example.test")
	conversationRepo := NewChatConversationRepository(client)
	messageRepo := NewChatMessageRepository(client)
	assetRepo := NewChatAssetRepository(client)
	chatService := chat.NewService(conversationRepo, messageRepo)
	chatService.SetAssetRepository(assetRepo)

	conversation, err := conversationRepo.GetOrCreateByUserID(ctx, user.ID)
	require.NoError(t, err)
	emptyConversation, err := conversationRepo.GetOrCreateByUserID(ctx, emptyUser.ID)
	require.NoError(t, err)

	now := time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)
	cutoff := now.Add(-30 * 24 * time.Hour)
	oldAt := now.Add(-40 * 24 * time.Hour)
	readAt := now.Add(-20 * 24 * time.Hour)
	freshAt := now.Add(-24 * time.Hour)
	setMessageCreatedAt := func(messageID int64, createdAt time.Time) {
		_, updateErr := client.ExecContext(ctx, "UPDATE chat_messages SET created_at = $1 WHERE id = $2", createdAt, messageID)
		require.NoError(t, updateErr)
	}

	oldUserMessage := &chat.Message{
		ConversationID: conversation.ID, SenderType: chat.SenderTypeUser, SenderID: user.ID,
		Content: "expired user text", Kind: chat.MessageKindText, Metadata: []byte(`{}`),
	}
	require.NoError(t, messageRepo.CreateAndTouch(ctx, oldUserMessage, oldAt, chat.SenderTypeUser))
	setMessageCreatedAt(oldUserMessage.ID, oldAt)

	oldAsset := &chat.Asset{
		Scope: chat.AssetScopeMessage, ConversationID: &conversation.ID, UploadedBy: &admin.ID,
		Name: "expired.png", MIMEType: "image/png", Size: 3, Data: []byte("png"),
	}
	require.NoError(t, assetRepo.Create(ctx, oldAsset))
	_, err = client.ExecContext(ctx, "UPDATE chat_assets SET created_at = $1 WHERE id = $2", oldAt, oldAsset.ID)
	require.NoError(t, err)
	oldImageMessage := &chat.Message{
		ConversationID: conversation.ID, SenderType: chat.SenderTypeAdmin, SenderID: admin.ID,
		Content: "[image]", Kind: chat.MessageKindImage, Metadata: []byte(`{}`), AssetIDs: []int64{oldAsset.ID},
	}
	require.NoError(t, messageRepo.CreateAndTouch(ctx, oldImageMessage, oldAt, chat.SenderTypeAdmin))
	setMessageCreatedAt(oldImageMessage.ID, oldAt)

	transferKey := "retention-transfer-key"
	transferReceipt := &chat.Message{
		ConversationID: conversation.ID, SenderType: chat.SenderTypeAdmin, SenderID: admin.ID,
		Content: "Balance credited: 10", Kind: chat.MessageKindBalanceTransfer,
		Metadata: []byte(`{"amount":10}`), IdempotencyKey: &transferKey,
	}
	require.NoError(t, messageRepo.CreateAndTouch(ctx, transferReceipt, oldAt, chat.SenderTypeAdmin))
	setMessageCreatedAt(transferReceipt.ID, oldAt)

	freshUserMessage := &chat.Message{
		ConversationID: conversation.ID, SenderType: chat.SenderTypeUser, SenderID: user.ID,
		Content: "recent question", Kind: chat.MessageKindText, Metadata: []byte(`{}`),
	}
	require.NoError(t, messageRepo.CreateAndTouch(ctx, freshUserMessage, freshAt, chat.SenderTypeUser))
	setMessageCreatedAt(freshUserMessage.ID, freshAt)

	_, err = client.ChatConversation.UpdateOneID(conversation.ID).
		SetLastReadByAdminAt(readAt).
		SetLastReadByUserAt(readAt).
		SetUnreadByAdmin(99).
		SetUnreadByUser(99).
		SetManuallyUnreadByAdmin(true).
		Save(ctx)
	require.NoError(t, err)

	onlyOldMessage := &chat.Message{
		ConversationID: emptyConversation.ID, SenderType: chat.SenderTypeUser, SenderID: emptyUser.ID,
		Content: "only expired message", Kind: chat.MessageKindText, Metadata: []byte(`{}`),
	}
	require.NoError(t, messageRepo.CreateAndTouch(ctx, onlyOldMessage, oldAt, chat.SenderTypeUser))
	setMessageCreatedAt(onlyOldMessage.ID, oldAt)
	_, err = client.ChatConversation.UpdateOneID(emptyConversation.ID).
		SetUnreadByAdmin(5).
		SetManuallyUnreadByAdmin(true).
		Save(ctx)
	require.NoError(t, err)

	result, err := chatService.CleanupExpiredMessages(ctx, cutoff, 100)
	require.NoError(t, err)
	require.Equal(t, chat.RetentionCleanupResult{MessagesDeleted: 3, AssetsDeleted: 1}, result)

	for _, deletedID := range []int64{oldUserMessage.ID, oldImageMessage.ID, onlyOldMessage.ID} {
		exists, queryErr := client.ChatMessage.Query().Where(chatmessage.IDEQ(deletedID)).Exist(ctx)
		require.NoError(t, queryErr)
		require.False(t, exists)
	}
	for _, retainedID := range []int64{transferReceipt.ID, freshUserMessage.ID} {
		exists, queryErr := client.ChatMessage.Query().Where(chatmessage.IDEQ(retainedID)).Exist(ctx)
		require.NoError(t, queryErr)
		require.True(t, exists)
	}
	assetExists, err := client.ChatAsset.Query().Where(chatasset.IDEQ(oldAsset.ID)).Exist(ctx)
	require.NoError(t, err)
	require.False(t, assetExists)

	conversation, err = conversationRepo.GetByID(ctx, conversation.ID)
	require.NoError(t, err)
	require.NotNil(t, conversation.LastMessageAt)
	require.WithinDuration(t, freshAt, *conversation.LastMessageAt, time.Second)
	require.Equal(t, 1, conversation.UnreadByAdmin)
	require.Zero(t, conversation.UnreadByUser)
	require.True(t, conversation.ManuallyUnreadByAdmin)

	emptyConversation, err = conversationRepo.GetByID(ctx, emptyConversation.ID)
	require.NoError(t, err)
	require.Nil(t, emptyConversation.LastMessageAt)
	require.Zero(t, emptyConversation.UnreadByAdmin)
	require.Zero(t, emptyConversation.UnreadByUser)
	require.False(t, emptyConversation.ManuallyUnreadByAdmin)
}

func TestChatQuickReplyOwnershipAndConcurrentLimit(t *testing.T) {
	ctx := context.Background()
	client := testEntClient(t)
	admin, err := client.User.Create().
		SetEmail(fmt.Sprintf("chat-quick-%d@example.test", time.Now().UnixNano())).
		SetPasswordHash("test-password-hash").
		Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM users WHERE id = $1", admin.ID)
	})
	otherAdmin, err := client.User.Create().
		SetEmail(fmt.Sprintf("chat-quick-other-%d@example.test", time.Now().UnixNano())).
		SetPasswordHash("test-password-hash").
		Save(ctx)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = integrationDB.ExecContext(context.Background(), "DELETE FROM users WHERE id = $1", otherAdmin.ID)
	})

	repo := NewChatQuickReplyRepository(client)
	first := &chat.QuickReply{AdminID: admin.ID, Title: "first", Content: "content"}
	require.NoError(t, repo.Create(ctx, first))
	_, err = repo.GetByID(ctx, otherAdmin.ID, first.ID)
	require.ErrorIs(t, err, chat.ErrQuickReplyNotFound)
	require.ErrorIs(t, repo.Delete(ctx, otherAdmin.ID, first.ID), chat.ErrQuickReplyNotFound)

	const concurrentCreates = 60
	results := make(chan error, concurrentCreates)
	var wg sync.WaitGroup
	for i := 0; i < concurrentCreates; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results <- repo.Create(ctx, &chat.QuickReply{
				AdminID: admin.ID, Title: fmt.Sprintf("reply-%d", index), Content: "content",
			})
		}(i)
	}
	wg.Wait()
	close(results)

	successes := 0
	limitErrors := 0
	for createErr := range results {
		switch {
		case createErr == nil:
			successes++
		case createErr == chat.ErrQuickReplyLimitReached:
			limitErrors++
		default:
			require.NoError(t, createErr)
		}
	}
	require.Equal(t, chat.MaxQuickReplies-1, successes)
	require.Equal(t, concurrentCreates-successes, limitErrors)
	items, err := repo.ListByAdminID(ctx, admin.ID)
	require.NoError(t, err)
	require.Len(t, items, chat.MaxQuickReplies)
}

//go:build integration

package repository

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
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

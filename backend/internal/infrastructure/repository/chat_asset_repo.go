package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/chatasset"
	"github.com/Wei-Shaw/sub2api/ent/chatconversation"
	"github.com/Wei-Shaw/sub2api/ent/chatmessage"
	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
)

type chatAssetRepository struct {
	client *dbent.Client
}

func NewChatAssetRepository(client *dbent.Client) chat.AssetRepository {
	return &chatAssetRepository{client: client}
}

func (r *chatAssetRepository) Create(ctx context.Context, asset *chat.Asset) error {
	created, err := clientFromContext(ctx, r.client).ChatAsset.Create().
		SetScope(chatasset.Scope(asset.Scope)).
		SetNillableConversationID(asset.ConversationID).
		SetNillableUploadedBy(asset.UploadedBy).
		SetName(asset.Name).
		SetMimeType(asset.MIMEType).
		SetSize(asset.Size).
		SetData(asset.Data).
		SetCollection(asset.Collection).
		SetCatalogVisible(asset.CatalogVisible).
		Save(ctx)
	if err != nil {
		return err
	}
	asset.ID = created.ID
	asset.CreatedAt = created.CreatedAt
	return nil
}

func (r *chatAssetRepository) ListCatalog(ctx context.Context, scope chat.AssetScope, limit int) ([]chat.Asset, error) {
	items, err := clientFromContext(ctx, r.client).ChatAsset.Query().
		Where(
			chatasset.ScopeEQ(chatasset.Scope(scope)),
			chatasset.CatalogVisibleEQ(true),
		).
		Select(chatAssetMetadataFields()...).
		Order(dbent.Desc(chatasset.FieldCreatedAt), dbent.Desc(chatasset.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]chat.Asset, 0, len(items))
	for _, item := range items {
		out = append(out, *chatAssetEntityToDomain(item, false))
	}
	return out, nil
}

func (r *chatAssetRepository) HideCatalog(ctx context.Context, scope chat.AssetScope, id int64) error {
	affected, err := clientFromContext(ctx, r.client).ChatAsset.Update().
		Where(
			chatasset.IDEQ(id),
			chatasset.ScopeEQ(chatasset.Scope(scope)),
			chatasset.CatalogVisibleEQ(true),
		).
		SetCatalogVisible(false).
		Save(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return chat.ErrAssetNotFound
	}
	return nil
}

func (r *chatAssetRepository) GetForUser(ctx context.Context, id, userID, conversationID int64) (*chat.Asset, error) {
	item, err := clientFromContext(ctx, r.client).ChatAsset.Query().
		Where(
			chatasset.IDEQ(id),
			chatasset.Or(
				chatasset.And(
					chatasset.ScopeEQ(chatasset.ScopeMessage),
					chatasset.ConversationIDEQ(conversationID),
					chatasset.UploadedByEQ(userID),
				),
				chatasset.HasMessagesWith(
					chatmessage.HasConversationWith(chatconversation.UserIDEQ(userID)),
				),
			),
		).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, chat.ErrAssetNotFound, nil)
	}
	return chatAssetEntityToDomain(item, true), nil
}

func (r *chatAssetRepository) GetForAdmin(ctx context.Context, id, adminID int64) (*chat.Asset, error) {
	item, err := clientFromContext(ctx, r.client).ChatAsset.Query().
		Where(
			chatasset.IDEQ(id),
			chatasset.Or(
				// Every administrator may read content that was actually sent.
				chatasset.HasMessages(),
				// A pending message upload is private to the administrator who
				// uploaded it until it is attached to a message.
				chatasset.And(
					chatasset.ScopeEQ(chatasset.ScopeMessage),
					chatasset.UploadedByEQ(adminID),
				),
				// Visible catalog entries are shared by all support agents.
				chatasset.And(
					chatasset.ScopeIn(chatasset.ScopeLibrary, chatasset.ScopeSticker),
					chatasset.CatalogVisibleEQ(true),
				),
			),
		).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, chat.ErrAssetNotFound, nil)
	}
	return chatAssetEntityToDomain(item, true), nil
}

func (r *chatAssetRepository) DeleteUnattachedBefore(ctx context.Context, before time.Time, limit int) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	const query = `
		WITH stale AS (
			SELECT a.id
			FROM chat_assets AS a
			WHERE a.scope = 'message'
			  AND a.catalog_visible = FALSE
			  AND a.created_at < $1
			  AND NOT EXISTS (
				SELECT 1 FROM chat_message_assets AS ma WHERE ma.asset_id = a.id
			  )
			ORDER BY a.created_at, a.id
			LIMIT $2
		)
		DELETE FROM chat_assets AS a
		USING stale
		WHERE a.id = stale.id
	`
	result, err := clientFromContext(ctx, r.client).ExecContext(ctx, query, before, limit)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	return int(affected), err
}

func chatAssetMetadataFields() []string {
	return []string{
		chatasset.FieldID,
		chatasset.FieldScope,
		chatasset.FieldConversationID,
		chatasset.FieldUploadedBy,
		chatasset.FieldName,
		chatasset.FieldMimeType,
		chatasset.FieldSize,
		chatasset.FieldCollection,
		chatasset.FieldCatalogVisible,
		chatasset.FieldCreatedAt,
	}
}

func chatAssetEntityToDomain(item *dbent.ChatAsset, includeData bool) *chat.Asset {
	if item == nil {
		return nil
	}
	asset := &chat.Asset{
		ID:             item.ID,
		Scope:          chat.AssetScope(item.Scope),
		ConversationID: item.ConversationID,
		UploadedBy:     item.UploadedBy,
		Name:           item.Name,
		MIMEType:       item.MimeType,
		Size:           item.Size,
		Collection:     item.Collection,
		CatalogVisible: item.CatalogVisible,
		CreatedAt:      item.CreatedAt,
	}
	if includeData {
		asset.Data = item.Data
	}
	return asset
}

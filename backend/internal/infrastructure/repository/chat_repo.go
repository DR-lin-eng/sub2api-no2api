package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/chatasset"
	"github.com/Wei-Shaw/sub2api/ent/chatconversation"
	"github.com/Wei-Shaw/sub2api/ent/chatmessage"
	"github.com/Wei-Shaw/sub2api/ent/chatmessageasset"
	"github.com/Wei-Shaw/sub2api/ent/predicate"
	"github.com/Wei-Shaw/sub2api/ent/user"
	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
)

type chatConversationRepository struct {
	client *dbent.Client
}

func NewChatConversationRepository(client *dbent.Client) chat.ConversationRepository {
	return &chatConversationRepository{client: client}
}

func (r *chatConversationRepository) GetOrCreateByUserID(ctx context.Context, userID int64) (*chat.Conversation, error) {
	client := clientFromContext(ctx, r.client)

	existing, err := client.ChatConversation.Query().
		Where(chatconversation.UserIDEQ(userID)).
		Only(ctx)
	if err == nil {
		return chatConversationEntityToDomain(existing), nil
	}
	if !dbent.IsNotFound(err) {
		return nil, err
	}

	created, err := client.ChatConversation.Create().
		SetUserID(userID).
		OnConflictColumns(chatconversation.FieldUserID).
		UpdateNewValues().
		ID(ctx)
	if err != nil {
		return nil, err
	}

	m, err := client.ChatConversation.Get(ctx, created)
	if err != nil {
		return nil, translatePersistenceError(err, chat.ErrConversationNotFound, nil)
	}
	return chatConversationEntityToDomain(m), nil
}

func (r *chatConversationRepository) GetByID(ctx context.Context, id int64) (*chat.Conversation, error) {
	m, err := clientFromContext(ctx, r.client).ChatConversation.Get(ctx, id)
	if err != nil {
		return nil, translatePersistenceError(err, chat.ErrConversationNotFound, nil)
	}
	return chatConversationEntityToDomain(m), nil
}

func (r *chatConversationRepository) GetByUserID(ctx context.Context, userID int64) (*chat.Conversation, error) {
	m, err := clientFromContext(ctx, r.client).ChatConversation.Query().
		Where(chatconversation.UserIDEQ(userID)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, chat.ErrConversationNotFound, nil)
	}
	return chatConversationEntityToDomain(m), nil
}

func (r *chatConversationRepository) List(
	ctx context.Context,
	params pagination.PaginationParams,
	filters chat.ConversationListFilters,
) ([]chat.Conversation, *pagination.PaginationResult, error) {
	q := clientFromContext(ctx, r.client).ChatConversation.Query()

	if filters.UnreadOnly {
		q = q.Where(chatConversationUnreadByAdminPredicate())
	}
	if search := strings.TrimSpace(filters.Search); search != "" {
		q = q.Where(chatconversation.HasUserWith(
			user.Or(
				user.EmailContainsFold(search),
				user.UsernameContainsFold(search),
			),
		))
	}

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	items, err := q.
		WithUser(func(uq *dbent.UserQuery) {
			uq.Select(user.FieldEmail, user.FieldUsername)
		}).
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(
			dbent.Desc(chatconversation.FieldLastMessageAt),
			dbent.Desc(chatconversation.FieldID),
		).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}

	out := make([]chat.Conversation, 0, len(items))
	for i := range items {
		conv := *chatConversationEntityToDomain(items[i])
		if u := items[i].Edges.User; u != nil {
			conv.UserEmail = u.Email
			conv.UserUsername = u.Username
		}
		out = append(out, conv)
	}
	return out, paginationResultFromTotal(int64(total), params), nil
}

func (r *chatConversationRepository) CountUnreadByAdmin(ctx context.Context) (int, error) {
	count, err := clientFromContext(ctx, r.client).ChatConversation.Query().
		Where(chatConversationUnreadByAdminPredicate()).
		Count(ctx)
	return count, err
}

func chatConversationUnreadByAdminPredicate() predicate.ChatConversation {
	return chatconversation.Or(
		chatconversation.UnreadByAdminGT(0),
		chatconversation.ManuallyUnreadByAdminEQ(true),
	)
}

func (r *chatConversationRepository) GetUnreadByUserID(ctx context.Context, userID int64) (int, error) {
	conversation, err := clientFromContext(ctx, r.client).ChatConversation.Query().
		Where(chatconversation.UserIDEQ(userID)).
		Only(ctx)
	if dbent.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return conversation.UnreadByUser, nil
}

func (r *chatConversationRepository) MarkRead(
	ctx context.Context,
	conversationID int64,
	sender chat.SenderType,
) (time.Time, bool, error) {
	client := clientFromContext(ctx, r.client)
	readAt := time.Now().UTC()
	builder := client.ChatConversation.Update().Where(chatconversation.IDEQ(conversationID))

	if sender == chat.SenderTypeUser {
		builder = builder.
			Where(chatconversation.Or(
				chatconversation.UnreadByUserGT(0),
				chatconversation.LastReadByUserAtIsNil(),
			)).
			SetUnreadByUser(0).
			SetLastReadByUserAt(readAt)
	} else {
		builder = builder.
			Where(chatconversation.Or(
				chatconversation.UnreadByAdminGT(0),
				chatconversation.ManuallyUnreadByAdminEQ(true),
				chatconversation.LastReadByAdminAtIsNil(),
			)).
			SetUnreadByAdmin(0).
			SetManuallyUnreadByAdmin(false).
			SetLastReadByAdminAt(readAt)
	}

	affected, err := builder.Save(ctx)
	if err != nil {
		return time.Time{}, false, translatePersistenceError(err, chat.ErrConversationNotFound, nil)
	}
	return readAt, affected > 0, nil
}

func (r *chatConversationRepository) MarkUnreadByAdmin(ctx context.Context, conversationID int64) (bool, error) {
	affected, err := clientFromContext(ctx, r.client).ChatConversation.Update().
		Where(
			chatconversation.IDEQ(conversationID),
			chatconversation.ManuallyUnreadByAdminEQ(false),
		).
		SetManuallyUnreadByAdmin(true).
		Save(ctx)
	if err != nil {
		return false, translatePersistenceError(err, chat.ErrConversationNotFound, nil)
	}
	return affected > 0, nil
}

func chatConversationEntityToDomain(m *dbent.ChatConversation) *chat.Conversation {
	if m == nil {
		return nil
	}
	return &chat.Conversation{
		ID:                    m.ID,
		UserID:                m.UserID,
		LastMessageAt:         m.LastMessageAt,
		UnreadByUser:          m.UnreadByUser,
		UnreadByAdmin:         m.UnreadByAdmin,
		ManuallyUnreadByAdmin: m.ManuallyUnreadByAdmin,
		LastReadByUserAt:      m.LastReadByUserAt,
		LastReadByAdminAt:     m.LastReadByAdminAt,
		CreatedAt:             m.CreatedAt,
		UpdatedAt:             m.UpdatedAt,
	}
}

type chatMessageRepository struct {
	client *dbent.Client
}

func NewChatMessageRepository(client *dbent.Client) chat.MessageRepository {
	return &chatMessageRepository{client: client}
}

// CreateAndTouch keeps the message row and the recipient unread counter in a
// single transaction. The context-aware transaction branch lets callers that
// already own an Ent transaction reuse it without nesting transactions.
func (r *chatMessageRepository) CreateAndTouch(ctx context.Context, m *chat.Message, at time.Time, sender chat.SenderType) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return r.createAndTouchWithClient(ctx, tx.Client(), m, at, sender)
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return fmt.Errorf("begin chat message transaction: %w", err)
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	if err := r.createAndTouchWithClient(txCtx, tx.Client(), m, at, sender); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit chat message transaction: %w", err)
	}
	return nil
}

// RecallByAdmin marks an administrator-authored message as recalled and, when
// the user had not read it yet, decrements the real unread counter in the same
// transaction. The original content remains in PostgreSQL for server-side
// audit, but every returned delivery view is redacted.
func (r *chatMessageRepository) RecallByAdmin(
	ctx context.Context,
	conversationID, messageID, recalledByID int64,
	at time.Time,
) (*chat.Message, bool, error) {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		return r.recallByAdminWithClient(ctx, tx.Client(), conversationID, messageID, recalledByID, at)
	}

	tx, err := r.client.Tx(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("begin chat recall transaction: %w", err)
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	message, changed, err := r.recallByAdminWithClient(
		txCtx,
		tx.Client(),
		conversationID,
		messageID,
		recalledByID,
		at,
	)
	if err != nil {
		_ = tx.Rollback()
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit chat recall transaction: %w", err)
	}
	return message, changed, nil
}

// DeleteExpiredBefore deletes a bounded batch and reconciles every affected
// conversation in the same statement. The candidates CTE is explicitly
// excluded from the summary aggregation because PostgreSQL data-modifying CTEs
// share one MVCC snapshot. FOR UPDATE SKIP LOCKED keeps the operation safe if a
// maintenance cycle overlaps during a rolling multi-instance deployment.
func (r *chatMessageRepository) DeleteExpiredBefore(
	ctx context.Context,
	before time.Time,
	limit int,
) (int, error) {
	if before.IsZero() || limit <= 0 {
		return 0, nil
	}
	const query = `
		WITH candidates AS MATERIALIZED (
			SELECT id, conversation_id
			FROM chat_messages
			WHERE created_at < $1
			  AND kind <> 'balance_transfer'
			ORDER BY created_at, id
			FOR UPDATE SKIP LOCKED
			LIMIT $2
		),
		deleted AS (
			DELETE FROM chat_messages AS message
			USING candidates
			WHERE message.id = candidates.id
			RETURNING message.conversation_id
		),
		affected AS (
			SELECT DISTINCT conversation_id FROM deleted
		),
		remaining AS (
			SELECT
				affected.conversation_id,
				MAX(message.created_at) AS last_message_at,
				COUNT(*) FILTER (
					WHERE message.sender_type = 'user'
					  AND message.recalled_at IS NULL
					  AND (
						conversation.last_read_by_admin_at IS NULL
						OR message.created_at > conversation.last_read_by_admin_at
					  )
				)::INT AS unread_by_admin,
				COUNT(*) FILTER (
					WHERE message.sender_type = 'admin'
					  AND message.recalled_at IS NULL
					  AND (
						conversation.last_read_by_user_at IS NULL
						OR message.created_at > conversation.last_read_by_user_at
					  )
				)::INT AS unread_by_user
			FROM affected
			JOIN chat_conversations AS conversation
			  ON conversation.id = affected.conversation_id
			LEFT JOIN chat_messages AS message
			  ON message.conversation_id = affected.conversation_id
			 AND NOT EXISTS (
				SELECT 1 FROM candidates WHERE candidates.id = message.id
			 )
			GROUP BY affected.conversation_id
		),
		reconciled AS (
			UPDATE chat_conversations AS conversation
			SET last_message_at = remaining.last_message_at,
				unread_by_admin = remaining.unread_by_admin,
				unread_by_user = remaining.unread_by_user,
				manually_unread_by_admin = CASE
					WHEN remaining.last_message_at IS NULL THEN FALSE
					ELSE conversation.manually_unread_by_admin
				END,
				updated_at = NOW()
			FROM remaining
			WHERE conversation.id = remaining.conversation_id
			RETURNING conversation.id
		)
		SELECT
			(SELECT COUNT(*) FROM deleted),
			(SELECT COUNT(*) FROM reconciled)
	`
	rows, err := clientFromContext(ctx, r.client).QueryContext(ctx, query, before.UTC(), limit)
	if err != nil {
		return 0, err
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, nil
	}
	var deleted, reconciled int
	if err := rows.Scan(&deleted, &reconciled); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (r *chatMessageRepository) recallByAdminWithClient(
	ctx context.Context,
	client *dbent.Client,
	conversationID, messageID, recalledByID int64,
	at time.Time,
) (*chat.Message, bool, error) {
	item, err := client.ChatMessage.Query().
		Where(
			chatmessage.IDEQ(messageID),
			chatmessage.ConversationIDEQ(conversationID),
		).
		Only(ctx)
	if err != nil {
		return nil, false, translatePersistenceError(err, chat.ErrMessageNotFound, nil)
	}
	if item.SenderType != chatmessage.SenderTypeAdmin || item.Kind == chatmessage.KindBalanceTransfer {
		return nil, false, chat.ErrMessageRecallNotAllowed
	}
	if item.RecalledAt != nil {
		return chatMessageEntityToDomain(item, true), false, nil
	}

	at = at.UTC()
	affected, err := client.ChatMessage.Update().
		Where(
			chatmessage.IDEQ(messageID),
			chatmessage.ConversationIDEQ(conversationID),
			chatmessage.RecalledAtIsNil(),
		).
		SetRecalledAt(at).
		SetRecalledBy(recalledByID).
		Save(ctx)
	if err != nil {
		return nil, false, err
	}
	if affected == 0 {
		current, queryErr := client.ChatMessage.Query().
			Where(
				chatmessage.IDEQ(messageID),
				chatmessage.ConversationIDEQ(conversationID),
			).
			Only(ctx)
		if queryErr != nil {
			return nil, false, translatePersistenceError(queryErr, chat.ErrMessageNotFound, nil)
		}
		return chatMessageEntityToDomain(current, true), false, nil
	}

	if _, err := client.ChatConversation.Update().
		Where(
			chatconversation.IDEQ(conversationID),
			chatconversation.UnreadByUserGT(0),
			chatconversation.Or(
				chatconversation.LastReadByUserAtIsNil(),
				chatconversation.LastReadByUserAtLT(item.CreatedAt),
			),
		).
		AddUnreadByUser(-1).
		Save(ctx); err != nil {
		return nil, false, err
	}

	item.RecalledAt = &at
	item.RecalledBy = &recalledByID
	return chatMessageEntityToDomain(item, true), true, nil
}

func (r *chatMessageRepository) createAndTouchWithClient(
	ctx context.Context,
	client *dbent.Client,
	m *chat.Message,
	at time.Time,
	sender chat.SenderType,
) error {
	if m.ReplyToID != nil {
		exists, err := client.ChatMessage.Query().
			Where(
				chatmessage.IDEQ(*m.ReplyToID),
				chatmessage.ConversationIDEQ(m.ConversationID),
				chatmessage.RecalledAtIsNil(),
			).
			Exist(ctx)
		if err != nil {
			return err
		}
		if !exists {
			return chat.ErrMessageReplyInvalid
		}
	}

	assets, err := loadMessageAssetsForCreate(ctx, client, m, sender)
	if err != nil {
		return err
	}

	create := client.ChatMessage.Create().
		SetConversationID(m.ConversationID).
		SetSenderType(chatmessage.SenderType(m.SenderType)).
		SetSenderID(m.SenderID).
		SetContent(m.Content).
		SetKind(chatmessage.Kind(m.Kind)).
		SetNillableReplyToID(m.ReplyToID).
		SetMetadata(m.Metadata).
		SetNillableIdempotencyKey(m.IdempotencyKey)
	created, err := create.Save(ctx)
	if err != nil {
		return err
	}
	m.ID = created.ID
	m.CreatedAt = created.CreatedAt
	m.Assets = assets

	if len(assets) > 0 {
		builders := make([]*dbent.ChatMessageAssetCreate, 0, len(assets))
		for i := range assets {
			builders = append(builders, client.ChatMessageAsset.Create().
				SetMessageID(created.ID).
				SetAssetID(assets[i].ID).
				SetSortOrder(i))
		}
		if _, err := client.ChatMessageAsset.CreateBulk(builders...).Save(ctx); err != nil {
			return err
		}
	}

	builder := client.ChatConversation.UpdateOneID(m.ConversationID).
		SetLastMessageAt(at)
	if sender == chat.SenderTypeUser {
		builder = builder.AddUnreadByAdmin(1)
	} else {
		builder = builder.AddUnreadByUser(1)
	}
	if _, err := builder.Save(ctx); err != nil {
		return translatePersistenceError(err, chat.ErrConversationNotFound, nil)
	}
	return nil
}

func loadMessageAssetsForCreate(
	ctx context.Context,
	client *dbent.Client,
	m *chat.Message,
	sender chat.SenderType,
) ([]chat.Asset, error) {
	if len(m.AssetIDs) == 0 {
		return nil, nil
	}
	predicates := []predicate.ChatAsset{
		chatasset.IDIn(m.AssetIDs...),
	}
	if sender == chat.SenderTypeUser {
		predicates = append(predicates,
			chatasset.ScopeEQ(chatasset.ScopeMessage),
			chatasset.ConversationIDEQ(m.ConversationID),
			chatasset.UploadedByEQ(m.SenderID),
		)
	} else {
		predicates = append(predicates, chatasset.Or(
			chatasset.And(
				chatasset.ScopeEQ(chatasset.ScopeMessage),
				chatasset.ConversationIDEQ(m.ConversationID),
				chatasset.UploadedByEQ(m.SenderID),
			),
			chatasset.And(
				chatasset.ScopeIn(chatasset.ScopeLibrary, chatasset.ScopeSticker),
				chatasset.CatalogVisibleEQ(true),
			),
		))
	}
	items, err := client.ChatAsset.Query().
		Where(predicates...).
		Select(chatAssetMetadataFields()...).
		All(ctx)
	if err != nil {
		return nil, err
	}
	if len(items) != len(m.AssetIDs) {
		return nil, chat.ErrMessageAssetsInvalid
	}
	byID := make(map[int64]*dbent.ChatAsset, len(items))
	for _, item := range items {
		byID[item.ID] = item
	}
	assets := make([]chat.Asset, 0, len(m.AssetIDs))
	for _, id := range m.AssetIDs {
		item := byID[id]
		if item == nil || (m.Kind == chat.MessageKindSticker && item.Scope != chatasset.ScopeSticker) ||
			(m.Kind == chat.MessageKindImage && item.Scope == chatasset.ScopeSticker) {
			return nil, chat.ErrMessageAssetsInvalid
		}
		assets = append(assets, *chatAssetEntityToDomain(item, false))
	}
	return assets, nil
}

func (r *chatMessageRepository) List(
	ctx context.Context,
	conversationID int64,
	params pagination.PaginationParams,
) ([]chat.Message, *pagination.PaginationResult, error) {
	client := clientFromContext(ctx, r.client)
	q := client.ChatMessage.Query().
		Where(chatmessage.ConversationIDEQ(conversationID))

	total, err := q.Count(ctx)
	if err != nil {
		return nil, nil, err
	}

	items, err := q.
		Offset(params.Offset()).
		Limit(params.Limit()).
		Order(dbent.Desc(chatmessage.FieldCreatedAt), dbent.Desc(chatmessage.FieldID)).
		All(ctx)
	if err != nil {
		return nil, nil, err
	}
	if err := loadChatMessageAssets(ctx, client, items); err != nil {
		return nil, nil, err
	}

	out := make([]chat.Message, 0, len(items))
	for i := range items {
		out = append(out, *chatMessageEntityToDomain(items[i], true))
	}
	return out, paginationResultFromTotal(int64(total), params), nil
}

func (r *chatMessageRepository) GetByIdempotencyKey(
	ctx context.Context,
	sender chat.SenderType,
	senderID int64,
	key string,
) (*chat.Message, error) {
	client := clientFromContext(ctx, r.client)
	item, err := client.ChatMessage.Query().
		Where(
			chatmessage.SenderTypeEQ(chatmessage.SenderType(sender)),
			chatmessage.SenderIDEQ(senderID),
			chatmessage.IdempotencyKeyEQ(key),
		).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, chat.ErrMessageNotFound, nil)
	}
	if err := loadChatMessageAssets(ctx, client, []*dbent.ChatMessage{item}); err != nil {
		return nil, err
	}
	return chatMessageEntityToDomain(item, false), nil
}

func loadChatMessageAssets(ctx context.Context, client *dbent.Client, messages []*dbent.ChatMessage) error {
	if len(messages) == 0 {
		return nil
	}
	messageByID := make(map[int64]*dbent.ChatMessage, len(messages))
	ids := make([]int64, 0, len(messages))
	for _, message := range messages {
		if message == nil {
			continue
		}
		message.Edges.Assets = nil
		messageByID[message.ID] = message
		ids = append(ids, message.ID)
	}
	links, err := client.ChatMessageAsset.Query().
		Where(chatmessageasset.MessageIDIn(ids...)).
		WithAsset(func(query *dbent.ChatAssetQuery) {
			query.Select(chatAssetMetadataFields()...)
		}).
		Order(
			dbent.Asc(chatmessageasset.FieldMessageID),
			dbent.Asc(chatmessageasset.FieldSortOrder),
		).
		All(ctx)
	if err != nil {
		return err
	}
	for _, link := range links {
		if message := messageByID[link.MessageID]; message != nil && link.Edges.Asset != nil {
			message.Edges.Assets = append(message.Edges.Assets, link.Edges.Asset)
		}
	}
	return nil
}

func chatMessageEntityToDomain(item *dbent.ChatMessage, redactRecalled bool) *chat.Message {
	if item == nil {
		return nil
	}
	assets := make([]chat.Asset, 0, len(item.Edges.Assets))
	for _, asset := range item.Edges.Assets {
		assets = append(assets, *chatAssetEntityToDomain(asset, false))
	}
	message := &chat.Message{
		ID:             item.ID,
		ConversationID: item.ConversationID,
		SenderType:     chat.SenderType(item.SenderType),
		SenderID:       item.SenderID,
		Content:        item.Content,
		Kind:           chat.MessageKind(item.Kind),
		ReplyToID:      item.ReplyToID,
		Metadata:       item.Metadata,
		IdempotencyKey: item.IdempotencyKey,
		Assets:         assets,
		RecalledAt:     item.RecalledAt,
		RecalledBy:     item.RecalledBy,
		CreatedAt:      item.CreatedAt,
	}
	if redactRecalled && message.RecalledAt != nil {
		message.Content = ""
		message.Metadata = json.RawMessage(`{}`)
		message.Assets = nil
	}
	return message
}

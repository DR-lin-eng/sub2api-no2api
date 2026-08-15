package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/chatconversation"
	"github.com/Wei-Shaw/sub2api/ent/chatmessage"
	"github.com/Wei-Shaw/sub2api/ent/chatquickreply"
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
	m, err := r.client.ChatConversation.Get(ctx, id)
	if err != nil {
		return nil, translatePersistenceError(err, chat.ErrConversationNotFound, nil)
	}
	return chatConversationEntityToDomain(m), nil
}

func (r *chatConversationRepository) GetByUserID(ctx context.Context, userID int64) (*chat.Conversation, error) {
	m, err := r.client.ChatConversation.Query().
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
	q := r.client.ChatConversation.Query()

	if filters.UnreadOnly {
		q = q.Where(chatconversation.UnreadByAdminGT(0))
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
		Where(chatconversation.UnreadByAdminGT(0)).
		Count(ctx)
	return count, err
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

func (r *chatConversationRepository) MarkRead(ctx context.Context, conversationID int64, sender chat.SenderType) error {
	client := clientFromContext(ctx, r.client)
	builder := client.ChatConversation.UpdateOneID(conversationID)

	if sender == chat.SenderTypeUser {
		builder = builder.SetUnreadByUser(0)
	} else {
		builder = builder.SetUnreadByAdmin(0)
	}

	_, err := builder.Save(ctx)
	return translatePersistenceError(err, chat.ErrConversationNotFound, nil)
}

func chatConversationEntityToDomain(m *dbent.ChatConversation) *chat.Conversation {
	if m == nil {
		return nil
	}
	return &chat.Conversation{
		ID:            m.ID,
		UserID:        m.UserID,
		LastMessageAt: m.LastMessageAt,
		UnreadByUser:  m.UnreadByUser,
		UnreadByAdmin: m.UnreadByAdmin,
		CreatedAt:     m.CreatedAt,
		UpdatedAt:     m.UpdatedAt,
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

func (r *chatMessageRepository) createAndTouchWithClient(
	ctx context.Context,
	client *dbent.Client,
	m *chat.Message,
	at time.Time,
	sender chat.SenderType,
) error {
	created, err := client.ChatMessage.Create().
		SetConversationID(m.ConversationID).
		SetSenderType(chatmessage.SenderType(m.SenderType)).
		SetSenderID(m.SenderID).
		SetContent(m.Content).
		Save(ctx)
	if err != nil {
		return err
	}
	m.ID = created.ID
	m.CreatedAt = created.CreatedAt

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

func (r *chatMessageRepository) List(
	ctx context.Context,
	conversationID int64,
	params pagination.PaginationParams,
) ([]chat.Message, *pagination.PaginationResult, error) {
	q := r.client.ChatMessage.Query().
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

	out := make([]chat.Message, 0, len(items))
	for i := range items {
		out = append(out, chat.Message{
			ID:             items[i].ID,
			ConversationID: items[i].ConversationID,
			SenderType:     chat.SenderType(items[i].SenderType),
			SenderID:       items[i].SenderID,
			Content:        items[i].Content,
			CreatedAt:      items[i].CreatedAt,
		})
	}
	return out, paginationResultFromTotal(int64(total), params), nil
}

type chatQuickReplyRepository struct {
	client *dbent.Client
}

func NewChatQuickReplyRepository(client *dbent.Client) chat.QuickReplyRepository {
	return &chatQuickReplyRepository{client: client}
}

func (r *chatQuickReplyRepository) ListByAdminID(ctx context.Context, adminID int64) ([]chat.QuickReply, error) {
	items, err := clientFromContext(ctx, r.client).ChatQuickReply.Query().
		Where(chatquickreply.AdminIDEQ(adminID)).
		Order(dbent.Asc(chatquickreply.FieldSortOrder), dbent.Asc(chatquickreply.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]chat.QuickReply, 0, len(items))
	for i := range items {
		out = append(out, chat.QuickReply{
			ID:        items[i].ID,
			AdminID:   items[i].AdminID,
			Title:     items[i].Title,
			Content:   items[i].Content,
			SortOrder: items[i].SortOrder,
			CreatedAt: items[i].CreatedAt,
			UpdatedAt: items[i].UpdatedAt,
		})
	}
	return out, nil
}

func (r *chatQuickReplyRepository) Create(ctx context.Context, qr *chat.QuickReply) error {
	created, err := clientFromContext(ctx, r.client).ChatQuickReply.Create().
		SetAdminID(qr.AdminID).
		SetTitle(qr.Title).
		SetContent(qr.Content).
		SetSortOrder(qr.SortOrder).
		Save(ctx)
	if err != nil {
		return err
	}
	qr.ID = created.ID
	qr.CreatedAt = created.CreatedAt
	qr.UpdatedAt = created.UpdatedAt
	return nil
}

func (r *chatQuickReplyRepository) Update(ctx context.Context, qr *chat.QuickReply) error {
	updated, err := clientFromContext(ctx, r.client).ChatQuickReply.UpdateOneID(qr.ID).
		SetTitle(qr.Title).
		SetContent(qr.Content).
		Save(ctx)
	if err != nil {
		return translatePersistenceError(err, chat.ErrConversationNotFound, nil)
	}
	qr.UpdatedAt = updated.UpdatedAt
	return nil
}

func (r *chatQuickReplyRepository) Delete(ctx context.Context, adminID, id int64) error {
	deleted, err := clientFromContext(ctx, r.client).ChatQuickReply.Delete().
		Where(
			chatquickreply.IDEQ(id),
			chatquickreply.AdminIDEQ(adminID),
		).
		Exec(ctx)
	if err != nil {
		return err
	}
	if deleted == 0 {
		return chat.ErrConversationNotFound
	}
	return nil
}

func (r *chatQuickReplyRepository) GetByID(ctx context.Context, adminID, id int64) (*chat.QuickReply, error) {
	item, err := clientFromContext(ctx, r.client).ChatQuickReply.Query().
		Where(
			chatquickreply.IDEQ(id),
			chatquickreply.AdminIDEQ(adminID),
		).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, chat.ErrConversationNotFound, nil)
	}
	return &chat.QuickReply{
		ID:        item.ID,
		AdminID:   item.AdminID,
		Title:     item.Title,
		Content:   item.Content,
		SortOrder: item.SortOrder,
		CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt,
	}, nil
}

func (r *chatQuickReplyRepository) UpdateSortOrders(ctx context.Context, adminID int64, idOrderMap map[int64]int) error {
	client := clientFromContext(ctx, r.client)

	for id, order := range idOrderMap {
		if _, err := client.ChatQuickReply.Update().
			Where(
				chatquickreply.IDEQ(id),
				chatquickreply.AdminIDEQ(adminID),
			).
			SetSortOrder(order).
			Save(ctx); err != nil {
			return err
		}
	}
	return nil
}

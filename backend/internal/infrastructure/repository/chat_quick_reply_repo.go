package repository

import (
	"context"
	"fmt"
	"strings"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/chatquickreply"
	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
)

const chatQuickReplyAdvisoryNamespace int64 = 0x5355505000000000

type chatQuickReplyRepository struct {
	client *dbent.Client
}

func NewChatQuickReplyRepository(client *dbent.Client) chat.QuickReplyRepository {
	return &chatQuickReplyRepository{client: client}
}

func (r *chatQuickReplyRepository) ListByAdminID(ctx context.Context, adminID int64) ([]chat.QuickReply, error) {
	return listChatQuickReplies(ctx, clientFromContext(ctx, r.client), adminID)
}

func (r *chatQuickReplyRepository) Create(ctx context.Context, reply *chat.QuickReply) error {
	return r.withAdminLock(ctx, reply.AdminID, func(txCtx context.Context, client *dbent.Client) error {
		count, err := client.ChatQuickReply.Query().Where(chatquickreply.AdminIDEQ(reply.AdminID)).Count(txCtx)
		if err != nil {
			return err
		}
		if count >= chat.MaxQuickReplies {
			return chat.ErrQuickReplyLimitReached
		}
		created, err := client.ChatQuickReply.Create().
			SetAdminID(reply.AdminID).
			SetTitle(reply.Title).
			SetContent(reply.Content).
			SetSortOrder(count).
			Save(txCtx)
		if err != nil {
			return err
		}
		*reply = *chatQuickReplyEntityToDomain(created)
		return nil
	})
}

func (r *chatQuickReplyRepository) Update(ctx context.Context, reply *chat.QuickReply) error {
	updated, err := clientFromContext(ctx, r.client).ChatQuickReply.Update().
		Where(chatquickreply.IDEQ(reply.ID), chatquickreply.AdminIDEQ(reply.AdminID)).
		SetTitle(reply.Title).
		SetContent(reply.Content).
		Save(ctx)
	if err != nil {
		return err
	}
	if updated == 0 {
		return chat.ErrQuickReplyNotFound
	}
	item, err := r.GetByID(ctx, reply.AdminID, reply.ID)
	if err != nil {
		return err
	}
	*reply = *item
	return nil
}

func (r *chatQuickReplyRepository) Delete(ctx context.Context, adminID, id int64) error {
	deleted, err := clientFromContext(ctx, r.client).ChatQuickReply.Delete().
		Where(chatquickreply.IDEQ(id), chatquickreply.AdminIDEQ(adminID)).
		Exec(ctx)
	if err != nil {
		return err
	}
	if deleted == 0 {
		return chat.ErrQuickReplyNotFound
	}
	return nil
}

func (r *chatQuickReplyRepository) GetByID(ctx context.Context, adminID, id int64) (*chat.QuickReply, error) {
	item, err := clientFromContext(ctx, r.client).ChatQuickReply.Query().
		Where(chatquickreply.IDEQ(id), chatquickreply.AdminIDEQ(adminID)).
		Only(ctx)
	if err != nil {
		return nil, translatePersistenceError(err, chat.ErrQuickReplyNotFound, nil)
	}
	return chatQuickReplyEntityToDomain(item), nil
}

func (r *chatQuickReplyRepository) Reorder(ctx context.Context, adminID int64, orderedIDs []int64) error {
	return r.withAdminLock(ctx, adminID, func(txCtx context.Context, client *dbent.Client) error {
		count, err := client.ChatQuickReply.Query().Where(chatquickreply.AdminIDEQ(adminID)).Count(txCtx)
		if err != nil {
			return err
		}
		if count != len(orderedIDs) {
			return chat.ErrQuickReplyNotFound
		}
		if count == 0 {
			return nil
		}

		values := make([]string, 0, len(orderedIDs))
		args := make([]any, 0, 1+len(orderedIDs)*2)
		args = append(args, adminID)
		for i, id := range orderedIDs {
			values = append(values, fmt.Sprintf("($%d::bigint, $%d::int)", len(args)+1, len(args)+2))
			args = append(args, id, i)
		}
		query := `
			WITH requested(id, sort_order) AS (VALUES ` + strings.Join(values, ",") + `),
			updated AS (
				UPDATE chat_quick_replies AS q
				SET sort_order = requested.sort_order, updated_at = NOW()
				FROM requested
				WHERE q.id = requested.id AND q.admin_id = $1
				RETURNING q.id
			)
			SELECT COUNT(*) FROM updated
		`
		var updated int
		rows, err := client.QueryContext(txCtx, query, args...)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		if !rows.Next() {
			if err := rows.Err(); err != nil {
				return err
			}
			return chat.ErrQuickReplyNotFound
		}
		if err := rows.Scan(&updated); err != nil {
			return err
		}
		if updated != count {
			return chat.ErrQuickReplyNotFound
		}
		return nil
	})
}

func (r *chatQuickReplyRepository) Import(ctx context.Context, adminID int64, replies []chat.QuickReply) ([]chat.QuickReply, error) {
	err := r.withAdminLock(ctx, adminID, func(txCtx context.Context, client *dbent.Client) error {
		existing, err := listChatQuickReplies(txCtx, client, adminID)
		if err != nil {
			return err
		}
		seen := make(map[string]struct{}, len(existing)+len(replies))
		for _, reply := range existing {
			seen[quickReplyDedupKey(reply.Title, reply.Content)] = struct{}{}
		}
		builders := make([]*dbent.ChatQuickReplyCreate, 0, len(replies))
		for _, reply := range replies {
			key := quickReplyDedupKey(reply.Title, reply.Content)
			if _, ok := seen[key]; ok {
				continue
			}
			if len(existing)+len(builders) >= chat.MaxQuickReplies {
				return chat.ErrQuickReplyLimitReached
			}
			seen[key] = struct{}{}
			builders = append(builders, client.ChatQuickReply.Create().
				SetAdminID(adminID).
				SetTitle(reply.Title).
				SetContent(reply.Content).
				SetSortOrder(len(existing)+len(builders)))
		}
		if len(builders) > 0 {
			_, err = client.ChatQuickReply.CreateBulk(builders...).Save(txCtx)
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return r.ListByAdminID(ctx, adminID)
}

func (r *chatQuickReplyRepository) withAdminLock(
	ctx context.Context,
	adminID int64,
	fn func(context.Context, *dbent.Client) error,
) error {
	if tx := dbent.TxFromContext(ctx); tx != nil {
		client := tx.Client()
		if _, err := client.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", chatQuickReplyAdvisoryNamespace^adminID); err != nil {
			return err
		}
		return fn(ctx, client)
	}
	tx, err := r.client.Tx(ctx)
	if err != nil {
		return err
	}
	txCtx := dbent.NewTxContext(ctx, tx)
	if _, err := tx.Client().ExecContext(txCtx, "SELECT pg_advisory_xact_lock($1)", chatQuickReplyAdvisoryNamespace^adminID); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := fn(txCtx, tx.Client()); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func listChatQuickReplies(ctx context.Context, client *dbent.Client, adminID int64) ([]chat.QuickReply, error) {
	items, err := client.ChatQuickReply.Query().
		Where(chatquickreply.AdminIDEQ(adminID)).
		Order(dbent.Asc(chatquickreply.FieldSortOrder), dbent.Asc(chatquickreply.FieldID)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]chat.QuickReply, 0, len(items))
	for _, item := range items {
		out = append(out, *chatQuickReplyEntityToDomain(item))
	}
	return out, nil
}

func chatQuickReplyEntityToDomain(item *dbent.ChatQuickReply) *chat.QuickReply {
	return &chat.QuickReply{
		ID: item.ID, AdminID: item.AdminID, Title: item.Title, Content: item.Content,
		SortOrder: item.SortOrder, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func quickReplyDedupKey(title, content string) string {
	return strings.ToLower(strings.TrimSpace(title)) + "\x00" + strings.TrimSpace(content)
}

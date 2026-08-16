package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
	"github.com/stretchr/testify/require"
)

type transferConversationRepo struct {
	conversation *chat.Conversation
}

func (r *transferConversationRepo) GetOrCreateByUserID(context.Context, int64) (*chat.Conversation, error) {
	return r.conversation, nil
}
func (r *transferConversationRepo) GetByID(context.Context, int64) (*chat.Conversation, error) {
	return r.conversation, nil
}
func (r *transferConversationRepo) GetByUserID(context.Context, int64) (*chat.Conversation, error) {
	return r.conversation, nil
}
func (r *transferConversationRepo) List(context.Context, pagination.PaginationParams, chat.ConversationListFilters) ([]chat.Conversation, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *transferConversationRepo) CountUnreadByAdmin(context.Context) (int, error) { return 0, nil }
func (r *transferConversationRepo) GetUnreadByUserID(context.Context, int64) (int, error) {
	return 0, nil
}
func (r *transferConversationRepo) MarkRead(context.Context, int64, chat.SenderType) (time.Time, bool, error) {
	return time.Time{}, false, nil
}
func (r *transferConversationRepo) MarkUnreadByAdmin(context.Context, int64) (bool, error) {
	return false, nil
}

type transferMessageRepo struct {
	created          *chat.Message
	getCalls         int
	hideUntilGetCall int
}

func (r *transferMessageRepo) CreateAndTouch(_ context.Context, message *chat.Message, _ time.Time, _ chat.SenderType) error {
	message.ID = 99
	message.CreatedAt = time.Now()
	r.created = message
	return nil
}
func (r *transferMessageRepo) List(context.Context, int64, pagination.PaginationParams) ([]chat.Message, *pagination.PaginationResult, error) {
	return nil, nil, nil
}
func (r *transferMessageRepo) GetByIdempotencyKey(_ context.Context, sender chat.SenderType, senderID int64, key string) (*chat.Message, error) {
	r.getCalls++
	if r.getCalls <= r.hideUntilGetCall {
		return nil, chat.ErrMessageNotFound
	}
	if r.created != nil && r.created.SenderType == sender && r.created.SenderID == senderID &&
		r.created.IdempotencyKey != nil && *r.created.IdempotencyKey == key {
		return r.created, nil
	}
	return nil, chat.ErrMessageNotFound
}
func (r *transferMessageRepo) RecallByAdmin(context.Context, int64, int64, int64, time.Time) (*chat.Message, bool, error) {
	return nil, false, chat.ErrMessageRecallNotAllowed
}
func (r *transferMessageRepo) DeleteExpiredBefore(context.Context, time.Time, int) (int, error) {
	return 0, nil
}

type transferAdminBalanceStub struct {
	user  *User
	err   error
	inTx  bool
	calls int
}

func (s *transferAdminBalanceStub) UpdateUserBalance(ctx context.Context, _ int64, _ float64, _ string, _ string) (*User, error) {
	s.calls++
	s.inTx = dbent.TxFromContext(ctx) != nil
	return s.user, s.err
}

type transferBroadcaster struct{ calls int }

func (b *transferBroadcaster) BroadcastMessage(int64, int64, *chat.Message, bool)   { b.calls++ }
func (b *transferBroadcaster) BroadcastMessageRecalled(int64, int64, *chat.Message) {}
func (b *transferBroadcaster) BroadcastReadState(int64, int64, chat.SenderType, time.Time, bool) {
}

func transferEntClient(t *testing.T) (*dbent.Client, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	driver := entsql.OpenDB(dialect.Postgres, db)
	client := dbent.NewClient(dbent.Driver(driver))
	t.Cleanup(func() {
		_ = client.Close()
		_ = db.Close()
	})
	return client, mock, db
}

func TestSupportChatTransferCommitsBalanceAndReceiptInOneTransaction(t *testing.T) {
	client, mock, _ := transferEntClient(t)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	conversation := &chat.Conversation{ID: 7, UserID: 42}
	messages := &transferMessageRepo{}
	chatService := chat.NewService(&transferConversationRepo{conversation: conversation}, messages)
	broadcaster := &transferBroadcaster{}
	chatService.SetBroadcaster(broadcaster)
	admin := &transferAdminBalanceStub{user: &User{ID: 42, Balance: 12.5}}
	service := &SupportChatTransferService{adminService: admin, chatService: chatService, entClient: client}

	result, err := service.Transfer(context.Background(), 7, 3, 2.5, "goodwill", "transfer-key-123")
	require.NoError(t, err)
	require.Equal(t, 12.5, result.Balance)
	require.Equal(t, chat.MessageKindBalanceTransfer, result.Message.Kind)
	require.True(t, admin.inTx)
	require.Equal(t, 1, admin.calls)
	require.Equal(t, 1, broadcaster.calls, "broadcast must happen only after commit")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSupportChatTransferRollsBackWithoutReceiptOnBalanceFailure(t *testing.T) {
	client, mock, _ := transferEntClient(t)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	conversation := &chat.Conversation{ID: 7, UserID: 42}
	messages := &transferMessageRepo{}
	chatService := chat.NewService(&transferConversationRepo{conversation: conversation}, messages)
	broadcaster := &transferBroadcaster{}
	chatService.SetBroadcaster(broadcaster)
	admin := &transferAdminBalanceStub{err: errors.New("balance write failed")}
	service := &SupportChatTransferService{adminService: admin, chatService: chatService, entClient: client}

	result, err := service.Transfer(context.Background(), 7, 3, 2.5, "", "transfer-key-123")
	require.ErrorContains(t, err, "balance write failed")
	require.Nil(t, result)
	require.Nil(t, messages.created)
	require.Zero(t, broadcaster.calls)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSupportChatTransferRecoversCommittedResponseAndRejectsPayloadReuse(t *testing.T) {
	client, mock, _ := transferEntClient(t)
	key := "transfer-key-123"
	metadata, err := json.Marshal(chat.BalanceTransferMetadata{Amount: 2.5, BalanceAfter: 12.5, Notes: "goodwill"})
	require.NoError(t, err)
	messages := &transferMessageRepo{created: &chat.Message{
		ID: 99, ConversationID: 7, SenderType: chat.SenderTypeAdmin, SenderID: 3,
		Kind: chat.MessageKindBalanceTransfer, Metadata: metadata, IdempotencyKey: &key,
	}}
	chatService := chat.NewService(
		&transferConversationRepo{conversation: &chat.Conversation{ID: 7, UserID: 42}},
		messages,
	)
	admin := &transferAdminBalanceStub{}
	service := &SupportChatTransferService{adminService: admin, chatService: chatService, entClient: client}

	result, err := service.Transfer(context.Background(), 7, 3, 2.5, "goodwill", key)
	require.NoError(t, err)
	require.True(t, result.Replayed)
	require.Equal(t, 12.5, result.Balance)
	require.Zero(t, admin.calls, "a lost response must recover from the durable receipt without crediting twice")

	result, err = service.Transfer(context.Background(), 7, 3, 3.5, "goodwill", key)
	require.ErrorIs(t, err, chat.ErrMessageIdempotencyConflict)
	require.Nil(t, result)
	require.Zero(t, admin.calls)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestSupportChatTransferRechecksReceiptUnderDatabaseLockBeforeCrediting(t *testing.T) {
	client, mock, _ := transferEntClient(t)
	mock.ExpectBegin()
	mock.ExpectExec("SELECT pg_advisory_xact_lock").WithArgs(sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectRollback()

	key := "transfer-key-123"
	metadata, err := json.Marshal(chat.BalanceTransferMetadata{Amount: 2.5, BalanceAfter: 12.5, Notes: "goodwill"})
	require.NoError(t, err)
	messages := &transferMessageRepo{
		created: &chat.Message{
			ID: 99, ConversationID: 7, SenderType: chat.SenderTypeAdmin, SenderID: 3,
			Kind: chat.MessageKindBalanceTransfer, Metadata: metadata, IdempotencyKey: &key,
		},
		hideUntilGetCall: 1,
	}
	chatService := chat.NewService(
		&transferConversationRepo{conversation: &chat.Conversation{ID: 7, UserID: 42}},
		messages,
	)
	admin := &transferAdminBalanceStub{user: &User{ID: 42, Balance: 15}}
	service := &SupportChatTransferService{adminService: admin, chatService: chatService, entClient: client}

	result, err := service.Transfer(context.Background(), 7, 3, 2.5, "goodwill", key)
	require.NoError(t, err)
	require.True(t, result.Replayed)
	require.Equal(t, 12.5, result.Balance)
	require.Zero(t, admin.calls, "a receipt that appears while waiting for the durable lock must prevent a second credit")
	require.NoError(t, mock.ExpectationsWereMet())
}

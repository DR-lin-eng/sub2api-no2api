package chat

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
	"github.com/stretchr/testify/require"
)

type conversationRepoStub struct {
	conversation     *Conversation
	getByUserErr     error
	getOrCreateCalls int
	markReadChanged  bool
}

func (r *conversationRepoStub) GetOrCreateByUserID(context.Context, int64) (*Conversation, error) {
	r.getOrCreateCalls++
	return r.conversation, nil
}

func (r *conversationRepoStub) GetByID(context.Context, int64) (*Conversation, error) {
	return r.conversation, nil
}

func (r *conversationRepoStub) GetByUserID(context.Context, int64) (*Conversation, error) {
	if r.getByUserErr != nil {
		return nil, r.getByUserErr
	}
	return r.conversation, nil
}

func (r *conversationRepoStub) List(context.Context, pagination.PaginationParams, ConversationListFilters) ([]Conversation, *pagination.PaginationResult, error) {
	return nil, nil, nil
}

func (r *conversationRepoStub) CountUnreadByAdmin(context.Context) (int, error) {
	return 0, nil
}

func (r *conversationRepoStub) GetUnreadByUserID(context.Context, int64) (int, error) {
	return 0, nil
}

func (r *conversationRepoStub) MarkRead(context.Context, int64, SenderType) (time.Time, bool, error) {
	return time.Now(), r.markReadChanged, nil
}

type messageRepoStub struct {
	createAndTouchErr   error
	createAndTouchCalls int
	existing            *Message
	getErr              error
}

func (r *messageRepoStub) CreateAndTouch(_ context.Context, m *Message, _ time.Time, _ SenderType) error {
	r.createAndTouchCalls++
	if r.createAndTouchErr != nil {
		return r.createAndTouchErr
	}
	m.ID = 10
	m.CreatedAt = time.Now()
	return nil
}

func (r *messageRepoStub) List(context.Context, int64, pagination.PaginationParams) ([]Message, *pagination.PaginationResult, error) {
	return nil, nil, errors.New("message list should not be called")
}

func (r *messageRepoStub) GetByIdempotencyKey(context.Context, SenderType, int64, string) (*Message, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	if r.existing != nil {
		return r.existing, nil
	}
	return nil, ErrMessageNotFound
}

type broadcasterStub struct {
	calls     int
	readCalls int
}

func (b *broadcasterStub) BroadcastMessage(int64, int64, *Message, bool) {
	b.calls++
}

func (b *broadcasterStub) BroadcastReadState(int64, int64, SenderType, time.Time, bool) {
	b.readCalls++
}

func TestUserUnreadAndEmptyMessageListDoNotCreateConversation(t *testing.T) {
	conversations := &conversationRepoStub{getByUserErr: ErrConversationNotFound}
	service := NewService(conversations, &messageRepoStub{})

	unread, err := service.GetUnreadCountForUser(context.Background(), 42)
	require.NoError(t, err)
	require.Zero(t, unread)

	messages, page, err := service.ListMessagesForUser(
		context.Background(),
		42,
		pagination.PaginationParams{Page: 1, PageSize: 20},
	)
	require.NoError(t, err)
	require.Empty(t, messages)
	require.Zero(t, page.Total)
	require.Zero(t, conversations.getOrCreateCalls)
}

func TestPostMessageRequiresAtomicWriteAndDoesNotBroadcastOnFailure(t *testing.T) {
	conversations := &conversationRepoStub{conversation: &Conversation{ID: 7, UserID: 42}}
	messages := &messageRepoStub{createAndTouchErr: errors.New("transaction failed")}
	broadcaster := &broadcasterStub{}
	service := NewService(conversations, messages)
	service.SetBroadcaster(broadcaster)

	message, err := service.PostMessageFromUser(context.Background(), 42, "hello")
	require.ErrorContains(t, err, "create chat message and update conversation")
	require.Nil(t, message)
	require.Equal(t, 1, messages.createAndTouchCalls)
	require.Zero(t, broadcaster.calls)
}

func TestPostMessageLengthLimitCountsUnicodeCharacters(t *testing.T) {
	conversations := &conversationRepoStub{conversation: &Conversation{ID: 7, UserID: 42}}
	messages := &messageRepoStub{}
	service := NewService(conversations, messages)

	message, err := service.PostMessageFromUser(
		context.Background(),
		42,
		strings.Repeat("你", MaxMessageContentLen),
	)
	require.NoError(t, err)
	require.NotNil(t, message)

	message, err = service.PostMessageFromUser(
		context.Background(),
		42,
		strings.Repeat("你", MaxMessageContentLen+1),
	)
	require.ErrorIs(t, err, ErrMessageContentTooLong)
	require.Nil(t, message)
}

func TestPostMessageIdempotencyRejectsPayloadReuse(t *testing.T) {
	key := "same-key-123"
	conversations := &conversationRepoStub{conversation: &Conversation{ID: 7, UserID: 42}}
	messages := &messageRepoStub{existing: &Message{
		ID: 10, ConversationID: 7, SenderType: SenderTypeUser, SenderID: 42,
		Content: "original", Kind: MessageKindText, Metadata: []byte(`{}`), IdempotencyKey: &key,
	}}
	service := NewService(conversations, messages)

	message, err := service.PostRichMessageFromUser(context.Background(), 42, SendMessageInput{
		Content: "different", Kind: MessageKindText, IdempotencyKey: key,
	})
	require.ErrorIs(t, err, ErrMessageIdempotencyConflict)
	require.Nil(t, message)
	require.Zero(t, messages.createAndTouchCalls)
}

func TestPostMessageIdempotencyReplaysIdenticalPayloadWithoutBroadcast(t *testing.T) {
	key := "same-key-123"
	existing := &Message{
		ID: 10, ConversationID: 7, SenderType: SenderTypeUser, SenderID: 42,
		Content: "same", Kind: MessageKindText, Metadata: []byte(`{}`), IdempotencyKey: &key,
	}
	conversations := &conversationRepoStub{conversation: &Conversation{ID: 7, UserID: 42}}
	messages := &messageRepoStub{existing: existing}
	broadcaster := &broadcasterStub{}
	service := NewService(conversations, messages)
	service.SetBroadcaster(broadcaster)

	message, err := service.PostRichMessageFromUser(context.Background(), 42, SendMessageInput{
		Content: "same", Kind: MessageKindText, IdempotencyKey: key,
	})
	require.NoError(t, err)
	require.Same(t, existing, message)
	require.Zero(t, messages.createAndTouchCalls)
	require.Zero(t, broadcaster.calls)
}

func TestMessageJSONDoesNotExposeIdempotencyKey(t *testing.T) {
	key := "private-idempotency-key"
	encoded, err := json.Marshal(&Message{ID: 10, Content: "hello", IdempotencyKey: &key})
	require.NoError(t, err)
	require.NotContains(t, string(encoded), key)
	require.NotContains(t, string(encoded), "IdempotencyKey")
}

func TestRichMessageValidationRejectsInvalidAssetShapes(t *testing.T) {
	service := NewService(
		&conversationRepoStub{conversation: &Conversation{ID: 7, UserID: 42}},
		&messageRepoStub{},
	)

	for _, input := range []SendMessageInput{
		{Content: "text", Kind: MessageKindText, AssetIDs: []int64{1}},
		{Content: "image", Kind: MessageKindImage},
		{Content: "image", Kind: MessageKindImage, AssetIDs: []int64{1, 1}},
		{Content: "sticker", Kind: MessageKindSticker, AssetIDs: []int64{1, 2}},
		{Content: "transfer", Kind: MessageKindBalanceTransfer},
	} {
		message, err := service.PostRichMessageFromUser(context.Background(), 42, input)
		require.Error(t, err)
		require.Nil(t, message)
	}
}

func TestMarkReadBroadcastsOnlyWhenPersistenceChanges(t *testing.T) {
	conversations := &conversationRepoStub{conversation: &Conversation{ID: 7, UserID: 42}}
	broadcaster := &broadcasterStub{}
	service := NewService(conversations, &messageRepoStub{})
	service.SetBroadcaster(broadcaster)

	require.NoError(t, service.MarkReadByUser(context.Background(), 42))
	require.Zero(t, broadcaster.readCalls)

	conversations.markReadChanged = true
	require.NoError(t, service.MarkReadByUser(context.Background(), 42))
	require.Equal(t, 1, broadcaster.readCalls)
}

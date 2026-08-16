package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
)

// Broadcaster pushes a newly created message to connected websocket clients.
// Implemented by the Hub (ws.go); kept as an interface so Service has no
// direct dependency on the transport layer.
type Broadcaster interface {
	BroadcastMessage(conversationID, recipientUserID int64, msg *Message, toAdmins bool)
	BroadcastMessageRecalled(conversationID, recipientUserID int64, msg *Message)
	BroadcastReadState(conversationID, recipientUserID int64, reader SenderType, readAt time.Time, toAdmins bool)
}

// noopBroadcaster is used until SetBroadcaster is called, so tests and
// early wiring don't need a real Hub.
type noopBroadcaster struct{}

func (noopBroadcaster) BroadcastMessage(int64, int64, *Message, bool)                {}
func (noopBroadcaster) BroadcastMessageRecalled(int64, int64, *Message)              {}
func (noopBroadcaster) BroadcastReadState(int64, int64, SenderType, time.Time, bool) {}

type Service struct {
	conversationRepo ConversationRepository
	messageRepo      MessageRepository
	quickReplyRepo   QuickReplyRepository
	assetRepo        AssetRepository
	broadcaster      Broadcaster
}

func (s *Service) SetQuickReplyRepository(repo QuickReplyRepository) {
	s.quickReplyRepo = repo
}

func (s *Service) SetAssetRepository(repo AssetRepository) {
	s.assetRepo = repo
}

func NewService(conversationRepo ConversationRepository, messageRepo MessageRepository) *Service {
	return &Service{
		conversationRepo: conversationRepo,
		messageRepo:      messageRepo,
		broadcaster:      noopBroadcaster{},
	}
}

// SetBroadcaster wires the websocket hub after construction, mirroring the
// SetXxx injection pattern used elsewhere in this codebase (e.g. OpsService).
func (s *Service) SetBroadcaster(b Broadcaster) {
	if b != nil {
		s.broadcaster = b
	}
}

// GetOrCreateConversationForUser returns the calling user's own conversation.
func (s *Service) GetOrCreateConversationForUser(ctx context.Context, userID int64) (*Conversation, error) {
	return s.conversationRepo.GetOrCreateByUserID(ctx, userID)
}

// GetConversationForAdmin loads any conversation by ID for the admin inbox.
func (s *Service) GetConversationForAdmin(ctx context.Context, conversationID int64) (*Conversation, error) {
	return s.conversationRepo.GetByID(ctx, conversationID)
}

// ListConversations powers the admin inbox list.
func (s *Service) ListConversations(
	ctx context.Context,
	params pagination.PaginationParams,
	filters ConversationListFilters,
) ([]Conversation, *pagination.PaginationResult, error) {
	return s.conversationRepo.List(ctx, params, filters)
}

// CountUnreadConversationsForAdmin returns the number of conversations that
// have user messages waiting for an administrator.
func (s *Service) CountUnreadConversationsForAdmin(ctx context.Context) (int, error) {
	return s.conversationRepo.CountUnreadByAdmin(ctx)
}

// GetUnreadCountForUser reads the user's unread counter without creating an
// empty conversation as a side effect of a sidebar poll.
func (s *Service) GetUnreadCountForUser(ctx context.Context, userID int64) (int, error) {
	return s.conversationRepo.GetUnreadByUserID(ctx, userID)
}

// ListMessagesForUser returns messages in the caller's own conversation.
func (s *Service) ListMessagesForUser(
	ctx context.Context,
	userID int64,
	params pagination.PaginationParams,
) ([]Message, *pagination.PaginationResult, error) {
	conv, err := s.conversationRepo.GetByUserID(ctx, userID)
	if errors.Is(err, ErrConversationNotFound) {
		return []Message{}, emptyPaginationResult(params), nil
	}
	if err != nil {
		return nil, nil, err
	}
	return s.messageRepo.List(ctx, conv.ID, params)
}

func emptyPaginationResult(params pagination.PaginationParams) *pagination.PaginationResult {
	page := params.Page
	if page < 1 {
		page = 1
	}
	return &pagination.PaginationResult{
		Total:    0,
		Page:     page,
		PageSize: params.Limit(),
		Pages:    0,
	}
}

// ListMessagesForAdmin returns messages in any conversation by ID.
func (s *Service) ListMessagesForAdmin(
	ctx context.Context,
	conversationID int64,
	params pagination.PaginationParams,
) ([]Message, *pagination.PaginationResult, error) {
	if _, err := s.conversationRepo.GetByID(ctx, conversationID); err != nil {
		return nil, nil, err
	}
	return s.messageRepo.List(ctx, conversationID, params)
}

// PostMessageFromUser appends a message sent by the conversation's own user.
func (s *Service) PostMessageFromUser(ctx context.Context, userID int64, content string) (*Message, error) {
	return s.PostRichMessageFromUser(ctx, userID, SendMessageInput{Content: content, Kind: MessageKindText})
}

func (s *Service) PostRichMessageFromUser(ctx context.Context, userID int64, input SendMessageInput) (*Message, error) {
	conv, err := s.conversationRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.postMessage(ctx, conv, SenderTypeUser, userID, input, false, true)
}

// PostMessageFromAdmin appends a message sent by an admin acting as support
// into the given user's conversation. adminID is recorded as the sender.
func (s *Service) PostMessageFromAdmin(ctx context.Context, conversationID, adminID int64, content string) (*Message, error) {
	return s.PostRichMessageFromAdmin(ctx, conversationID, adminID, SendMessageInput{Content: content, Kind: MessageKindText})
}

func (s *Service) PostRichMessageFromAdmin(ctx context.Context, conversationID, adminID int64, input SendMessageInput) (*Message, error) {
	conv, err := s.conversationRepo.GetByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	return s.postMessage(ctx, conv, SenderTypeAdmin, adminID, input, false, true)
}

// RecallMessageByAdmin hides an administrator-authored message from every
// delivery path while retaining the original row for server-side audit. A
// balance-transfer receipt cannot be recalled because the financial action is
// still effective and must remain visible to the user.
func (s *Service) RecallMessageByAdmin(
	ctx context.Context,
	conversationID, messageID, adminID int64,
) (*Message, error) {
	conv, err := s.conversationRepo.GetByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	message, changed, err := s.messageRepo.RecallByAdmin(
		ctx,
		conversationID,
		messageID,
		adminID,
		time.Now().UTC(),
	)
	if err != nil {
		return nil, err
	}
	message = messageForDelivery(message)
	if changed {
		s.broadcaster.BroadcastMessageRecalled(conversationID, conv.UserID, message)
	}
	return message, nil
}

var idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{8,128}$`)

func (s *Service) PostBalanceTransferFromAdmin(
	ctx context.Context,
	conversationID, adminID int64,
	metadata BalanceTransferMetadata,
	idempotencyKey string,
	broadcast bool,
) (*Message, error) {
	conv, err := s.conversationRepo.GetByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return nil, fmt.Errorf("encode balance transfer metadata: %w", err)
	}
	input := SendMessageInput{
		Content:        fmt.Sprintf("Balance credited: %.8g", metadata.Amount),
		Kind:           MessageKindBalanceTransfer,
		IdempotencyKey: idempotencyKey,
	}
	return s.postMessageWithMetadata(ctx, conv, SenderTypeAdmin, adminID, input, encoded, true, broadcast)
}

func (s *Service) GetMessageByIdempotencyKey(
	ctx context.Context,
	sender SenderType,
	senderID int64,
	key string,
) (*Message, error) {
	key = strings.TrimSpace(key)
	if !idempotencyKeyPattern.MatchString(key) {
		return nil, ErrMessageKindInvalid
	}
	return s.messageRepo.GetByIdempotencyKey(ctx, sender, senderID, key)
}

func (s *Service) BroadcastPersistedMessage(recipientUserID int64, msg *Message) {
	if msg == nil {
		return
	}
	s.broadcaster.BroadcastMessage(msg.ConversationID, recipientUserID, msg, msg.SenderType == SenderTypeUser)
}

func (s *Service) postMessage(
	ctx context.Context,
	conv *Conversation,
	sender SenderType,
	senderID int64,
	input SendMessageInput,
	allowBalanceTransfer bool,
	broadcast bool,
) (*Message, error) {
	return s.postMessageWithMetadata(ctx, conv, sender, senderID, input, nil, allowBalanceTransfer, broadcast)
}

func (s *Service) postMessageWithMetadata(
	ctx context.Context,
	conv *Conversation,
	sender SenderType,
	senderID int64,
	input SendMessageInput,
	metadata json.RawMessage,
	allowBalanceTransfer bool,
	broadcast bool,
) (*Message, error) {
	content, kind, assetIDs, encodedMetadata, key, err := normalizeMessageInput(input, metadata, allowBalanceTransfer)
	if err != nil {
		return nil, err
	}
	if key != nil {
		existing, findErr := s.messageRepo.GetByIdempotencyKey(ctx, sender, senderID, *key)
		if findErr == nil {
			if !messageMatchesNormalizedInput(existing, conv.ID, sender, senderID, content, kind, input.ReplyToID, assetIDs, encodedMetadata) {
				return nil, ErrMessageIdempotencyConflict
			}
			return messageForDelivery(existing), nil
		}
		if !errors.Is(findErr, ErrMessageNotFound) {
			return nil, findErr
		}
	}

	msg := &Message{
		ConversationID: conv.ID,
		SenderType:     sender,
		SenderID:       senderID,
		Content:        content,
		Kind:           kind,
		ReplyToID:      input.ReplyToID,
		Metadata:       encodedMetadata,
		IdempotencyKey: key,
		AssetIDs:       assetIDs,
	}
	now := time.Now()
	if err := s.messageRepo.CreateAndTouch(ctx, msg, now, sender); err != nil {
		if key != nil {
			existing, findErr := s.messageRepo.GetByIdempotencyKey(ctx, sender, senderID, *key)
			if findErr == nil {
				if !messageMatchesNormalizedInput(existing, conv.ID, sender, senderID, content, kind, input.ReplyToID, assetIDs, encodedMetadata) {
					return nil, ErrMessageIdempotencyConflict
				}
				return messageForDelivery(existing), nil
			}
		}
		return nil, fmt.Errorf("create chat message and update conversation: %w", err)
	}

	// A user message needs pushing to admins; an admin message needs pushing
	// to the specific user. toAdmins tells the hub which pool to fan out to.
	if broadcast {
		s.BroadcastPersistedMessage(conv.UserID, msg)
	}

	return msg, nil
}

func messageForDelivery(message *Message) *Message {
	if message == nil || message.RecalledAt == nil {
		return message
	}
	redacted := *message
	redacted.Content = ""
	redacted.Metadata = json.RawMessage(`{}`)
	redacted.Assets = nil
	redacted.AssetIDs = nil
	return &redacted
}

func messageMatchesNormalizedInput(
	existing *Message,
	conversationID int64,
	sender SenderType,
	senderID int64,
	content string,
	kind MessageKind,
	replyToID *int64,
	assetIDs []int64,
	metadata json.RawMessage,
) bool {
	if existing == nil || existing.ConversationID != conversationID || existing.SenderType != sender ||
		existing.SenderID != senderID || existing.Content != content || existing.Kind != kind ||
		!sameOptionalInt64(existing.ReplyToID, replyToID) || !sameJSON(existing.Metadata, metadata) ||
		len(existing.Assets) != len(assetIDs) {
		return false
	}
	for i := range assetIDs {
		if existing.Assets[i].ID != assetIDs[i] {
			return false
		}
	}
	return true
}

func sameOptionalInt64(left, right *int64) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func sameJSON(left, right json.RawMessage) bool {
	var leftCompact, rightCompact bytes.Buffer
	if json.Compact(&leftCompact, left) != nil || json.Compact(&rightCompact, right) != nil {
		return bytes.Equal(left, right)
	}
	return bytes.Equal(leftCompact.Bytes(), rightCompact.Bytes())
}

func normalizeMessageInput(
	input SendMessageInput,
	metadata json.RawMessage,
	allowBalanceTransfer bool,
) (string, MessageKind, []int64, json.RawMessage, *string, error) {
	kind := input.Kind
	if kind == "" {
		kind = MessageKindText
	}
	content := strings.TrimSpace(input.Content)
	assetIDs := uniquePositiveIDs(input.AssetIDs)
	if len(assetIDs) != len(input.AssetIDs) || len(assetIDs) > MaxMessageAssets {
		return "", "", nil, nil, nil, ErrMessageAssetsInvalid
	}
	if input.ReplyToID != nil && *input.ReplyToID <= 0 {
		return "", "", nil, nil, nil, ErrMessageReplyInvalid
	}

	switch kind {
	case MessageKindText:
		if len(assetIDs) != 0 || input.Sticker != nil || content == "" {
			return "", "", nil, nil, nil, ErrMessageContentEmpty
		}
	case MessageKindImage:
		if len(assetIDs) == 0 || input.Sticker != nil {
			return "", "", nil, nil, nil, ErrMessageAssetsInvalid
		}
		if content == "" {
			content = "[image]"
		}
	case MessageKindSticker:
		if len(assetIDs) > 1 {
			return "", "", nil, nil, nil, ErrMessageAssetsInvalid
		}
		if len(assetIDs) == 0 {
			if input.Sticker == nil || strings.TrimSpace(input.Sticker.Emoji) == "" {
				return "", "", nil, nil, nil, ErrMessageContentEmpty
			}
			sticker := StickerMetadata{
				Name:  trimRunes(input.Sticker.Name, 80),
				Emoji: trimRunes(input.Sticker.Emoji, 16),
			}
			var err error
			metadata, err = json.Marshal(sticker)
			if err != nil {
				return "", "", nil, nil, nil, err
			}
			content = sticker.Emoji
		} else if content == "" {
			content = "[sticker]"
		}
	case MessageKindBalanceTransfer:
		if !allowBalanceTransfer || len(assetIDs) != 0 || len(metadata) == 0 {
			return "", "", nil, nil, nil, ErrMessageKindInvalid
		}
	default:
		return "", "", nil, nil, nil, ErrMessageKindInvalid
	}

	if utf8.RuneCountInString(content) > MaxMessageContentLen {
		return "", "", nil, nil, nil, ErrMessageContentTooLong
	}
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}

	var key *string
	if value := strings.TrimSpace(input.IdempotencyKey); value != "" {
		if !idempotencyKeyPattern.MatchString(value) {
			return "", "", nil, nil, nil, ErrMessageKindInvalid
		}
		key = &value
	}
	return content, kind, assetIDs, metadata, key, nil
}

func uniquePositiveIDs(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func trimRunes(value string, limit int) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > limit {
		runes = runes[:limit]
	}
	return string(runes)
}

// MarkReadByUser clears the user-side unread counter for their own conversation.
func (s *Service) MarkReadByUser(ctx context.Context, userID int64) error {
	conv, err := s.conversationRepo.GetByUserID(ctx, userID)
	if errors.Is(err, ErrConversationNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	readAt, changed, err := s.conversationRepo.MarkRead(ctx, conv.ID, SenderTypeUser)
	if err != nil {
		return err
	}
	if changed {
		s.broadcaster.BroadcastReadState(conv.ID, userID, SenderTypeUser, readAt, true)
	}
	return nil
}

// MarkReadByAdmin clears the admin-side unread counter for the given conversation.
func (s *Service) MarkReadByAdmin(ctx context.Context, conversationID int64) error {
	conv, err := s.conversationRepo.GetByID(ctx, conversationID)
	if err != nil {
		return err
	}
	readAt, changed, err := s.conversationRepo.MarkRead(ctx, conversationID, SenderTypeAdmin)
	if err != nil {
		return err
	}
	if changed {
		s.broadcaster.BroadcastReadState(conversationID, conv.UserID, SenderTypeAdmin, readAt, false)
	}
	return nil
}

// MarkUnreadByAdmin adds a private inbox reminder without undoing the read
// receipt already sent to the user or fabricating an unread message count.
func (s *Service) MarkUnreadByAdmin(ctx context.Context, conversationID int64) error {
	if _, err := s.conversationRepo.GetByID(ctx, conversationID); err != nil {
		return err
	}
	_, err := s.conversationRepo.MarkUnreadByAdmin(ctx, conversationID)
	return err
}

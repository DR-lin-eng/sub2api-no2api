package chat

import (
	"context"
	"errors"
	"fmt"
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
	BroadcastReadState(conversationID, recipientUserID int64, reader SenderType, toAdmins bool)
}

// noopBroadcaster is used until SetBroadcaster is called, so tests and
// early wiring don't need a real Hub.
type noopBroadcaster struct{}

func (noopBroadcaster) BroadcastMessage(int64, int64, *Message, bool)     {}
func (noopBroadcaster) BroadcastReadState(int64, int64, SenderType, bool) {}

type Service struct {
	conversationRepo ConversationRepository
	messageRepo      MessageRepository
	broadcaster      Broadcaster
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
	conv, err := s.conversationRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.postMessage(ctx, conv, SenderTypeUser, userID, content)
}

// PostMessageFromAdmin appends a message sent by an admin acting as support
// into the given user's conversation. adminID is recorded as the sender.
func (s *Service) PostMessageFromAdmin(ctx context.Context, conversationID, adminID int64, content string) (*Message, error) {
	conv, err := s.conversationRepo.GetByID(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	return s.postMessage(ctx, conv, SenderTypeAdmin, adminID, content)
}

func (s *Service) postMessage(ctx context.Context, conv *Conversation, sender SenderType, senderID int64, content string) (*Message, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, ErrMessageContentEmpty
	}
	if utf8.RuneCountInString(content) > MaxMessageContentLen {
		return nil, ErrMessageContentTooLong
	}

	msg := &Message{
		ConversationID: conv.ID,
		SenderType:     sender,
		SenderID:       senderID,
		Content:        content,
	}
	now := time.Now()
	if err := s.messageRepo.CreateAndTouch(ctx, msg, now, sender); err != nil {
		return nil, fmt.Errorf("create chat message and update conversation: %w", err)
	}

	// A user message needs pushing to admins; an admin message needs pushing
	// to the specific user. toAdmins tells the hub which pool to fan out to.
	toAdmins := sender == SenderTypeUser
	s.broadcaster.BroadcastMessage(conv.ID, conv.UserID, msg, toAdmins)

	return msg, nil
}

// MarkReadByUser clears the user-side unread counter for their own conversation.
func (s *Service) MarkReadByUser(ctx context.Context, userID int64) error {
	conv, err := s.conversationRepo.GetOrCreateByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.conversationRepo.MarkRead(ctx, conv.ID, SenderTypeUser); err != nil {
		return err
	}
	// Broadcast to admins that user has read their messages
	s.broadcaster.BroadcastReadState(conv.ID, userID, SenderTypeUser, true)
	return nil
}

// MarkReadByAdmin clears the admin-side unread counter for the given conversation.
func (s *Service) MarkReadByAdmin(ctx context.Context, conversationID int64) error {
	conv, err := s.conversationRepo.GetByID(ctx, conversationID)
	if err != nil {
		return err
	}
	if err := s.conversationRepo.MarkRead(ctx, conversationID, SenderTypeAdmin); err != nil {
		return err
	}
	// Broadcast to the specific user that admin has read their messages
	s.broadcaster.BroadcastReadState(conversationID, conv.UserID, SenderTypeAdmin, false)
	return nil
}

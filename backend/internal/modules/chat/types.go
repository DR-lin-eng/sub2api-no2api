// Package chat implements the real-time customer-support chat feature.
//
// Every admin acts as a support agent (no separate agent role). Each user
// has exactly one long-lived conversation with "support" — there is no
// per-ticket/session splitting. v1 is text-only: no attachments or images.
package chat

import (
	"context"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
)

// SenderType identifies who sent a chat message.
type SenderType string

const (
	SenderTypeUser  SenderType = "user"
	SenderTypeAdmin SenderType = "admin"
)

// Conversation is the single long-lived thread between a user and support.
type Conversation struct {
	ID            int64
	UserID        int64
	LastMessageAt *time.Time
	UnreadByUser  int
	UnreadByAdmin int
	CreatedAt     time.Time
	UpdatedAt     time.Time

	// UserEmail/UserUsername are only populated by List (the admin inbox),
	// which eager-loads the owning user so admins can identify conversations
	// without a second round-trip per row.
	UserEmail    string
	UserUsername string
}

// Message is a single text message within a conversation.
type Message struct {
	ID             int64
	ConversationID int64
	SenderType     SenderType
	SenderID       int64
	Content        string
	CreatedAt      time.Time
}

// ConversationListFilters narrows the admin conversation list.
type ConversationListFilters struct {
	// UnreadOnly restricts results to conversations with unread admin messages.
	UnreadOnly bool
	// Search matches against the user's email/username.
	Search string
}

var (
	ErrConversationNotFound  = infraerrors.NotFound("CHAT_CONVERSATION_NOT_FOUND", "chat conversation not found")
	ErrMessageContentEmpty   = infraerrors.BadRequest("CHAT_MESSAGE_CONTENT_REQUIRED", "message content is required")
	ErrMessageContentTooLong = infraerrors.BadRequest(
		"CHAT_MESSAGE_CONTENT_TOO_LONG",
		"message content exceeds the maximum length",
	)
)

// MaxMessageContentLen bounds a single message's length (matches the ent schema's MaxLen).
const MaxMessageContentLen = 10000

// ConversationRepository persists chat conversations.
type ConversationRepository interface {
	// GetOrCreateByUserID returns the user's conversation, creating it on first contact.
	GetOrCreateByUserID(ctx context.Context, userID int64) (*Conversation, error)
	GetByID(ctx context.Context, id int64) (*Conversation, error)
	GetByUserID(ctx context.Context, userID int64) (*Conversation, error)

	// List returns conversations for the admin inbox, sorted by most recent activity.
	List(ctx context.Context, params pagination.PaginationParams, filters ConversationListFilters) ([]Conversation, *pagination.PaginationResult, error)
	// CountUnreadByAdmin returns the number of conversations with messages waiting for support.
	CountUnreadByAdmin(ctx context.Context) (int, error)
	// GetUnreadByUserID returns the user's unread count without creating a conversation.
	GetUnreadByUserID(ctx context.Context, userID int64) (int, error)

	// MarkRead zeroes the unread counter for the given side.
	MarkRead(ctx context.Context, conversationID int64, sender SenderType) error
}

// MessageRepository persists chat messages.
type MessageRepository interface {
	// CreateAndTouch atomically persists the message and updates the
	// conversation activity/unread state. Implementations must not degrade to
	// separate writes because callers broadcast only after this method succeeds.
	CreateAndTouch(ctx context.Context, m *Message, at time.Time, sender SenderType) error
	List(ctx context.Context, conversationID int64, params pagination.PaginationParams) ([]Message, *pagination.PaginationResult, error)
}

// QuickReply is an admin's saved quick reply template.
type QuickReply struct {
	ID        int64
	AdminID   int64
	Title     string
	Content   string
	SortOrder int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// QuickReplyRepository persists quick reply templates.
type QuickReplyRepository interface {
	ListByAdminID(ctx context.Context, adminID int64) ([]QuickReply, error)
	Create(ctx context.Context, qr *QuickReply) error
	Update(ctx context.Context, qr *QuickReply) error
	Delete(ctx context.Context, adminID, id int64) error
	GetByID(ctx context.Context, adminID, id int64) (*QuickReply, error)
	UpdateSortOrders(ctx context.Context, adminID int64, idOrderMap map[int64]int) error
}

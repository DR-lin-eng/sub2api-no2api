// Package chat implements the real-time customer-support chat feature.
//
// Every admin acts as a support agent (no separate agent role). Each user
// has exactly one long-lived conversation with "support" — there is no
// per-ticket/session splitting.
package chat

import (
	"context"
	"encoding/json"
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
	ID                    int64
	UserID                int64
	LastMessageAt         *time.Time
	UnreadByUser          int
	UnreadByAdmin         int
	ManuallyUnreadByAdmin bool
	LastReadByUserAt      *time.Time
	LastReadByAdminAt     *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time

	// UserEmail/UserUsername are only populated by List (the admin inbox),
	// which eager-loads the owning user so admins can identify conversations
	// without a second round-trip per row.
	UserEmail    string
	UserUsername string
}

// MessageKind identifies the server-validated presentation contract.
type MessageKind string

const (
	MessageKindText            MessageKind = "text"
	MessageKindImage           MessageKind = "image"
	MessageKindSticker         MessageKind = "sticker"
	MessageKindBalanceTransfer MessageKind = "balance_transfer"
)

// Message is one persisted message within a conversation.
type Message struct {
	ID             int64
	ConversationID int64
	SenderType     SenderType
	SenderID       int64
	Content        string
	Kind           MessageKind
	ReplyToID      *int64
	Metadata       json.RawMessage
	IdempotencyKey *string `json:"-"`
	Assets         []Asset
	AssetIDs       []int64 `json:"-"`
	RecalledAt     *time.Time
	RecalledBy     *int64 `json:"-"`
	CreatedAt      time.Time
}

type StickerMetadata struct {
	Name  string `json:"name"`
	Emoji string `json:"emoji,omitempty"`
}

type BalanceTransferMetadata struct {
	Amount       float64 `json:"amount"`
	BalanceAfter float64 `json:"balance_after"`
	Notes        string  `json:"notes,omitempty"`
}

type SendMessageInput struct {
	Content        string
	Kind           MessageKind
	ReplyToID      *int64
	AssetIDs       []int64
	Sticker        *StickerMetadata
	IdempotencyKey string
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
	ErrMessageKindInvalid         = infraerrors.BadRequest("CHAT_MESSAGE_KIND_INVALID", "message kind is invalid")
	ErrMessageReplyInvalid        = infraerrors.BadRequest("CHAT_MESSAGE_REPLY_INVALID", "reply target is invalid")
	ErrMessageAssetsInvalid       = infraerrors.BadRequest("CHAT_MESSAGE_ASSETS_INVALID", "message assets are invalid")
	ErrMessageNotFound            = infraerrors.NotFound("CHAT_MESSAGE_NOT_FOUND", "chat message not found")
	ErrMessageIdempotencyConflict = infraerrors.Conflict(
		"CHAT_MESSAGE_IDEMPOTENCY_CONFLICT",
		"idempotency key was already used for a different chat message",
	)
	ErrMessageRecallNotAllowed = infraerrors.Conflict(
		"CHAT_MESSAGE_RECALL_NOT_ALLOWED",
		"message cannot be recalled",
	)
	ErrAssetNotFound          = infraerrors.NotFound("CHAT_ASSET_NOT_FOUND", "chat asset not found")
	ErrQuickReplyNotFound     = infraerrors.NotFound("CHAT_QUICK_REPLY_NOT_FOUND", "quick reply not found")
	ErrQuickReplyLimitReached = infraerrors.BadRequest("CHAT_QUICK_REPLY_LIMIT_REACHED", "quick reply limit reached")
)

// MaxMessageContentLen bounds a single message's length (matches the ent schema's MaxLen).
const (
	MaxMessageContentLen = 10000
	MaxMessageAssets     = 4
	MaxQuickReplies      = 50
)

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

	// MarkRead zeroes the unread counter and returns the persisted read time.
	// changed is false when the same side has already read every message.
	MarkRead(ctx context.Context, conversationID int64, sender SenderType) (readAt time.Time, changed bool, err error)
	// MarkUnreadByAdmin persists a private admin reminder without changing the
	// count of real unread user messages.
	MarkUnreadByAdmin(ctx context.Context, conversationID int64) (changed bool, err error)
}

// MessageRepository persists chat messages.
type MessageRepository interface {
	// CreateAndTouch atomically persists the message and updates the
	// conversation activity/unread state. Implementations must not degrade to
	// separate writes because callers broadcast only after this method succeeds.
	CreateAndTouch(ctx context.Context, m *Message, at time.Time, sender SenderType) error
	List(ctx context.Context, conversationID int64, params pagination.PaginationParams) ([]Message, *pagination.PaginationResult, error)
	GetByIdempotencyKey(ctx context.Context, sender SenderType, senderID int64, key string) (*Message, error)
	// RecallByAdmin atomically marks an administrator message as recalled and
	// removes it from the recipient's unread count when it was still unread.
	// changed is false for an idempotent replay of an earlier recall.
	RecallByAdmin(ctx context.Context, conversationID, messageID, recalledByID int64, at time.Time) (message *Message, changed bool, err error)
	// DeleteExpiredBefore removes ordinary support messages in bounded batches
	// and reconciles the affected conversation summaries. Balance-transfer
	// receipts are deliberately retained as durable financial evidence.
	DeleteExpiredBefore(ctx context.Context, before time.Time, limit int) (int, error)
}

// RetentionCleanupResult reports one bounded retention-cleanup batch.
type RetentionCleanupResult struct {
	MessagesDeleted int
	AssetsDeleted   int
}

type AssetScope string

const (
	AssetScopeMessage AssetScope = "message"
	AssetScopeLibrary AssetScope = "library"
	AssetScopeSticker AssetScope = "sticker"
)

// Asset contains image metadata; Data is populated only by authorized reads.
type Asset struct {
	ID             int64      `json:"id"`
	Scope          AssetScope `json:"scope"`
	ConversationID *int64     `json:"-"`
	UploadedBy     *int64     `json:"-"`
	Name           string     `json:"name"`
	MIMEType       string     `json:"mime_type"`
	Size           int        `json:"size"`
	Data           []byte     `json:"-"`
	Collection     string     `json:"collection,omitempty"`
	CatalogVisible bool       `json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
}

type AssetRepository interface {
	Create(ctx context.Context, asset *Asset) error
	ListCatalog(ctx context.Context, scope AssetScope, limit int) ([]Asset, error)
	HideCatalog(ctx context.Context, scope AssetScope, id int64) error
	GetForUser(ctx context.Context, id, userID, conversationID int64) (*Asset, error)
	GetForAdmin(ctx context.Context, id, adminID int64) (*Asset, error)
	DeleteUnattachedBefore(ctx context.Context, before time.Time, limit int) (int, error)
}

// QuickReply is an administrator-owned reusable message template.
type QuickReply struct {
	ID        int64
	AdminID   int64
	Title     string
	Content   string
	SortOrder int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type QuickReplyRepository interface {
	ListByAdminID(ctx context.Context, adminID int64) ([]QuickReply, error)
	Create(ctx context.Context, reply *QuickReply) error
	Update(ctx context.Context, reply *QuickReply) error
	Delete(ctx context.Context, adminID, id int64) error
	GetByID(ctx context.Context, adminID, id int64) (*QuickReply, error)
	Reorder(ctx context.Context, adminID int64, orderedIDs []int64) error
	Import(ctx context.Context, adminID int64, replies []QuickReply) ([]QuickReply, error)
}

package admin

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/Wei-Shaw/sub2api/internal/shared/wsutil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ChatHandler handles the admin side of the support chat: the conversation
// inbox, replying to any conversation, marking read, and the realtime
// WebSocket push shared by every connected admin (all admins act as support
// agents; there is no separate agent identity).
type ChatHandler struct {
	chatService     *chat.Service
	transferService *service.SupportChatTransferService
	hub             *chat.Hub
	upgrader        websocket.Upgrader
	limiter         *wsutil.ConnLimiter
}

func (h *ChatHandler) SetTransferService(transferService *service.SupportChatTransferService) {
	h.transferService = transferService
}

// NewChatHandler creates the admin-facing chat handler.
func NewChatHandler(chatService *chat.Service, hub *chat.Hub) *ChatHandler {
	return &ChatHandler{
		chatService: chatService,
		hub:         hub,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return wsutil.IsAllowedOrigin(r, true, wsutil.DefaultTrustedProxies(), wsutil.OriginPolicyPermissive)
			},
			Subprotocols: []string{"sub2api-admin-chat"},
		},
		limiter: wsutil.NewConnLimiter(chatAdminWSMaxConnsTotal, chatAdminWSMaxConnsPerIP),
	}
}

type adminSendMessageRequest struct {
	Content        string                `json:"content"`
	Kind           chat.MessageKind      `json:"kind"`
	ReplyToID      *int64                `json:"reply_to_id"`
	AssetIDs       []int64               `json:"asset_ids"`
	Sticker        *chat.StickerMetadata `json:"sticker"`
	IdempotencyKey string                `json:"idempotency_key"`
}

const (
	maxChatRequestBodyBytes = 64 << 10
	maxChatSearchRunes      = 200
)

func limitChatSearch(search string) string {
	search = strings.TrimSpace(search)
	if utf8.RuneCountInString(search) <= maxChatSearchRunes {
		return search
	}
	return string([]rune(search)[:maxChatSearchRunes])
}

// ListConversations returns the admin inbox, paginated and optionally
// filtered to unread-by-admin conversations or a user/message search.
// GET /api/v1/admin/chat/conversations
func (h *ChatHandler) ListConversations(c *gin.Context) {
	page, pageSize := response.ParsePaginationWithMax(c, 100)
	unreadOnly := parseBoolQueryWithDefault(c.Query("unread_only"), false)
	search := limitChatSearch(c.Query("search"))

	items, paginationResult, err := h.chatService.ListConversations(
		c.Request.Context(),
		pagination.PaginationParams{Page: page, PageSize: pageSize},
		chat.ConversationListFilters{UnreadOnly: unreadOnly, Search: search},
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, paginationResult.Total, page, pageSize)
}

// GetUnreadCount returns the number of conversations waiting for support.
// GET /api/v1/admin/chat/unread-count
func (h *ChatHandler) GetUnreadCount(c *gin.Context) {
	count, err := h.chatService.CountUnreadConversationsForAdmin(c.Request.Context())
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"unread_count": count})
}

// ListMessages returns a conversation's message history, paginated.
// GET /api/v1/admin/chat/conversations/:id/messages
func (h *ChatHandler) ListMessages(c *gin.Context) {
	conversationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || conversationID <= 0 {
		response.BadRequest(c, "Invalid conversation ID")
		return
	}

	page, pageSize := response.ParsePaginationWithMax(c, 100)
	items, paginationResult, err := h.chatService.ListMessagesForAdmin(c.Request.Context(), conversationID, pagination.PaginationParams{
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, paginationResult.Total, page, pageSize)
}

// SendMessage appends an admin reply to the given conversation.
// POST /api/v1/admin/chat/conversations/:id/messages
func (h *ChatHandler) SendMessage(c *gin.Context) {
	conversationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || conversationID <= 0 {
		response.BadRequest(c, "Invalid conversation ID")
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxChatRequestBodyBytes)
	var req adminSendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			response.RequestEntityTooLarge(c, "Request body too large")
			return
		}
		response.BadRequest(c, "Invalid request")
		return
	}

	if req.IdempotencyKey == "" {
		req.IdempotencyKey = c.GetHeader("Idempotency-Key")
	}
	msg, err := h.chatService.PostRichMessageFromAdmin(c.Request.Context(), conversationID, subject.UserID, chat.SendMessageInput{
		Content:        req.Content,
		Kind:           req.Kind,
		ReplyToID:      req.ReplyToID,
		AssetIDs:       req.AssetIDs,
		Sticker:        req.Sticker,
		IdempotencyKey: req.IdempotencyKey,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, msg)
}

// RecallMessage hides an administrator-authored message from both delivery
// APIs and realtime clients while retaining its server-side audit row.
// POST /api/v1/admin/chat/conversations/:id/messages/:message_id/recall
func (h *ChatHandler) RecallMessage(c *gin.Context) {
	conversationID, ok := parsePositiveChatID(c, "id", "Invalid conversation ID")
	if !ok {
		return
	}
	messageID, ok := parsePositiveChatID(c, "message_id", "Invalid message ID")
	if !ok {
		return
	}
	adminID, ok := chatAdminID(c)
	if !ok {
		return
	}

	message, err := h.chatService.RecallMessageByAdmin(
		c.Request.Context(),
		conversationID,
		messageID,
		adminID,
	)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, message)
}

type balanceTransferRequest struct {
	Amount float64 `json:"amount" binding:"required,gt=0,lte=1000000000"`
	Notes  string  `json:"notes"`
}

// TransferBalance atomically credits the conversation owner and persists a
// structured chat receipt. Idempotency-Key is required by the shared admin
// idempotency coordinator and is also enforced durably by chat_messages.
func (h *ChatHandler) TransferBalance(c *gin.Context) {
	conversationID, ok := parsePositiveChatID(c, "id", "Invalid conversation ID")
	if !ok {
		return
	}
	adminID, ok := chatAdminID(c)
	if !ok {
		return
	}
	if h.transferService == nil {
		response.InternalError(c, "Balance transfer service is unavailable")
		return
	}
	var req balanceTransferRequest
	if !bindChatJSON(c, &req) {
		return
	}
	payload := struct {
		ConversationID int64                  `json:"conversation_id"`
		Body           balanceTransferRequest `json:"body"`
	}{ConversationID: conversationID, Body: req}
	executeAdminIdempotentJSON(
		c,
		"admin.chat.balance.transfer",
		payload,
		service.DefaultWriteIdempotencyTTL(),
		func(ctx context.Context) (any, error) {
			return h.transferService.Transfer(
				ctx,
				conversationID,
				adminID,
				req.Amount,
				req.Notes,
				c.GetHeader("Idempotency-Key"),
			)
		},
	)
}

// MarkRead clears the admin-side unread counter for the given conversation.
// POST /api/v1/admin/chat/conversations/:id/read
func (h *ChatHandler) MarkRead(c *gin.Context) {
	conversationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || conversationID <= 0 {
		response.BadRequest(c, "Invalid conversation ID")
		return
	}

	if err := h.chatService.MarkReadByAdmin(c.Request.Context(), conversationID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

// MarkUnread persists a private reminder in the shared admin inbox without
// changing the real unread-message count or undoing the user's read receipt.
// POST /api/v1/admin/chat/conversations/:id/unread
func (h *ChatHandler) MarkUnread(c *gin.Context) {
	conversationID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || conversationID <= 0 {
		response.BadRequest(c, "Invalid conversation ID")
		return
	}

	if err := h.chatService.MarkUnreadByAdmin(c.Request.Context(), conversationID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

const (
	chatAdminWSMaxConnsTotal  = 200
	chatAdminWSMaxConnsPerIP  = 20
	chatAdminWSWriteTimeout   = 10 * time.Second
	chatAdminWSPongWait       = 60 * time.Second
	chatAdminWSPingInterval   = 30 * time.Second
	chatAdminWSMaxReadBytes   = 1024
	chatAdminWSSendBufferSize = 32
	chatAdminWSMaxAuthAge     = 0
)

// WS handles the realtime push connection shared by every connected admin:
// any admin socket receives every user-sent message across all conversations.
// GET /api/v1/admin/chat/ws
func (h *ChatHandler) WS(c *gin.Context) {
	clientIP := c.ClientIP()
	release, acquired := h.limiter.TryAcquire(clientIP)
	if !acquired {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "too many connections"})
		return
	}
	defer release()

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.LegacyPrintf("handler.admin.chat_ws", "[AdminChatWS] upgrade failed: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()

	send := make(chan []byte, chatAdminWSSendBufferSize)
	handle := h.hub.RegisterAdmin(send)
	defer h.hub.UnregisterAdmin(handle)
	wsutil.PumpWebSocket(conn, send, wsutil.PumpConfig{
		WriteTimeout: chatAdminWSWriteTimeout,
		PongWait:     chatAdminWSPongWait,
		PingInterval: chatAdminWSPingInterval,
		MaxReadBytes: chatAdminWSMaxReadBytes,
		MaxAuthAge:   chatAdminWSMaxAuthAge,
	})
}

package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/Wei-Shaw/sub2api/internal/shared/wsutil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ChatHandler handles the authenticated user's side of the support chat: their
// own conversation, sending/listing messages, marking read, and the realtime
// WebSocket push.
type ChatHandler struct {
	chatService *chat.Service
	hub         *chat.Hub
	upgrader    websocket.Upgrader
	limiter     *wsutil.ConnLimiter
}

// NewChatHandler creates the user-facing chat handler.
func NewChatHandler(chatService *chat.Service, hub *chat.Hub) *ChatHandler {
	return &ChatHandler{
		chatService: chatService,
		hub:         hub,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return wsutil.IsAllowedOrigin(r, true, wsutil.DefaultTrustedProxies(), wsutil.OriginPolicyPermissive)
			},
			Subprotocols: []string{"sub2api-chat"},
		},
		limiter: wsutil.NewConnLimiter(chatWSMaxConnsTotal, chatWSMaxConnsPerIP),
	}
}

type sendMessageRequest struct {
	Content        string                `json:"content"`
	Kind           chat.MessageKind      `json:"kind"`
	ReplyToID      *int64                `json:"reply_to_id"`
	AssetIDs       []int64               `json:"asset_ids"`
	Sticker        *chat.StickerMetadata `json:"sticker"`
	IdempotencyKey string                `json:"idempotency_key"`
}

const maxChatRequestBodyBytes = 64 << 10

// GetConversation returns (creating if necessary) the caller's own conversation.
// GET /api/v1/chat/conversation
func (h *ChatHandler) GetConversation(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	conv, err := h.chatService.GetOrCreateConversationForUser(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, conv)
}

// ListMessages returns the caller's own message history, paginated.
// GET /api/v1/chat/messages
func (h *ChatHandler) ListMessages(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	page, pageSize := response.ParsePaginationWithMax(c, 100)
	items, paginationResult, err := h.chatService.ListMessagesForUser(c.Request.Context(), subject.UserID, pagination.PaginationParams{
		Page:     page,
		PageSize: pageSize,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Paginated(c, items, paginationResult.Total, page, pageSize)
}

// GetUnreadCount returns the caller's unread count without creating a
// conversation. GET /api/v1/chat/unread-count
func (h *ChatHandler) GetUnreadCount(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	count, err := h.chatService.GetUnreadCountForUser(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"unread_count": count})
}

// SendMessage appends a message to the caller's own conversation.
// POST /api/v1/chat/messages
func (h *ChatHandler) SendMessage(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxChatRequestBodyBytes)
	var req sendMessageRequest
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
	msg, err := h.chatService.PostRichMessageFromUser(c.Request.Context(), subject.UserID, chat.SendMessageInput{
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

// MarkRead clears the caller's own unread counter.
// POST /api/v1/chat/read
func (h *ChatHandler) MarkRead(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	if err := h.chatService.MarkReadByUser(c.Request.Context(), subject.UserID); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

const (
	chatWSMaxConnsTotal  = 2000
	chatWSMaxConnsPerIP  = 20
	chatWSWriteTimeout   = 10 * time.Second
	chatWSPongWait       = 60 * time.Second
	chatWSPingInterval   = 30 * time.Second
	chatWSMaxReadBytes   = 1024
	chatWSSendBufferSize = 16
	chatWSMaxAuthAge     = 5 * time.Minute
)

// WS handles the realtime push connection for the caller's own conversation.
// GET /api/v1/chat/ws
func (h *ChatHandler) WS(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	clientIP := c.ClientIP()
	release, acquired := h.limiter.TryAcquire(clientIP)
	if !acquired {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "too many connections"})
		return
	}
	defer release()

	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		logger.LegacyPrintf("handler.chat_ws", "[ChatWS] upgrade failed: %v", err)
		return
	}
	defer func() { _ = conn.Close() }()

	send := make(chan []byte, chatWSSendBufferSize)
	handle := h.hub.RegisterUser(subject.UserID, send)
	defer h.hub.UnregisterUser(subject.UserID, handle)
	authExpiresAt, _ := middleware2.GetJWTExpiresAtFromContext(c)

	wsutil.PumpWebSocket(conn, send, wsutil.PumpConfig{
		WriteTimeout:  chatWSWriteTimeout,
		PongWait:      chatWSPongWait,
		PingInterval:  chatWSPingInterval,
		MaxReadBytes:  chatWSMaxReadBytes,
		AuthExpiresAt: authExpiresAt,
		MaxAuthAge:    chatWSMaxAuthAge,
	})
}

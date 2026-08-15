package admin

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/modules/chat"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/Wei-Shaw/sub2api/internal/shared/wsutil"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler/supportchatasset"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// ChatHandler handles the admin side of the support chat: the conversation
// inbox, replying to any conversation, marking read, and the realtime
// WebSocket push shared by every connected admin (all admins act as support
// agents; there is no separate agent identity).
type ChatHandler struct {
	chatService *chat.Service
	hub         *chat.Hub
	upgrader    websocket.Upgrader
	limiter     *wsutil.ConnLimiter
}

// NewChatHandler creates the admin-facing chat handler.
func NewChatHandler(chatService *chat.Service, hub *chat.Hub) *ChatHandler {
	return &ChatHandler{
		chatService: chatService,
		hub:         hub,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// In development with Vite proxy (changeOrigin: false), the Host header
				// is preserved as the frontend's host (e.g., localhost:3000).
				// Accept requests where Origin matches Host for local development.
				origin := strings.TrimSpace(r.Header.Get("Origin"))
				if origin == "" {
					return true
				}

				originURL, err := url.Parse(origin)
				if err != nil {
					return false
				}

				reqHost := strings.ToLower(strings.TrimSpace(r.Host))
				originHost := strings.ToLower(originURL.Host)

				return originHost == reqHost
			},
			Subprotocols: []string{"sub2api-admin-chat"},
		},
		limiter: wsutil.NewConnLimiter(chatAdminWSMaxConnsTotal, chatAdminWSMaxConnsPerIP),
	}
}

type adminSendMessageRequest struct {
	Content string `json:"content" binding:"required"`
}

const (
	maxChatRequestBodyBytes = 32 << 10
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
// filtered to unread-by-admin conversations or a user email/username search.
// GET /api/v1/admin/chat/conversations
func (h *ChatHandler) ListConversations(c *gin.Context) {
	page, pageSize := response.ParsePagination(c)
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

	page, pageSize := response.ParsePagination(c)
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

	msg, err := h.chatService.PostMessageFromAdmin(c.Request.Context(), conversationID, subject.UserID, req.Content)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, msg)
}

// UploadAsset stores an image that can be embedded in a support-chat message.
// POST /api/v1/admin/chat/assets
func (h *ChatHandler) UploadAsset(c *gin.Context) {
	supportchatasset.Upload(c)
}

// ListLibrary returns shared support-chat image library items.
// GET /api/v1/admin/chat/image-library
func (h *ChatHandler) ListLibrary(c *gin.Context) {
	supportchatasset.ListLibrary(c)
}

// CreateLibraryItem uploads and saves a shared support-chat image library item.
// POST /api/v1/admin/chat/image-library
func (h *ChatHandler) CreateLibraryItem(c *gin.Context) {
	supportchatasset.CreateLibraryItem(c)
}

// DeleteLibraryItem removes a shared support-chat image library item.
// DELETE /api/v1/admin/chat/image-library/:id
func (h *ChatHandler) DeleteLibraryItem(c *gin.Context) {
	supportchatasset.DeleteLibraryItem(c)
}

// ListStickers returns shared support-chat stickers.
// GET /api/v1/admin/chat/stickers
func (h *ChatHandler) ListStickers(c *gin.Context) {
	supportchatasset.ListStickers(c)
}

// CreateSticker uploads and saves a shared support-chat sticker.
// POST /api/v1/admin/chat/stickers
func (h *ChatHandler) CreateSticker(c *gin.Context) {
	supportchatasset.CreateSticker(c)
}

// DeleteSticker removes a shared support-chat sticker.
// DELETE /api/v1/admin/chat/stickers/:id
func (h *ChatHandler) DeleteSticker(c *gin.Context) {
	supportchatasset.DeleteSticker(c)
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

// ListQuickReplies returns the logged-in admin's quick replies.
// GET /api/v1/admin/chat/quick-replies
func (h *ChatHandler) ListQuickReplies(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	items, err := h.chatService.ListQuickReplies(c.Request.Context(), subject.UserID)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, items)
}

type createQuickReplyRequest struct {
	Title   string `json:"title" binding:"required,max=100"`
	Content string `json:"content" binding:"required"`
}

// CreateQuickReply adds a new quick reply for the logged-in admin.
// POST /api/v1/admin/chat/quick-replies
func (h *ChatHandler) CreateQuickReply(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxChatRequestBodyBytes)
	var req createQuickReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			response.RequestEntityTooLarge(c, "Request body too large")
			return
		}
		response.BadRequest(c, "Invalid request")
		return
	}

	item, err := h.chatService.CreateQuickReply(c.Request.Context(), subject.UserID, req.Title, req.Content)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

type updateQuickReplyRequest struct {
	Title   string `json:"title" binding:"required,max=100"`
	Content string `json:"content" binding:"required"`
}

// UpdateQuickReply updates an existing quick reply.
// PUT /api/v1/admin/chat/quick-replies/:id
func (h *ChatHandler) UpdateQuickReply(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid quick reply ID")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxChatRequestBodyBytes)
	var req updateQuickReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			response.RequestEntityTooLarge(c, "Request body too large")
			return
		}
		response.BadRequest(c, "Invalid request")
		return
	}

	item, err := h.chatService.UpdateQuickReply(c.Request.Context(), subject.UserID, id, req.Title, req.Content)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, item)
}

// DeleteQuickReply removes a quick reply.
// DELETE /api/v1/admin/chat/quick-replies/:id
func (h *ChatHandler) DeleteQuickReply(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.BadRequest(c, "Invalid quick reply ID")
		return
	}

	if err := h.chatService.DeleteQuickReply(c.Request.Context(), subject.UserID, id); err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, gin.H{"message": "ok"})
}

type reorderQuickRepliesRequest struct {
	IDs []int64 `json:"ids" binding:"required"`
}

// ReorderQuickReplies updates the sort order of quick replies.
// POST /api/v1/admin/chat/quick-replies/reorder
func (h *ChatHandler) ReorderQuickReplies(c *gin.Context) {
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not found in context")
		return
	}

	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxChatRequestBodyBytes)
	var req reorderQuickRepliesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			response.RequestEntityTooLarge(c, "Request body too large")
			return
		}
		response.BadRequest(c, "Invalid request")
		return
	}

	if err := h.chatService.ReorderQuickReplies(c.Request.Context(), subject.UserID, req.IDs); err != nil {
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

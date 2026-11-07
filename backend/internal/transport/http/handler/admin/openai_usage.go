package admin

import (
	"context"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/response"
	"github.com/Wei-Shaw/sub2api/internal/shared/wsutil"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type openAIUsageDetailsQuerier interface {
	QueryUsageDetails(context.Context, int64, service.OpenAIUsageDetailsRequest) (*service.OpenAIUsageDetails, error)
}

type openAIUsageSubscriber interface {
	SubscribeUsage(context.Context, int64) (<-chan service.OpenAIUsagePushEvent, func(), error)
}

var openAIUsageSocketLimiter = wsutil.NewConnLimiter(512, 256)

// QueryUsageDetails only accepts named usage resources, never arbitrary paths.
// POST /api/v1/admin/openai/accounts/:id/usage/query
func (h *OpenAIOAuthHandler) QueryUsageDetails(c *gin.Context) {
	id, ok := openAIQuotaAccountID(c)
	if !ok || id <= 0 {
		if ok {
			response.BadRequest(c, "Invalid account ID")
		}
		return
	}
	querier, ok := h.quotaService.(openAIUsageDetailsQuerier)
	if !ok {
		response.Error(c, http.StatusServiceUnavailable, "usage details unavailable")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
	var query service.OpenAIUsageDetailsRequest
	if err := c.ShouldBindJSON(&query); err != nil {
		response.BadRequest(c, "Invalid usage query")
		return
	}
	result, err := querier.QueryUsageDetails(c.Request.Context(), id, query)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, result)
}

// StreamUsage is protected by the existing administrator authentication and
// account-management permission middleware, including jwt.* WS subprotocols.
// GET /api/v1/admin/openai/accounts/:id/usage/ws
func (h *OpenAIOAuthHandler) StreamUsage(c *gin.Context) {
	id, ok := openAIQuotaAccountID(c)
	if !ok || id <= 0 {
		if ok {
			response.BadRequest(c, "Invalid account ID")
		}
		return
	}
	subscriber, ok := h.quotaService.(openAIUsageSubscriber)
	if !ok {
		response.Error(c, http.StatusServiceUnavailable, "usage push unavailable")
		return
	}
	if !wsutil.IsAllowedOrigin(c.Request, true, wsutil.DefaultTrustedProxies(), wsutil.OriginPolicyPermissive) {
		response.Error(c, http.StatusForbidden, "Invalid WebSocket origin")
		return
	}
	releaseSlot, ok := openAIUsageSocketLimiter.TryAcquire(c.ClientIP())
	if !ok {
		response.Error(c, http.StatusTooManyRequests, "usage connection limit reached")
		return
	}
	defer releaseSlot()
	// Reauthenticate periodically, and bound a hijacked connection's lifetime.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Minute)
	defer cancel()
	events, release, err := subscriber.SubscribeUsage(ctx, id)
	if err != nil {
		response.Error(c, http.StatusServiceUnavailable, "usage push unavailable; use the existing quota refresh")
		return
	}
	defer release()
	upgrader := websocket.Upgrader{
		Subprotocols: []string{"sub2api-admin"},
		CheckOrigin: func(r *http.Request) bool {
			return wsutil.IsAllowedOrigin(r, true, wsutil.DefaultTrustedProxies(), wsutil.OriginPolicyPermissive)
		},
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(1024)
	_ = conn.SetReadDeadline(time.Now().Add(time.Minute))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(time.Minute)) })
	go func() {
		defer cancel()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				return
			}
		case event := <-events:
			_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
			if err := conn.WriteJSON(event); err != nil || event.Type == "fallback" {
				return
			}
		}
	}
}

package chat

import (
	"encoding/json"
	"sync"
	"time"
)

// conn is the minimal surface the hub needs from a websocket connection.
// The real implementation (gorilla/websocket) lives in the transport-layer
// handler; the hub only needs to push outbound frames onto a buffered
// channel so a slow/misbehaving reader can't block message delivery to
// everyone else.
type conn struct {
	send chan []byte
}

// outboundEvent is the JSON payload pushed to connected websocket clients.
type outboundEvent struct {
	Type      string            `json:"type"`
	Message   *Message          `json:"message,omitempty"`
	ReadState *readStatePayload `json:"read_state,omitempty"`
}

type readStatePayload struct {
	ConversationID int64      `json:"conversation_id"`
	Reader         SenderType `json:"reader"`
	ReadAt         time.Time  `json:"read_at"`
}

// Hub is a package-level singleton connection registry (constructed once
// via wire and injected into both the user-facing and admin-facing WS
// handlers, plus the Service via SetBroadcaster). There is no existing
// pub/sub registry in this codebase to reuse — ops_ws_handler.go only
// broadcasts one shared payload to anonymous connections, and the OpenAI
// gateway WS pools are outbound-only — so this is new.
type Hub struct {
	mu     sync.Mutex
	users  map[int64][]*conn  // userID -> that user's open connections (multiple tabs/devices)
	admins map[*conn]struct{} // every connected admin socket, unkeyed (any admin sees any conversation)
}

// NewHub creates an empty connection registry.
func NewHub() *Hub {
	return &Hub{
		users:  make(map[int64][]*conn),
		admins: make(map[*conn]struct{}),
	}
}

// RegisterUser adds a connection to the given user's pool and returns a
// handle to unregister it later. send is owned by the caller (the transport-
// layer handler), which is responsible for pumping it out to the real socket.
func (h *Hub) RegisterUser(userID int64, send chan []byte) *conn {
	c := &conn{send: send}
	h.mu.Lock()
	h.users[userID] = append(h.users[userID], c)
	h.mu.Unlock()
	return c
}

// UnregisterUser removes a previously registered user connection.
func (h *Hub) UnregisterUser(userID int64, c *conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	conns := h.users[userID]
	for i, existing := range conns {
		if existing == c {
			h.users[userID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}
	if len(h.users[userID]) == 0 {
		delete(h.users, userID)
	}
}

// RegisterAdmin adds a connection to the admin pool.
func (h *Hub) RegisterAdmin(send chan []byte) *conn {
	c := &conn{send: send}
	h.mu.Lock()
	h.admins[c] = struct{}{}
	h.mu.Unlock()
	return c
}

// UnregisterAdmin removes a previously registered admin connection.
func (h *Hub) UnregisterAdmin(c *conn) {
	h.mu.Lock()
	delete(h.admins, c)
	h.mu.Unlock()
}

// BroadcastMessage implements Broadcaster. When toAdmins is true (a user
// just sent a message) it fans out to every connected admin socket;
// otherwise (an admin replied) it fans out to the recipient user's own
// connections only.
func (h *Hub) BroadcastMessage(_ int64, recipientUserID int64, msg *Message, toAdmins bool) {
	payload, err := json.Marshal(outboundEvent{Type: "message", Message: msg})
	if err != nil {
		return
	}
	h.broadcast(payload, recipientUserID, toAdmins)
}

// BroadcastMessageRecalled updates both sides of the shared conversation. The
// service passes a delivery-redacted message so the original payload never
// enters a WebSocket frame after recall.
func (h *Hub) BroadcastMessageRecalled(_ int64, recipientUserID int64, msg *Message) {
	payload, err := json.Marshal(outboundEvent{Type: "message_recalled", Message: msg})
	if err != nil {
		return
	}
	h.broadcast(payload, recipientUserID, true)
	h.broadcast(payload, recipientUserID, false)
}

func (h *Hub) BroadcastReadState(
	conversationID, recipientUserID int64,
	reader SenderType,
	readAt time.Time,
	toAdmins bool,
) {
	payload, err := json.Marshal(outboundEvent{
		Type: "read_state",
		ReadState: &readStatePayload{
			ConversationID: conversationID,
			Reader:         reader,
			ReadAt:         readAt,
		},
	})
	if err != nil {
		return
	}
	h.broadcast(payload, recipientUserID, toAdmins)
}

func (h *Hub) broadcast(payload []byte, recipientUserID int64, toAdmins bool) {
	h.mu.Lock()
	var targets []*conn
	if toAdmins {
		targets = make([]*conn, 0, len(h.admins))
		for c := range h.admins {
			targets = append(targets, c)
		}
	} else {
		targets = append(targets, h.users[recipientUserID]...)
	}
	h.mu.Unlock()

	for _, c := range targets {
		select {
		case c.send <- payload:
		default:
			// Slow consumer: drop rather than block the broadcaster.
		}
	}
}

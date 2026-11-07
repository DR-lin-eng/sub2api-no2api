package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/chatgptimages"
	"github.com/tidwall/gjson"
)

func urlPathEscapeChatGPTImage(value string) string { return url.PathEscape(value) }

func (s *chatGPTImageSession) consumeTopic(ctx context.Context, d *chatgptimages.Decoder) error {
	topic := d.Handoff.Topic
	if len(topic) > 512 || !strings.HasPrefix(topic, "conversation-") && !strings.HasPrefix(topic, "conv-turn-low-ttl-") {
		return fmt.Errorf("invalid ChatGPT image topic")
	}
	discovery, err := s.json(ctx, http.MethodGet, "/celsius/ws/user", nil)
	if err != nil {
		return err
	}
	address := gjson.GetBytes(discovery, "websocket_url").String()
	if !validUsageWebSocketURL(address) {
		return fmt.Errorf("invalid ChatGPT image websocket endpoint")
	}
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	dialer := s.gateway.getOpenAIWSPassthroughDialer()
	proxy := ""
	if s.account.ProxyID != nil && s.account.Proxy != nil {
		proxy = s.account.Proxy.URL()
	}
	conn, _, _, err := dialOpenAIWSRouteWithProfile(dialer, dialCtx, address, http.Header{}, accountEgressRoute(s.account, proxy), s.gateway.resolveTLSProfile(s.account))
	cancel()
	if err != nil {
		return fmt.Errorf("ChatGPT image websocket unavailable")
	}
	defer func() { _ = conn.Close() }()
	for i, command := range []any{map[string]any{"type": "connect", "presence": map[string]any{"type": "presence", "state": "foreground"}}, map[string]any{"type": "subscribe", "topic_id": topic, "offset": "0"}} {
		writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = conn.WriteJSON(writeCtx, []any{map[string]any{"id": i + 1, "command": command}})
		cancel()
		if err != nil {
			return err
		}
	}
	seen := make(map[string]bool)
	deadline := time.Now().Add(5 * time.Second)
	for frameCount := 0; frameCount < 65536; frameCount++ {
		readCtx, cancel := context.WithDeadline(ctx, deadline)
		raw, err := conn.ReadMessage(readCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("ChatGPT image topic interrupted")
		}
		if len(raw) > chatgptimages.MaxPayloadBytes {
			return fmt.Errorf("ChatGPT image topic exceeds limit")
		}
		var messages []json.RawMessage
		if json.Unmarshal(raw, &messages) != nil {
			messages = []json.RawMessage{raw}
		}
		for index := 0; index < len(messages); index++ {
			raw := messages[index]
			m := gjson.ParseBytes(raw)
			if reply := m.Get("reply"); reply.Exists() {
				switch m.Get("id").Int() {
				case 1:
					if reply.Get("type").String() != "connect" {
						return fmt.Errorf("ChatGPT image topic connect rejected")
					}
				case 2:
					if reply.Get("type").String() != "subscribe" || reply.Get("topic_id").String() != topic || !reply.Get("recovered").Bool() {
						return fmt.Errorf("ChatGPT image topic history unavailable")
					}
					for _, item := range reply.Get("catchups").Array() {
						if len(messages) >= 65536 {
							return fmt.Errorf("ChatGPT image topic catchup exceeds limit")
						}
						messages = append(messages, json.RawMessage(item.Raw))
					}
				}
				continue
			}
			if actual := m.Get("topic_id").String(); actual != "" && actual != topic {
				continue
			}
			payload := m.Get("payload")
			if payload.Get("type").String() != "conversation-turn-stream" {
				continue
			}
			deadline = time.Now().Add(30 * time.Second)
			if actual := payload.Get("conversation_id").String(); actual != "" && d.ConversationID != "" && actual != d.ConversationID {
				return fmt.Errorf("ChatGPT image topic conversation changed")
			}
			body := payload.Get("payload")
			if !body.Exists() {
				body = payload.Get("data")
			}
			if !body.Exists() {
				body = payload
			}
			if actual := body.Get("conversation_id").String(); actual != "" {
				if d.ConversationID != "" && actual != d.ConversationID {
					return fmt.Errorf("ChatGPT image topic conversation changed")
				}
				d.ConversationID = actual
			}
			switch body.Get("type").String() {
			case "heartbeat":
				continue
			case "done":
				d.Done = true
				return nil
			case "stream-item":
				id := body.Get("stream_item_id").String()
				parent := body.Get("parent_stream_item_id").String()
				if id != "" && seen[id] {
					continue
				}
				if parent != "" && !seen[parent] {
					return fmt.Errorf("ChatGPT image topic item gap")
				}
				if len(seen) >= 65536 {
					return fmt.Errorf("ChatGPT image topic history exceeds limit")
				}
				item := body.Get("encoded_item")
				data := []byte(item.Raw)
				if item.Type == gjson.String {
					data = []byte(item.String())
				}
				encoded := strings.TrimSpace(string(data))
				if strings.HasPrefix(encoded, "data:") || strings.HasPrefix(encoded, "event:") {
					var decodeErr error
					forEachOpenAISSEDataPayload(string(data), func(raw []byte) {
						if decodeErr == nil {
							decodeErr = d.Feed(raw)
						}
					})
					if decodeErr != nil {
						return decodeErr
					}
				} else if err := d.Feed(data); err != nil {
					return err
				}
				if id != "" {
					seen[id] = true
				}
			}
		}
	}
	return fmt.Errorf("ChatGPT image topic event limit exceeded")
}

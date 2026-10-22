package service

import (
	"strings"
	"time"
	"unsafe"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// Strict Responses clients require response.created_at on every lifecycle
// object. Native compatibility conversion already supplies the field, but
// raw OpenAI/Grok passthrough must also repair vendor responses that omit it.
// The helper only touches a missing or null field and preserves all other
// upstream bytes.
type responsesCreatedAtStamper struct {
	createdAt int64
}

func (s *responsesCreatedAtStamper) learn(value gjson.Result) {
	if s.createdAt == 0 && value.Type == gjson.Number && value.Int() > 0 {
		s.createdAt = value.Int()
	}
}

func (s *responsesCreatedAtStamper) Value() int64 {
	if s.createdAt <= 0 {
		s.createdAt = time.Now().Unix()
	}
	return s.createdAt
}

// Observe records an upstream timestamp without changing the payload.
func (s *responsesCreatedAtStamper) Observe(payload []byte, eventType string) {
	if !strings.HasPrefix(strings.TrimSpace(eventType), "response.") {
		return
	}
	s.learn(gjson.GetBytes(payload, "response.created_at"))
}

// Stamp adds response.created_at to a Responses lifecycle event that lacks it.
// Existing values, including zero, are retained for byte compatibility.
func (s *responsesCreatedAtStamper) Stamp(payload []byte, eventType string) ([]byte, bool) {
	if !strings.HasPrefix(strings.TrimSpace(eventType), "response.") {
		return payload, false
	}
	// The view is read-only and local to this call. Avoid copying a potentially
	// large terminal output merely to check its timestamp.
	response := gjson.Get(unsafe.String(unsafe.SliceData(payload), len(payload)), "response")
	if !response.IsObject() {
		return payload, false
	}
	createdAt := response.Get("created_at")
	if createdAt.Exists() && createdAt.Type != gjson.Null {
		s.learn(createdAt)
		return payload, false
	}
	updated, err := sjson.SetBytes(payload, "response.created_at", s.Value())
	if err != nil {
		return payload, false
	}
	return updated, true
}

// ensureOpenAIResponseObjectCreatedAt repairs a buffered non-streaming
// response object. Error envelopes and unrelated JSON remain unchanged.
func ensureOpenAIResponseObjectCreatedAt(body []byte) []byte {
	if !gjson.ValidBytes(body) || strings.TrimSpace(gjson.GetBytes(body, "object").String()) != "response" {
		return body
	}
	createdAt := gjson.GetBytes(body, "created_at")
	if createdAt.Exists() && createdAt.Type != gjson.Null {
		return body
	}
	updated, err := sjson.SetBytes(body, "created_at", time.Now().Unix())
	if err != nil {
		return body
	}
	return updated
}

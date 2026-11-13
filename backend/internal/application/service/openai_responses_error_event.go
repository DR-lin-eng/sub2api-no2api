package service

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

// Responses clients need response.failed, including when a relay sends a
// generic error without a JSON type or uses the native flat type:error shape.
// Keep unrelated event families and specific provider/client errors intact.
func normalizeOpenAIResponsesErrorEvent(payload []byte) ([]byte, bool) {
	if normalized, changed := normalizeOpenAIPolicyErrorEvent(payload); changed {
		return normalized, true
	}
	if !gjson.ValidBytes(payload) || !IsOpenAIGenericUpstreamFailureBody(payload) {
		return payload, false
	}
	eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	if eventType != "" && eventType != "error" {
		return payload, false
	}
	errorValue := gjson.GetBytes(payload, "error")
	var errorBody json.RawMessage
	if errorValue.IsObject() {
		errorBody = json.RawMessage(errorValue.Raw)
	} else {
		code := strings.TrimSpace(gjson.GetBytes(payload, "code").String())
		if code == "" {
			code = "upstream_error"
		}
		errorBody, _ = json.Marshal(map[string]string{
			"type": "upstream_error", "code": code,
			"message": gjson.GetBytes(payload, "message").String(),
		})
	}
	response := map[string]any{
		"id":     "resp_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		"object": "response", "created_at": time.Now().Unix(),
		"status": "failed", "output": []any{}, "error": errorBody,
	}
	if usage := gjson.GetBytes(payload, "usage"); usage.IsObject() {
		response["usage"] = json.RawMessage(usage.Raw)
	}
	normalized, err := json.Marshal(map[string]any{
		"type": "response.failed", "sequence_number": gjson.GetBytes(payload, "sequence_number").Int(),
		"response": response,
	})
	if err != nil {
		return payload, false
	}
	return normalized, true
}

// After semantic output a network failure cannot be replayed. End Responses
// with a classified failure instead of leaving EOF to the SDK or emitting a
// generic type:error frame that the SDK treats as an unfinished stream.
func writeOpenAIResponsesStreamReadFailure(c *gin.Context, cause error) {
	if c == nil || c.Writer == nil || c.Request == nil || c.Request.Context().Err() != nil || IsResponseCommitted(c) {
		return
	}
	code, message := classifyOpenAIUpstreamStreamReadError(cause)
	StopOpenAICompactSSEKeepaliveCommitted(c)
	MarkOpsStreamFailure(c, "upstream_error", code, message, http.StatusBadGateway)
	MarkResponseCommitted(c)
	// EOF can dispatch a final data line without a blank delimiter. Complete
	// that event before appending the terminal event, preserving its payload.
	if _, err := c.Writer.Write([]byte("\n")); err != nil {
		_ = c.Error(err)
		return
	}
	writeOpenAICompactSSEFailureMessage(c, http.StatusBadGateway, code, message)
}

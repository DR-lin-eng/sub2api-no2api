package service

import (
	"bytes"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAIUpstreamClientErrorFallbackType    = "invalid_request_error"
	openAIUpstreamClientErrorFallbackMessage = "Upstream rejected the request"
	// OpenAIGenericUpstreamFailureClientMessage is the local terminal message
	// used after the bounded account failover budget is exhausted. The upstream
	// sentinel itself is kept for internal classification, but is never useful
	// client-facing information.
	OpenAIGenericUpstreamFailureClientMessage = "Upstream service temporarily unavailable"
	openAIGenericUpstreamFailureMessage       = "Upstream request failed"
)

// IsOpenAIGenericUpstreamFailureBody reports whether body is the generic
// upstream failure envelope used by OpenAI-compatible providers and by the
// gateway's internal transport failover sentinel. It intentionally accepts
// both the top-level OpenAI shape and a Responses response.error shape.
// Specific provider/client errors must retain their existing semantics.
func IsOpenAIGenericUpstreamFailureBody(body []byte) bool {
	if len(bytes.TrimSpace(body)) == 0 || !gjson.ValidBytes(body) {
		return false
	}
	for _, prefix := range []string{"error", "response.error"} {
		message := strings.TrimSpace(gjson.GetBytes(body, prefix+".message").String())
		if !strings.EqualFold(message, openAIGenericUpstreamFailureMessage) {
			continue
		}
		errType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, prefix+".type").String()))
		switch errType {
		case "", "upstream_error", "server_error", "api_error":
			return true
		}
	}
	return false
}

// isOpenAIGenericUpstreamFailure is the service-local form used while the
// response body and already-extracted message are both available. An empty
// body is accepted only when the caller has already classified the message as
// the fixed sentinel (for example a transport-level error).
func isOpenAIGenericUpstreamFailure(upstreamMsg string, body []byte) bool {
	if !strings.EqualFold(strings.TrimSpace(upstreamMsg), openAIGenericUpstreamFailureMessage) {
		return false
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return true
	}
	return IsOpenAIGenericUpstreamFailureBody(body)
}

func isOpenAIDeterministicClientError(statusCode int) bool {
	return statusCode == http.StatusBadRequest
}

func writeOpenAIUpstreamClientError(c *gin.Context, statusCode int, body []byte, upstreamMsg string) {
	errorPayload := gin.H{"type": openAIUpstreamClientErrorFallbackType}
	if errType := strings.TrimSpace(gjson.GetBytes(body, "error.type").String()); errType != "" {
		errorPayload["type"] = errType
	}
	if code := strings.TrimSpace(extractUpstreamErrorCode(body)); code != "" {
		errorPayload["code"] = code
	}
	if param := strings.TrimSpace(gjson.GetBytes(body, "error.param").String()); param != "" {
		errorPayload["param"] = param
	}
	message := strings.TrimSpace(upstreamMsg)
	if message == "" {
		message = openAIUpstreamClientErrorFallbackMessage
	}
	errorPayload["message"] = message

	c.JSON(statusCode, gin.H{"error": errorPayload})
}

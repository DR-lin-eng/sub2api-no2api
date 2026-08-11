package service

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAIUpstreamClientErrorFallbackType    = "invalid_request_error"
	openAIUpstreamClientErrorFallbackMessage = "Upstream rejected the request"
)

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

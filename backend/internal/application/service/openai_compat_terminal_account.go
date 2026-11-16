package service

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *OpenAIGatewayService) applyCompatStreamTerminalAccountError(c *gin.Context, account *Account, payload []byte, message string, headers http.Header, model string) {
	if s == nil || account == nil || isOpenAIUpstreamCapacityShedEvent(payload) {
		return
	}
	status := openAIStreamFailedEventSemanticStatus(payload, message)
	if status != http.StatusUnauthorized && status != http.StatusForbidden && status != http.StatusTooManyRequests {
		return
	}
	ctx := context.Background()
	if c != nil && c.Request != nil {
		ctx = context.WithoutCancel(c.Request.Context())
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	s.handleOpenAIAccountUpstreamError(ctx, account, status, headers, openAIStreamFailedEventPassthroughBody(payload, message), model)
}

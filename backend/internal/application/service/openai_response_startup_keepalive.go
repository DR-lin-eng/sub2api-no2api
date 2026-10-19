package service

import (
	"context"

	"github.com/gin-gonic/gin"
)

func (s *OpenAIGatewayService) startOpenAIResponsePreparationKeepalive(ctx context.Context, c *gin.Context, account *Account, stream bool) func() {
	if !stream || account == nil || !account.IsOpenAI() || OpenAIImageGenerationIntentFromContext(ctx) {
		return func() {}
	}
	return StartOpenAIResponsesSSEKeepalive(c, s.openAIStreamKeepaliveIntervalWithContext(ctx))
}

package service

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// Complete response ownership and quota observation only after a successful
// HTTP terminal response; rejected attempts must not become continuation state.
func (s *OpenAIGatewayService) completeOpenAIHTTPResponse(ctx context.Context, c *gin.Context, account *Account, headers http.Header, responseID string) {
	s.bindStagedOpenAICodexTurnState(c, account, headers)
	s.bindHTTPResponseAccount(ctx, c, account, responseID)
	// Shadow quota snapshots are owned by their explicit usage query.
	if account.Type == AccountTypeOAuth && !account.IsShadow() {
		if snapshot := ParseCodexRateLimitHeaders(headers); snapshot != nil {
			s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
		}
	}
}

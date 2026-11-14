package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// The canonical body retains the downstream stream flag; OAuth preparation has
// already set stream=true in body. Keep this recovery specific to HTTP JSON
// callers whose prewarm opt-in selected WS instead of the usual HTTP upstream.
func (s *OpenAIGatewayService) prepareOpenAIWSNonStreamingHTTPFallback(
	ctx context.Context, c *gin.Context, account *Account, wsErr error, body, canonicalBody []byte,
) bool {
	if account == nil || !account.IsCodexPrewarmContinuationEnabled() ||
		!openAIWSHTTPUnambiguousFields(gjson.ParseBytes(canonicalBody), "stream") ||
		(gjson.GetBytes(canonicalBody, "stream").Exists() && gjson.GetBytes(canonicalBody, "stream").Type != gjson.False) ||
		!s.openAIWSHTTPRecoveryAllowed(ctx, c, account, body, canonicalBody) {
		return false
	}
	status := http.StatusBadGateway
	var failover *UpstreamFailoverError
	var fallback *openAIWSFallbackError
	switch {
	case errors.As(wsErr, &failover):
		if failover.StatusCode < 500 || failover.StatusCode > 599 || failover.SafeToFailoverAfterWrite ||
			failover.Reason == openAIWorkspaceRoutingFailureReason ||
			isOpenAIOAuthPrincipalChangedError(wsErr) {
			return false
		}
		if _, policy := openAIUpstreamPolicyErrorDetails(failover.ResponseBody); policy {
			return false
		}
		status = failover.StatusCode
	case errors.As(wsErr, &fallback):
		switch strings.TrimPrefix(strings.TrimSpace(fallback.Reason), "prewarm_") {
		case "upstream_5xx", "upstream_error_event":
		default:
			return false
		}
	default:
		return false
	}
	s.markOpenAIWSHTTPRecovery(c, account, body, "ws_non_stream_transient_http")
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
		UpstreamStatusCode: status, Kind: "ws_http_fallback", Stage: "websocket",
		Reason: "non_stream_transient", Message: "Upstream WebSocket temporarily unavailable; recovering non-streaming request through HTTP",
	})
	return true
}

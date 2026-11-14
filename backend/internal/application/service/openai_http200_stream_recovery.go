package service

import (
	"context"
	"errors"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAIHTTP200GenericFailureReason GatewayFailureReason = "openai_http200_generic_failure"
	openAIHTTP200RecoveryKey          string               = "openai_http200_recovery_tried"
)

func openAIResponseFailureReportsUsage(payload []byte) bool {
	for _, prefix := range []string{"usage", "response.usage"} {
		for _, name := range []string{"input_tokens", "output_tokens", "total_tokens", "input_tokens_details.cached_tokens", "output_tokens_details.reasoning_tokens"} {
			if gjson.GetBytes(payload, prefix+"."+name).Int() > 0 {
				return true
			}
		}
	}
	return false
}

func openAIResponseHasReportedUsage(usage *OpenAIUsage) bool {
	return usage != nil && (usage.InputTokens > 0 || usage.OutputTokens > 0 ||
		usage.CacheReadInputTokens > 0 || usage.CacheCreationInputTokens > 0 ||
		usage.ImageInputTokens > 0 || usage.ImageOutputTokens > 0 || usage.ImageCacheReadTokens > 0 ||
		(usage.ReasoningTokens != nil && *usage.ReasoningTokens > 0) || usage.SVGUsage != nil)
}

// A 200 response with an explicit generic failure and no output can be a
// transient model attempt. Recover once on the selected account, then leave
// exhaustion and account switching to the existing handler. The request-wide
// flag and passthrough send budget prevent multiplying this recovery.
func (s *OpenAIGatewayService) retryOpenAIHTTP200Failure(ctx context.Context, c *gin.Context, account *Account, body, canonicalBody []byte, usage *OpenAIUsage, err error) (bool, error) {
	var failover *UpstreamFailoverError
	if !errors.As(err, &failover) || failover.Reason != openAIHTTP200GenericFailureReason ||
		ctx == nil || ctx.Err() != nil || c == nil || c.Request == nil || c.Request.Context().Err() != nil ||
		account == nil || !account.IsOpenAI() || s.isOpenAIAccountRuntimeBlocked(account) ||
		s.IsDistillationGroupRequest(c, account) || codexContinuationRecoveryForbidden(c) || codexContinuationRequiresExactConnection(c) ||
		IsResponseCommitted(c) || OpenAICompactKeepaliveAdjustedWrittenSize(c) >= 0 ||
		openAIResponseHasReportedUsage(usage) {
		return false, nil
	}
	if tried, _ := c.Get(openAIHTTP200RecoveryKey); tried == true ||
		!openAIWSHTTPReplaySafeBody(body) || !openAIWSHTTPReplaySafeBody(canonicalBody) {
		return false, nil
	}
	c.Set(openAIHTTP200RecoveryKey, true)
	logger.LegacyPrintf("service.openai_gateway",
		"[OpenAI] Retrying HTTP 200 generic stream failure once: account_id=%d", account.ID)
	if waitErr := waitOpenAITransportRetry(ctx, 250*time.Millisecond); waitErr != nil {
		return false, waitErr
	}
	return true, nil
}

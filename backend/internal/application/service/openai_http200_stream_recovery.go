package service

import (
	"context"
	"errors"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/logger"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const (
	openAIHTTP200GenericFailureReason     GatewayFailureReason = "openai_http200_generic_failure"
	openAIHTTP200RecoveryKey              string               = "openai_http200_recovery_tried"
	openAIOriginalRequestReplaySafeKey    string               = "openai_original_request_replay_safe"
	openAIOriginalRequestStreamKey        string               = "openai_original_request_stream"
	openAIOriginalRequestStreamPresentKey string               = "openai_original_request_stream_present"
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
		!openAIOriginalRequestReplaySafe(c) ||
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

func openAIOriginalRequestReplaySafe(c *gin.Context) bool {
	if c == nil {
		return true
	}
	value, ok := c.Get(openAIOriginalRequestReplaySafeKey)
	if !ok {
		return true
	}
	replaySafe, ok := value.(bool)
	return !ok || replaySafe
}

func openAIOriginalRequestStream(c *gin.Context) bool {
	if c == nil {
		return false
	}
	value, ok := c.Get(openAIOriginalRequestStreamKey)
	stream, valid := value.(bool)
	return ok && valid && stream
}

func openAIOriginalRequestStreamPresent(c *gin.Context, canonicalBody []byte) bool {
	if c != nil {
		if value, ok := c.Get(openAIOriginalRequestStreamPresentKey); ok {
			present, valid := value.(bool)
			return !valid || present
		}
	}
	stream := gjson.GetBytes(canonicalBody, "stream")
	return !stream.Exists() || stream.Type == gjson.False
}

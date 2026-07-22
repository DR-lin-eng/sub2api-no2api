package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (h *OpenAIGatewayHandler) recoverResponsesPanic(c *gin.Context, streamStarted *bool) {
	recovered := recover()
	if recovered == nil {
		return
	}

	started := false
	if streamStarted != nil {
		started = *streamStarted
	}
	wroteFallback := h.ensureForwardErrorResponse(c, started)
	requestLogger(c, "handler.openai_gateway.responses").Error(
		"openai.responses_panic_recovered",
		zap.Bool("fallback_error_response_written", wroteFallback),
		zap.Any("panic", recovered),
		zap.ByteString("stack", debug.Stack()),
	)
}

// recoverAnthropicMessagesPanic recovers from panics in the Anthropic Messages
// handler and returns an Anthropic-formatted error response.
func (h *OpenAIGatewayHandler) recoverAnthropicMessagesPanic(c *gin.Context, streamStarted *bool) {
	recovered := recover()
	if recovered == nil {
		return
	}

	started := streamStarted != nil && *streamStarted
	requestLogger(c, "handler.openai_gateway.messages").Error(
		"openai.messages_panic_recovered",
		zap.Bool("stream_started", started),
		zap.Any("panic", recovered),
		zap.ByteString("stack", debug.Stack()),
	)
	if !started {
		h.anthropicErrorResponse(c, http.StatusInternalServerError, "api_error", "Internal server error")
	}
}

func (h *OpenAIGatewayHandler) ensureResponsesDependencies(c *gin.Context, reqLog *zap.Logger) bool {
	missing := h.missingResponsesDependencies()
	if len(missing) == 0 {
		return true
	}

	if reqLog == nil {
		reqLog = requestLogger(c, "handler.openai_gateway.responses")
	}
	reqLog.Error("openai.handler_dependencies_missing", zap.Strings("missing_dependencies", missing))

	if c != nil && c.Writer != nil && !c.Writer.Written() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": gin.H{
				"type":    "api_error",
				"message": "Service temporarily unavailable",
			},
		})
	}
	return false
}

func (h *OpenAIGatewayHandler) missingResponsesDependencies() []string {
	missing := make([]string, 0, 5)
	if h == nil {
		return append(missing, "handler")
	}
	if h.gatewayService == nil {
		missing = append(missing, "gatewayService")
	}
	if h.billingCacheService == nil {
		missing = append(missing, "billingCacheService")
	}
	if h.apiKeyService == nil {
		missing = append(missing, "apiKeyService")
	}
	if h.concurrencyHelper == nil || h.concurrencyHelper.concurrencyService == nil {
		missing = append(missing, "concurrencyHelper")
	}
	return missing
}

func getContextInt64(c *gin.Context, key string) (int64, bool) {
	if c == nil || key == "" {
		return 0, false
	}
	v, ok := c.Get(key)
	if !ok {
		return 0, false
	}
	switch t := v.(type) {
	case int64:
		return t, true
	case int:
		return int64(t), true
	case int32:
		return int64(t), true
	case float64:
		return int64(t), true
	default:
		return 0, false
	}
}

func (h *OpenAIGatewayHandler) submitUsageRecordTask(parent context.Context, task service.UsageRecordTask) {
	if task == nil {
		return
	}
	task = wrapUsageRecordTaskContext(parent, task)
	if h.usageRecordWorkerPool != nil {
		if mode := h.usageRecordWorkerPool.Submit(task); mode != service.UsageRecordSubmitModeDropped {
			return
		}
		logger.L().With(
			zap.String("component", "handler.openai_gateway.usage"),
		).Warn("openai.usage_record_task_stopped_sync_fallback")
	}
	// 回退路径：worker 池未注入时同步执行，避免退回到无界 goroutine 模式。
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.responses"),
				zap.Any("panic", recovered),
			).Error("openai.usage_record_task_panic_recovered")
		}
	}()
	task(ctx)
}

func (h *OpenAIGatewayHandler) submitOpenAIUsageRecordTask(parent context.Context, result *service.OpenAIForwardResult, task service.UsageRecordTask) {
	if result != nil && result.ImageCount > 0 {
		h.submitMandatoryUsageRecordTask(parent, task)
		return
	}
	h.submitUsageRecordTask(parent, task)
}

func (h *OpenAIGatewayHandler) submitMandatoryUsageRecordTask(parent context.Context, task service.UsageRecordTask) {
	if task == nil {
		return
	}
	task = wrapUsageRecordTaskContext(parent, task)
	if h.usageRecordWorkerPool != nil {
		if mode := h.usageRecordWorkerPool.Submit(task); mode != service.UsageRecordSubmitModeDropped {
			return
		}
		logger.L().With(
			zap.String("component", "handler.openai_gateway.usage"),
		).Warn("openai.usage_record_task_mandatory_sync_fallback")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.usage"),
				zap.Any("panic", recovered),
			).Error("openai.usage_record_task_panic_recovered")
		}
	}()
	task(ctx)
}

func (h *OpenAIGatewayHandler) acquireImageGenerationSlot(c *gin.Context, streamStarted bool) (func(), bool) {
	if h == nil || h.cfg == nil || h.imageLimiter == nil {
		return nil, true
	}
	imageConcurrency := h.cfg.Gateway.ImageConcurrency
	wait := strings.TrimSpace(imageConcurrency.OverflowMode) == config.ImageConcurrencyOverflowModeWait
	release, acquired := h.imageLimiter.Acquire(
		c.Request.Context(),
		imageConcurrency.Enabled,
		imageConcurrency.MaxConcurrentRequests,
		wait,
		time.Duration(imageConcurrency.WaitTimeoutSeconds)*time.Second,
		imageConcurrency.MaxWaitingRequests,
	)
	if acquired {
		return release, true
	}
	h.handleStreamingAwareError(c, http.StatusTooManyRequests, "rate_limit_error", "Image generation concurrency limit exceeded, please retry later", streamStarted)
	return nil, false
}

// handleConcurrencyError handles concurrency-related acquire errors.
func (h *OpenAIGatewayHandler) handleConcurrencyError(c *gin.Context, err error, slotType string, streamStarted bool) {
	status, errType, message := concurrencyErrorResponse(err, slotType)
	h.handleStreamingAwareError(c, status, errType, message, streamStarted)
}

func copyOpenAIPassthroughFailoverHeaders(dst http.Header, src http.Header) {
	if dst == nil || src == nil {
		return
	}
	for _, key := range []string{"Content-Type", "Cache-Control", "Retry-After"} {
		values := src.Values(key)
		if len(values) == 0 {
			continue
		}
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func (h *OpenAIGatewayHandler) handleFailoverExhausted(c *gin.Context, failoverErr *service.UpstreamFailoverError, streamStarted bool) {
	if failoverErr == nil {
		h.handleFailoverExhaustedSimple(c, http.StatusBadGateway, streamStarted)
		return
	}
	if failoverErr.IsOpenAIRequestBodyTooLarge() {
		service.SetOpsUpstreamError(c, http.StatusRequestEntityTooLarge, service.OpenAIRequestBodyTooLargeClientMessage, "")
		h.handleStreamingAwareError(
			c,
			http.StatusRequestEntityTooLarge,
			"invalid_request_error",
			service.OpenAIRequestBodyTooLargeClientMessage,
			streamStarted,
		)
		return
	}
	copyFailoverRetryAfter(c, failoverErr.ResponseHeaders)
	if failoverErr.IsCredentialFailure() {
		status, message := credentialFailoverClientResponse(failoverErr)
		h.handleStreamingAwareError(c, status, "upstream_error", message, streamStarted)
		return
	}
	statusCode := failoverErr.StatusCode
	responseBody := failoverErr.ResponseBody
	if service.StopOpenAICompactSSEKeepaliveCommitted(c) {
		streamStarted = true
	}
	if failoverErr.PreserveUpstreamResponse && !streamStarted && !c.Writer.Written() && !service.IsResponseCommitted(c) {
		copyOpenAIPassthroughFailoverHeaders(c.Writer.Header(), failoverErr.ResponseHeaders)
		contentType := strings.TrimSpace(c.Writer.Header().Get("Content-Type"))
		if contentType == "" {
			contentType = "application/json"
		}
		service.SetOpsUpstreamError(c, statusCode, service.ExtractUpstreamErrorMessage(responseBody), "")
		c.Data(statusCode, contentType, responseBody)
		return
	}
	if service.IsOpenAISilentRefusalErrorBody(responseBody) {
		service.SetOpsUpstreamError(c, statusCode, service.OpenAISilentRefusalClientMessage(), "")
		h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", service.OpenAISilentRefusalClientMessage(), streamStarted)
		return
	}

	// 先检查透传规则
	if h.errorPassthroughService != nil && len(responseBody) > 0 {
		if rule := h.errorPassthroughService.MatchRule("openai", statusCode, responseBody); rule != nil {
			// 确定响应状态码
			respCode := statusCode
			if !rule.PassthroughCode && rule.ResponseCode != nil {
				respCode = *rule.ResponseCode
			}

			// 确定响应消息
			msg := service.ExtractUpstreamErrorMessage(responseBody)
			if !rule.PassthroughBody && rule.CustomMessage != nil {
				msg = *rule.CustomMessage
			}

			if rule.SkipMonitoring {
				c.Set(service.OpsSkipPassthroughKey, true)
			}

			h.handleStreamingAwareError(c, respCode, "upstream_error", msg, streamStarted)
			return
		}
	}

	// 记录原始上游状态码，以便 ops 错误日志捕获真实的上游错误
	upstreamMsg := service.ExtractUpstreamErrorMessage(responseBody)
	service.SetOpsUpstreamError(c, statusCode, upstreamMsg, "")

	// 使用默认的错误映射
	status, errType, errMsg := h.mapUpstreamError(statusCode)
	h.handleStreamingAwareError(c, status, errType, errMsg, streamStarted)
}

func credentialFailoverClientResponse(failoverErr *service.UpstreamFailoverError) (int, string) {
	_ = failoverErr
	return http.StatusServiceUnavailable, service.GrokCredentialUnavailableClientMessage
}

func copyFailoverRetryAfter(c *gin.Context, headers http.Header) {
	if c == nil || headers == nil {
		return
	}
	retryAfter := strings.TrimSpace(headers.Get("Retry-After"))
	if retryAfter == "" || len(retryAfter) > 128 || strings.ContainsAny(retryAfter, "\r\n") || !isSafeRetryAfter(retryAfter) {
		return
	}
	c.Header("Retry-After", retryAfter)
}

func isSafeRetryAfter(value string) bool {
	digitsOnly := true
	for _, char := range value {
		if char < '0' || char > '9' {
			digitsOnly = false
			break
		}
	}
	if digitsOnly {
		seconds, err := strconv.ParseUint(value, 10, 32)
		return err == nil && seconds <= uint64((7*24*time.Hour)/time.Second)
	}
	retryAt, err := http.ParseTime(value)
	if err != nil {
		return false
	}
	return !retryAt.After(time.Now().Add(7 * 24 * time.Hour))
}

// handleFailoverExhaustedSimple 简化版本，用于没有响应体的情况
func (h *OpenAIGatewayHandler) handleFailoverExhaustedSimple(c *gin.Context, statusCode int, streamStarted bool) {
	status, errType, errMsg := h.mapUpstreamError(statusCode)
	service.SetOpsUpstreamError(c, statusCode, errMsg, "")
	h.handleStreamingAwareError(c, status, errType, errMsg, streamStarted)
}

func (h *OpenAIGatewayHandler) mapUpstreamError(statusCode int) (int, string, string) {
	switch statusCode {
	case 401:
		return http.StatusBadGateway, "upstream_error", "Upstream authentication failed, please contact administrator"
	case 403:
		return http.StatusBadGateway, "upstream_error", "Upstream access forbidden, please contact administrator"
	case 429:
		return http.StatusTooManyRequests, "rate_limit_error", "Upstream rate limit exceeded, please retry later"
	case 529:
		return http.StatusServiceUnavailable, "upstream_error", "Upstream service overloaded, please retry later"
	case 500, 502, 503, 504:
		return http.StatusBadGateway, "upstream_error", "Upstream service temporarily unavailable"
	default:
		return http.StatusBadGateway, "upstream_error", "Upstream request failed"
	}
}

// handleStreamingAwareError handles errors that may occur after streaming has started
func (h *OpenAIGatewayHandler) handleStreamingAwareError(c *gin.Context, status int, errType, message string, streamStarted bool) {
	h.handleStreamingAwareErrorWithCode(c, status, errType, "", message, streamStarted, false)
}

func (h *OpenAIGatewayHandler) handleStreamingAwareErrorWithCode(
	c *gin.Context,
	status int,
	errType string,
	code string,
	message string,
	streamStarted bool,
	countTowardsSLA bool,
) {
	// body-signal compact 心跳可能已把响应头提交为 200：先停心跳（建立
	// happens-before，接管 ResponseWriter），并升级为流内错误处理。
	if service.StopOpenAICompactSSEKeepaliveCommitted(c) {
		streamStarted = true
	}
	if service.OpenAIImagesSSEKeepalivePresent(c) {
		service.StopOpenAIImagesJSONKeepaliveCommitted(c)
		streamStarted = true
	}
	if streamStarted {
		setSSEResponseHeaders(c)
		if countTowardsSLA {
			service.MarkOpsStreamFailure(c, errType, code, message, status)
		} else {
			service.MarkOpsStreamError(c, errType, message, status)
		}
		// /v1/responses 的严格 SDK（Codex CLI）要求终止事件必须属于
		// response.completed/failed/incomplete/cancelled 集合。
		// 通用 `event: error` 帧不被识别为终止事件，会导致
		// "stream closed before response.completed"。
		if inboundIsResponses(c) {
			if writeResponsesFailedSSE(c, errType, message) {
				return
			}
		}
		if inboundIsOpenAIImages(c) {
			if writeOpenAIImagesProxyErrorSSE(c, errType, message) {
				return
			}
		}
		// Stream already started, send error as SSE event then close
		flusher, ok := c.Writer.(http.Flusher)
		if ok {
			errorObject := gin.H{"type": errType, "message": message}
			if code != "" {
				errorObject["code"] = code
			}
			payload, err := json.Marshal(gin.H{"error": errorObject})
			if err != nil {
				payload = []byte(`{"error":{"type":"upstream_error","message":"Upstream request failed"}}`)
			}
			errorEvent := "event: error\ndata: " + string(payload) + "\n\n"
			if _, err := fmt.Fprint(c.Writer, errorEvent); err != nil {
				_ = c.Error(err)
			}
			flusher.Flush()
		}
		return
	}

	// Normal case: return JSON response with proper status code
	if code == "" {
		h.errorResponse(c, status, errType, message)
		return
	}
	c.JSON(status, gin.H{"error": gin.H{
		"type": errType, "code": code, "message": message,
	}})
}

func (h *OpenAIGatewayHandler) ensureOpenAIStreamReadErrorResponse(c *gin.Context, err error, streamStarted bool) bool {
	code, message, ok := service.OpenAIUpstreamStreamReadErrorDetails(err)
	if !ok || c == nil || c.Writer == nil || service.IsResponseCommitted(c) {
		return false
	}
	if c.Writer.Written() {
		streamStarted = true
	}
	h.handleStreamingAwareErrorWithCode(
		c, http.StatusBadGateway, "upstream_error", code, message, streamStarted, true,
	)
	return true
}

func inboundIsOpenAIImages(c *gin.Context) bool {
	endpoint := GetInboundEndpoint(c)
	return endpoint == EndpointImagesGenerations || endpoint == EndpointImagesEdits
}

func writeOpenAIImagesProxyErrorSSE(c *gin.Context, errType, message string) bool {
	if c == nil || c.Writer == nil {
		return false
	}
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return false
	}
	eventName := "proxy_error"
	if strings.TrimSpace(errType) == "upstream_error" {
		eventName = "upstream_error"
	}
	setSSEResponseHeaders(c)
	payload := `{"error":{"type":` + strconv.Quote(errType) + `,"message":` + strconv.Quote(message) + `}}`
	if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", eventName, payload); err != nil {
		_ = c.Error(err)
		return false
	}
	flusher.Flush()
	return true
}

// ensureForwardErrorResponse 在 Forward 返回错误但尚未写响应时补写统一错误响应。
func (h *OpenAIGatewayHandler) ensureForwardErrorResponse(c *gin.Context, streamStarted bool) bool {
	if c == nil || c.Writer == nil {
		return false
	}
	// 先停 compact 心跳再读 Writer 状态，避免与心跳 goroutine 竞争。
	compactKeepaliveCommitted := service.StopOpenAICompactSSEKeepaliveCommitted(c)
	if compactKeepaliveCommitted {
		streamStarted = true
	}
	imageKeepalivePresent := service.OpenAIImagesJSONKeepalivePresent(c)
	service.StopOpenAIImagesJSONKeepaliveCommitted(c)
	imageKeepalivePaddingOnly := false
	imageKeepaliveResponseWritten := false
	if imageKeepalivePresent {
		adjustedSize := service.OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c)
		imageKeepalivePaddingOnly = adjustedSize < 0
		imageKeepaliveResponseWritten = adjustedSize >= 0
	}
	if service.IsResponseCommitted(c) || (!compactKeepaliveCommitted && imageKeepaliveResponseWritten) {
		return false
	}
	if c.Writer.Written() && !imageKeepalivePaddingOnly {
		streamStarted = true
	}
	h.handleStreamingAwareError(c, http.StatusBadGateway, "upstream_error", "Upstream request failed", streamStarted)
	return true
}

func shouldLogOpenAIForwardFailureAsWarn(c *gin.Context, wroteFallback bool) bool {
	if wroteFallback {
		return false
	}
	if c == nil || c.Writer == nil {
		return false
	}
	return c.Writer.Written()
}

// openAIForwardErrorAlreadyCommunicated reports whether Forward returned an
// error after it had already written the upstream terminal error response to
// the client.
//
// This matters for Responses streams: upstream may return HTTP 200 with a
// non-retryable `response.failed` event (for example a policy/safety rejection).
// The service layer forwards that terminal event verbatim, then returns an
// error so the caller can log/account for the failed upstream response. The
// handler must not append its generic fallback `response.failed`, otherwise
// strict clients may see the useful upstream message replaced by "Upstream
// request failed" or receive duplicate terminal events.
func openAIForwardErrorAlreadyCommunicated(c *gin.Context, writerSizeBeforeForward int, err error) bool {
	if err == nil || c == nil || c.Writer == nil {
		return false
	}
	// 与快照同口径：排除 compact 心跳字节，避免"仅心跳写出"被误判为
	// 响应已写出（#3887）。
	if service.OpenAICompactKeepaliveAdjustedWrittenSize(c) == writerSizeBeforeForward ||
		service.OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c) == writerSizeBeforeForward {
		return false
	}

	// cyber_policy 命中时上游原始错误体已透传给客户端（非流式 c.Data 写出 400 body，
	// 流式写出 response.failed 事件），不能再让 ensureForwardErrorResponse 追加
	// fallback —— 否则在已写出的完整响应尾部追加 SSE（responses 端点尾随
	// response.failed、chat 端点尾随 event:error），污染响应体。Size 已变化证明响应确已写出。
	if service.GetOpsCyberPolicy(c) != nil {
		return true
	}

	msg := strings.TrimSpace(err.Error())
	for _, prefix := range []string{
		"upstream response failed:",
		"non-streaming openai protocol error:",
	} {
		if strings.HasPrefix(msg, prefix) {
			return true
		}
	}
	return false
}

func openAIForwardMayFailover(c *gin.Context, writerSizeBeforeForward int, failoverErr *service.UpstreamFailoverError) bool {
	if c == nil || c.Writer == nil {
		return false
	}
	if service.OpenAICompactKeepaliveAdjustedWrittenSize(c) == writerSizeBeforeForward {
		return true
	}
	if service.OpenAIImagesJSONKeepaliveAdjustedWrittenSize(c) == writerSizeBeforeForward {
		return true
	}
	return failoverErr != nil && failoverErr.SafeToFailoverAfterWrite
}

func openAIRequestAllowsFailoverReplay(c *gin.Context) bool {
	if c == nil || c.Request == nil {
		return false
	}
	return !failoverClientGone(c)
}

func openAIFirstOutputFailoverExhausted(failoverErr *service.UpstreamFailoverError, switchCount *int) bool {
	if failoverErr == nil || !failoverErr.SafeToFailoverAfterWrite || switchCount == nil {
		return false
	}
	if *switchCount >= maxOpenAIFirstOutputTimeoutSwitches {
		return true
	}
	*switchCount = *switchCount + 1
	return false
}

// errorResponse returns OpenAI API format error response
func (h *OpenAIGatewayHandler) errorResponse(c *gin.Context, status int, errType, message string) {
	// body-signal compact 心跳可能已把响应头提交为 200：JSON 错误体会与已
	// 提交的 SSE 流交错，必须降级为 response.failed 终止事件（#3887）。
	if service.StopOpenAICompactSSEKeepaliveCommitted(c) {
		service.MarkOpsStreamError(c, errType, message, status)
		if writeResponsesFailedSSE(c, errType, message) {
			return
		}
	}
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// openAICompactKeepaliveInterval 复用流式 keepalive 配置作为 compact 下游
// 心跳间隔；0 表示禁用（与流式路径语义一致）。
func (h *OpenAIGatewayHandler) openAICompactKeepaliveInterval() time.Duration {
	if h.cfg == nil || h.cfg.Gateway.StreamKeepaliveInterval <= 0 {
		return 0
	}
	return time.Duration(h.cfg.Gateway.StreamKeepaliveInterval) * time.Second
}

func setOpenAIClientTransportHTTP(c *gin.Context) {
	service.SetOpenAIClientTransport(c, service.OpenAIClientTransportHTTP)
}

func setOpenAIClientTransportWS(c *gin.Context) {
	service.SetOpenAIClientTransport(c, service.OpenAIClientTransportWS)
}

func ensureOpenAIPoolModeSessionHash(sessionHash string, account *service.Account) string {
	if sessionHash != "" || account == nil || !account.IsPoolMode() {
		return sessionHash
	}
	// 为当前请求生成一次性粘性会话键，确保同账号重试不会重新负载均衡到其他账号。
	return "openai-pool-retry-" + uuid.NewString()
}

func openAIWSIngressFallbackSessionSeed(userID, apiKeyID int64, groupID *int64) string {
	gid := int64(0)
	if groupID != nil {
		gid = *groupID
	}
	return fmt.Sprintf("openai_ws_ingress:%d:%d:%d", gid, userID, apiKeyID)
}

func isOpenAIWSUpgradeRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Upgrade")), "websocket") {
		return false
	}
	return strings.Contains(strings.ToLower(strings.TrimSpace(r.Header.Get("Connection"))), "upgrade")
}

func closeOpenAIClientWS(conn *coderws.Conn, status coderws.StatusCode, reason string) {
	if conn == nil {
		return
	}
	reason = strings.TrimSpace(reason)
	if len(reason) > 120 {
		reason = reason[:120]
	}
	_ = conn.Close(status, reason)
	_ = conn.CloseNow()
}

func closeOpenAIWSFailoverExhausted(conn *coderws.Conn, failoverErr *service.UpstreamFailoverError) {
	if failoverErr == nil {
		closeOpenAIClientWS(conn, coderws.StatusInternalError, "upstream websocket proxy failed")
		return
	}
	if failoverErr.Stage == service.GatewayFailureStageAccountAuth {
		closeOpenAIClientWS(conn, coderws.StatusTryAgainLater, service.GrokCredentialUnavailableClientMessage)
		return
	}
	switch failoverErr.StatusCode {
	case http.StatusTooManyRequests:
		closeOpenAIClientWS(conn, coderws.StatusTryAgainLater, "upstream rate limit exceeded, please retry later")
	case 529, http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		closeOpenAIClientWS(conn, coderws.StatusTryAgainLater, "upstream service temporarily unavailable")
	case http.StatusUnauthorized, http.StatusForbidden:
		closeOpenAIClientWS(conn, coderws.StatusPolicyViolation, "upstream websocket authentication failed")
	default:
		closeOpenAIClientWS(conn, coderws.StatusInternalError, "upstream websocket proxy failed")
	}
}

func writeContentModerationWSError(ctx context.Context, conn *coderws.Conn, decision *service.ContentModerationDecision) {
	if conn == nil || decision == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	message := strings.TrimSpace(decision.Message)
	if message == "" {
		message = "content moderation blocked this request"
	}
	payload, err := json.Marshal(gin.H{
		"event_id": "evt_content_moderation_blocked",
		"type":     "error",
		"error": gin.H{
			"type":    "invalid_request_error",
			"code":    contentModerationErrorCode(decision),
			"message": message,
		},
	})
	if err != nil {
		payload = []byte(`{"event_id":"evt_content_moderation_blocked","type":"error","error":{"type":"invalid_request_error","code":"content_policy_violation","message":"content moderation blocked this request"}}`)
	}
	writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_ = conn.Write(writeCtx, coderws.MessageText, payload)
}

// writeCyberSessionBlockedWSError sends an error frame telling the client this
// session is blocked by the cyber session block (F5a) before closing.
func writeCyberSessionBlockedWSError(ctx context.Context, conn *coderws.Conn) {
	if conn == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	payload, err := json.Marshal(gin.H{
		"event_id": "evt_cyber_session_blocked",
		"type":     "error",
		"error": gin.H{
			"type":    "permission_error",
			"code":    "session_blocked_by_cyber_policy",
			"message": cyberSessionBlockedClientMsg,
		},
	})
	if err != nil {
		payload = []byte(`{"event_id":"evt_cyber_session_blocked","type":"error","error":{"type":"permission_error","code":"session_blocked_by_cyber_policy","message":"This session is blocked by cyber-security policy, please start a new session"}}`)
	}
	writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_ = conn.Write(writeCtx, coderws.MessageText, payload)
}

// cyberPolicyRecordedKey guards against double-firing recordCyberPolicyIfMarked
// within one request (e.g. in a retry/failover loop).

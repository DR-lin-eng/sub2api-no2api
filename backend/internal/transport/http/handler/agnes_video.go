package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/shared/httputil"
	"github.com/Wei-Shaw/sub2api/internal/shared/ip"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// AgnesVideoGeneration handles Agnes video generation.
func (h *OpenAIGatewayHandler) AgnesVideoGeneration(c *gin.Context) {
	h.handleAgnesVideo(c, service.AgnesVideoEndpointGenerations, "")
}

// AgnesVideoStatus handles Agnes video status retrieval.
func (h *OpenAIGatewayHandler) AgnesVideoStatus(c *gin.Context) {
	h.handleAgnesVideo(c, service.AgnesVideoEndpointStatus, c.Param("request_id"))
}

// AgnesVideoContent proxies downloadable video content.
func (h *OpenAIGatewayHandler) AgnesVideoContent(c *gin.Context) {
	h.handleAgnesVideo(c, service.AgnesVideoEndpointContent, c.Param("request_id"))
}

func (h *OpenAIGatewayHandler) ResolveCustomModelVideoAPIType(ctx context.Context, model string) (string, bool, error) {
	if h == nil || h.gatewayService == nil {
		return "", false, nil
	}
	return h.gatewayService.ResolveCustomModelVideoAPIType(ctx, model)
}

func (h *OpenAIGatewayHandler) HasAgnesVideoTaskBinding(c *gin.Context, taskID string) bool {
	if h == nil || h.gatewayService == nil || c == nil {
		return false
	}
	apiKey, apiKeyOK := middleware2.GetAPIKeyFromContext(c)
	subject, subjectOK := middleware2.GetAuthSubjectFromContext(c)
	if !apiKeyOK || !subjectOK {
		return false
	}
	accountID, err := h.gatewayService.ResolveAgnesVideoTaskAccount(
		c.Request.Context(), apiKey.GroupID, taskID, subject.UserID, apiKey.ID,
	)
	return err == nil && accountID > 0
}

func (h *OpenAIGatewayHandler) handleAgnesVideo(c *gin.Context, endpoint service.AgnesVideoEndpoint, taskID string) {
	streamStarted := false
	defer h.recoverResponsesPanic(c, &streamStarted)

	requestStart := time.Now()
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}

	reqLog := requestLogger(
		c,
		"handler.openai_gateway.agnes_video",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
		zap.String("endpoint", string(endpoint)),
	)
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}

	var body []byte
	var err error
	var contentType string
	if endpoint == service.AgnesVideoEndpointGenerations {
		body, err = pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		if err != nil {
			if maxErr, ok := extractMaxBytesError(err); ok {
				h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
				return
			}
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
			return
		}
		if len(body) == 0 {
			h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
			return
		}
		contentType = c.GetHeader("Content-Type")
		h.concurrencyHelper.SetPriorityAdmissionPendingBytes(c, int64(len(body)))
	}

	requestInfo := service.ParseAgnesVideoRequest(body)
	requestModel := requestInfo.Model
	if endpoint == service.AgnesVideoEndpointGenerations && strings.TrimSpace(requestModel) == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	if (endpoint == service.AgnesVideoEndpointStatus || endpoint == service.AgnesVideoEndpointContent) && strings.TrimSpace(taskID) == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "task_id is required")
		return
	}

	reqLog = reqLog.With(zap.String("model", requestModel))
	setOpsRequestContext(c, requestModel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))

	if endpoint == service.AgnesVideoEndpointGenerations {
		if !service.GroupAllowsMediaStudioGeneration(apiKey, "video") {
			h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
			return
		}
		if len(requestInfo.Prompt) > 0 {
			decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIImages, requestModel, []byte(requestInfo.Prompt))
			if decision != nil && !decision.AllowNextStage {
				h.openAISecurityAuditError(c, decision)
				return
			}
		}
		imageReleaseFunc, acquired := h.acquireImageGenerationSlot(c, streamStarted)
		if !acquired {
			return
		}
		if imageReleaseFunc != nil {
			defer imageReleaseFunc()
		}
	}

	if h.errorPassthroughService != nil {
		service.BindErrorPassthroughService(c, h.errorPassthroughService)
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())

	userReleaseFunc, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("agnes_video.billing_eligibility_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	sessionSeed := body
	if len(sessionSeed) == 0 && strings.TrimSpace(taskID) != "" {
		sessionSeed = []byte(taskID)
	}
	sessionHash := h.gatewayService.GenerateExplicitSessionHash(c, sessionSeed)
	boundLookupAccountID := int64(0)
	if endpoint == service.AgnesVideoEndpointStatus || endpoint == service.AgnesVideoEndpointContent {
		sessionHash = service.AgnesVideoTaskSessionHash(taskID, subject.UserID, apiKey.ID)
		boundLookupAccountID, err = h.gatewayService.ResolveAgnesVideoTaskAccount(
			c.Request.Context(), apiKey.GroupID, taskID, subject.UserID, apiKey.ID,
		)
		if err != nil || boundLookupAccountID <= 0 {
			reqLog.Info("agnes_video.task_owner_binding_missing", zap.Error(err))
			h.errorResponse(c, http.StatusNotFound, "not_found_error", "Video request not found")
			return
		}
	}
	requestCtx := c.Request.Context()
	var failedAccountIDs map[int64]struct{}
	var lastFailoverErr *service.UpstreamFailoverError
	switchCount := 0
	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
	}
	routingStart := time.Now()
	var requiredCapability service.OpenAIEndpointCapability

	for {
		if failoverClientGone(c) {
			return
		}
		selection, scheduleDecision, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
			requestCtx,
			apiKey.GroupID,
			"",
			sessionHash,
			requestModel,
			failedAccountIDs,
			service.OpenAIUpstreamTransportHTTPSSE,
			requiredCapability,
			false,
			false,
			false,
			service.PlatformOpenAI,
		)
		_ = scheduleDecision
		if err != nil {
			if failoverClientGone(c) {
				reqLog.Info("agnes_video.account_select_aborted_client_disconnected", zap.Error(err))
				return
			}
			reqLog.Warn("agnes_video.account_select_failed",
				zap.Error(err),
				// This set contains accounts that were selected earlier and then
				// failed over or became unschedulable while waiting. It is not the
				// scheduler's capability-filter count.
				zap.Int("failed_account_count", len(failedAccountIDs)),
			)
			if errors.Is(err, service.ErrPriorityAdmissionUnavailable) {
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable, please retry later")
				return
			}
			if len(failedAccountIDs) == 0 {
				cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, requestModel, requestModel, service.PlatformOpenAI)
				if !cls.ModelNotFound {
					markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				}
				h.errorResponse(c, cls.Status, cls.ErrType, cls.Message)
				return
			}
			if lastFailoverErr != nil {
				h.handleFailoverExhausted(c, lastFailoverErr, false)
			} else {
				h.errorResponse(c, http.StatusBadGateway, "api_error", "Upstream request failed")
			}
			return
		}
		if selection == nil || selection.Account == nil {
			cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, requestModel, requestModel, service.PlatformOpenAI)
			if !cls.ModelNotFound {
				markOpsRoutingCapacityLimited(c)
			}
			h.errorResponse(c, cls.Status, cls.ErrType, cls.Message)
			return
		}
		if boundLookupAccountID > 0 && selection.Account.ID != boundLookupAccountID {
			reqLog.Warn("agnes_video.task_bound_account_unavailable",
				zap.Int64("bound_account_id", boundLookupAccountID),
				zap.Int64("selected_account_id", selection.Account.ID),
			)
			h.errorResponse(c, http.StatusNotFound, "not_found_error", "Video request not found")
			return
		}

		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)
		c.Request = c.Request.WithContext(service.WithAccountEgressContext(c.Request.Context(), account, h.cfg))

		accountReleaseFunc, acquireOutcome := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, false, &streamStarted, reqLog)
		if acquireOutcome == accountSlotAcquireReschedule {
			addFailedAccountID(&failedAccountIDs, account.ID)
			continue
		}
		if acquireOutcome != accountSlotAcquireReady {
			return
		}

		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardStart := time.Now()
		writerSizeBeforeForward := c.Writer.Size()
		result, err := func() (*service.OpenAIForwardResult, error) {
			defer func() {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
				}
			}()
			return h.gatewayService.ForwardAgnesVideo(requestCtx, c, account, endpoint, taskID, body, contentType)
		}()

		forwardDurationMs := time.Since(forwardStart).Milliseconds()
		upstreamLatencyMs, _ := getContextInt64(c, service.OpsUpstreamLatencyMsKey)
		responseLatencyMs := forwardDurationMs
		if upstreamLatencyMs > 0 && forwardDurationMs > upstreamLatencyMs {
			responseLatencyMs = forwardDurationMs - upstreamLatencyMs
		}
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, responseLatencyMs)

		if err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				lastFailoverErr = failoverErr
				if failoverClientGone(c) {
					reqLog.Info("agnes_video.failover_aborted_client_disconnected",
						zap.Int64("account_id", account.ID),
						zap.Int("upstream_status", failoverErr.StatusCode),
					)
					return
				}
				if failoverErr.ShouldReportAccountScheduleFailure() {
					h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, requestModel, false, nil)
				}
				if c.Writer.Size() != writerSizeBeforeForward {
					reqLog.Warn("agnes_video.failover_aborted_partial_write",
						zap.Int64("account_id", account.ID),
						zap.Int("upstream_status", failoverErr.StatusCode),
					)
					return
				}
				if !failoverErr.ShouldRetryNextAccount() {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
				}
				if endpoint == service.AgnesVideoEndpointStatus || endpoint == service.AgnesVideoEndpointContent {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
				}
				addFailedAccountID(&failedAccountIDs, account.ID)
				if switchCount >= maxAccountSwitches {
					continue
				}
				switchCount++
				continue
			}
			reqLog.Error("agnes_video.forward_error", zap.Error(err))
			h.errorResponse(c, http.StatusBadGateway, "api_error", "Upstream request failed")
			return
		}

		if result == nil {
			reqLog.Error("agnes_video.nil_result")
			h.errorResponse(c, http.StatusInternalServerError, "api_error", "Internal error")
			return
		}

		h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, requestModel, true, nil)
		if endpoint == service.AgnesVideoEndpointGenerations && strings.TrimSpace(result.ResponseID) != "" {
			if err := h.gatewayService.BindAgnesVideoTaskAccount(
				requestCtx, apiKey.GroupID, result.ResponseID, subject.UserID, apiKey.ID, account.ID,
			); err != nil {
				reqLog.Warn("agnes_video.bind_task_account_failed",
					zap.Int64("account_id", account.ID),
					zap.String("task_id", result.ResponseID),
					zap.Error(err),
				)
			}
		}

		if endpoint == service.AgnesVideoEndpointGenerations && strings.TrimSpace(requestModel) != "" {
			recordAgnesVideoUsage(c, h, reqLog, apiKey, subject, subscription, account, result, requestModel, body, taskID)
		}

		// 响应已经在 service 层通过 writeAgnesVideoResponse 写入 gin.Context
		return
	}
}

func recordAgnesVideoUsage(
	c *gin.Context,
	h *OpenAIGatewayHandler,
	reqLog *zap.Logger,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	subscription *service.UserSubscription,
	account *service.Account,
	result *service.OpenAIForwardResult,
	requestModel string,
	body []byte,
	taskID string,
) {
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	sessionID := service.ExtractClientSessionID(c)
	payloadForHash := body
	if len(payloadForHash) == 0 && strings.TrimSpace(taskID) != "" {
		payloadForHash = []byte(taskID)
	}
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := GetUpstreamEndpoint(c, account.Platform)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	channelUsageFields := service.ChannelUsageFields{
		OriginalModel:      clientRequestedModel(c, requestModel),
		ChannelMappedModel: requestModel,
	}

	h.submitOpenAIUsageRecordTask(c.Request.Context(), result, func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
			Result:             result,
			APIKey:             apiKey,
			User:               apiKey.User,
			Account:            account,
			Subscription:       subscription,
			InboundEndpoint:    inboundEndpoint,
			UpstreamEndpoint:   upstreamEndpoint,
			UserAgent:          userAgent,
			IPAddress:          clientIP,
			RequestPayloadHash: service.HashUsageRequestPayload(payloadForHash),
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      quotaPlatform,
			SessionID:          sessionID,
			ChannelUsageFields: channelUsageFields,
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.agnes_video"),
				zap.Int64("user_id", subject.UserID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Any("group_id", apiKey.GroupID),
				zap.String("model", requestModel),
				zap.Int64("account_id", account.ID),
			).Error("agnes_video.record_usage_failed", zap.Error(err))
			reqLog.Debug("agnes_video.record_usage_failed", zap.Error(err))
		}
	})
}

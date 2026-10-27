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

// MultimodalImages handles image generation/editing through multimodal groups.
func (h *OpenAIGatewayHandler) MultimodalImages(c *gin.Context) {
	endpoint := service.MultimodalMediaEndpointImagesGenerations
	if strings.Contains(c.Request.URL.Path, "/images/edits") {
		endpoint = service.MultimodalMediaEndpointImagesEdits
	}
	h.handleMultimodalMedia(c, endpoint, "")
}

// MultimodalVideoGeneration handles video generation through multimodal groups.
func (h *OpenAIGatewayHandler) MultimodalVideoGeneration(c *gin.Context) {
	h.handleMultimodalMedia(c, service.MultimodalMediaEndpointVideosGenerations, "")
}

// MultimodalVideoEdit handles asynchronous video edits through multimodal groups.
func (h *OpenAIGatewayHandler) MultimodalVideoEdit(c *gin.Context) {
	h.handleMultimodalMedia(c, service.MultimodalMediaEndpointVideosEdits, "")
}

// MultimodalVideoExtension handles asynchronous video extensions through multimodal groups.
func (h *OpenAIGatewayHandler) MultimodalVideoExtension(c *gin.Context) {
	h.handleMultimodalMedia(c, service.MultimodalMediaEndpointVideosExtensions, "")
}

// MultimodalVideoStatus handles multimodal video status retrieval.
func (h *OpenAIGatewayHandler) MultimodalVideoStatus(c *gin.Context) {
	h.handleMultimodalMedia(c, service.MultimodalMediaEndpointVideoStatus, c.Param("request_id"))
}

// MultimodalVideoContent proxies downloadable video content through the task's upstream account.
func (h *OpenAIGatewayHandler) MultimodalVideoContent(c *gin.Context) {
	h.handleMultimodalMedia(c, service.MultimodalMediaEndpointVideoContent, c.Param("request_id"))
}

func (h *OpenAIGatewayHandler) handleMultimodalMedia(c *gin.Context, endpoint service.MultimodalMediaEndpoint, requestID string) {
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
		"handler.openai_gateway.multimodal_media",
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
	if endpoint.RequiresRequestBody() {
		body, ok = pkghttputil.BufferedRequestBody(c.Request)
		if !ok {
			body, err = pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
		}
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
		h.concurrencyHelper.SetPriorityAdmissionPendingBytes(c, int64(len(body)))
	}

	contentType := c.GetHeader("Content-Type")
	requestInfo := service.ParseGrokMediaRequest(contentType, body)
	requestModel := requestInfo.Model
	routingModel := requestModel
	if endpoint.IsGenerationRequest() && strings.TrimSpace(requestModel) == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	if endpoint.IsVideoLookupRequest() && strings.TrimSpace(requestID) == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "request_id is required")
		return
	}

	reqLog = reqLog.With(zap.String("model", requestModel))
	setOpsRequestContext(c, requestModel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))

	if endpoint.IsGenerationRequest() {
		mediaType := "video"
		if endpoint.IsImageRequest() {
			mediaType = "image"
		}
		if !service.GroupAllowsMediaStudioGeneration(apiKey, mediaType) {
			h.errorResponse(c, http.StatusForbidden, "permission_error", service.ImageGenerationPermissionMessage())
			return
		}
		if moderationBody := requestInfo.ModerationBody(); len(moderationBody) > 0 {
			decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIImages, requestModel, moderationBody)
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
		reqLog.Info("multimodal_media.billing_eligibility_check_failed", zap.Error(err))
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	sessionSeed := body
	if len(sessionSeed) == 0 && strings.TrimSpace(requestID) != "" {
		sessionSeed = []byte(requestID)
	}
	sessionHash := h.gatewayService.GenerateExplicitSessionHash(c, sessionSeed)
	boundLookupAccountID := int64(0)
	if endpoint.IsVideoLookupRequest() {
		sessionHash = service.MultimodalMediaVideoRequestSessionHash(requestID, subject.UserID, apiKey.ID)
		boundLookupAccountID, err = h.gatewayService.ResolveMultimodalMediaVideoRequestAccount(
			c.Request.Context(), apiKey.GroupID, requestID, subject.UserID, apiKey.ID,
		)
		if err != nil || boundLookupAccountID <= 0 {
			// 归属缓存可能因 Redis 故障或 TTL 过期丢失；已冻结或已结算的任务
			// 必须仍能用冻结记录里的账号恢复轮询与取片，直接 404 等于白拿视频。
			recovered := h.gatewayService.ResolveMultimodalVideoBalanceAccountID(
				c.Request.Context(), requestID, subject.UserID, apiKey.ID,
			)
			if recovered <= 0 {
				reqLog.Info("multimodal_media.video_lookup_owner_binding_missing", zap.Error(err))
				h.errorResponse(c, http.StatusNotFound, "not_found_error", "Video request not found")
				return
			}
			boundLookupAccountID = recovered
			reqLog.Warn("multimodal_media.video_lookup_owner_recovered_from_hold",
				zap.String("request_id", requestID),
				zap.Int64("account_id", recovered),
				zap.Error(err),
			)
			if bindErr := h.gatewayService.BindMultimodalMediaVideoRequestAccount(
				c.Request.Context(), apiKey.GroupID, requestID, subject.UserID, apiKey.ID, recovered,
			); bindErr != nil {
				reqLog.Warn("multimodal_media.video_lookup_owner_rebind_failed", zap.Error(bindErr))
			}
		}
	}
	requestCtx := c.Request.Context()
	var failedAccountIDs map[int64]struct{}
	var sameAccountRetryCount map[int64]int
	var lastFailoverErr *service.UpstreamFailoverError
	var oauth429FailoverState service.OpenAIOAuth429FailoverState
	switchCount := 0
	maxAccountSwitches := h.maxAccountSwitches
	if maxAccountSwitches <= 0 {
		maxAccountSwitches = 3
	}
	routingStart := time.Now()
	requiredCapability := multimodalMediaRequiredCapability(endpoint)
	var accountReleaseFunc func()
	releaseAccount := func() {
		if accountReleaseFunc != nil {
			accountReleaseFunc()
			accountReleaseFunc = nil
		}
	}
	defer releaseAccount()

	// 视频生成走两段式计费：提交上游前冻结预估金额，成功拿到任务 id 后交由
	// 轮询阶段结算；任何未成功提交上游的退出路径都在这里全额释放。
	var videoHold *service.BalanceHoldRecord
	videoHoldBound := false
	// 两段式开启后，创建阶段一律不写用量、不直扣：失败由轮询阶段全额释放，
	// 成功在轮询结算时落唯一一条流水。订阅制与简单模式仍走原直扣路径。
	videoHoldEnabled := endpoint.IsVideoGenerationRequest() && h.gatewayService.MultimodalVideoBalanceHoldEnabled(apiKey)
	defer func() {
		if videoHold == nil || videoHoldBound {
			return
		}
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if releaseErr := h.gatewayService.ReleaseMultimodalVideoBalance(releaseCtx, videoHold); releaseErr != nil {
			reqLog.Warn("multimodal_media.release_video_balance_hold_failed", zap.Error(releaseErr))
		}
	}()

	// 未知视频分辨率不能静默按 480p 计费：提交上游前直接拒绝，避免少扣费。
	if endpoint.IsVideoGenerationRequest() {
		if resolutionErr := service.ValidateMultimodalVideoResolution(requestInfo.ResolutionRaw); resolutionErr != nil {
			reqLog.Warn("multimodal_media.video_resolution_unsupported",
				zap.String("model", requestModel),
				zap.String("resolution", requestInfo.ResolutionRaw),
				zap.Error(resolutionErr),
			)
			status, code, message, retryAfter := billingErrorDetails(resolutionErr)
			if retryAfter > 0 {
				c.Header("Retry-After", strconv.Itoa(retryAfter))
			}
			h.errorResponse(c, status, code, message)
			return
		}
	}

	// 未启用两段式冻结（订阅制分组、简单模式）时创建期不会直扣，必须在提交上游前
	// 校验视频单价可解析；缺失时直接拒绝，避免生成完成后无法计费。
	if endpoint.IsVideoGenerationRequest() && !videoHoldEnabled {
		var pricingGroupID int64
		if apiKey.GroupID != nil {
			pricingGroupID = *apiKey.GroupID
		}
		if pricingErr := h.gatewayService.ValidateMultimodalVideoPricing(requestCtx, service.VideoBalanceHoldRequest{
			APIKey:          apiKey,
			UserID:          subject.UserID,
			GroupID:         pricingGroupID,
			Model:           requestModel,
			Resolution:      requestInfo.Resolution,
			DurationSeconds: service.MultimodalVideoBillingDurationSeconds(requestInfo, body),
		}); pricingErr != nil {
			reqLog.Warn("multimodal_media.video_pricing_unavailable",
				zap.String("model", requestModel),
				zap.Error(pricingErr),
			)
			status, code, message, retryAfter := billingErrorDetails(pricingErr)
			if retryAfter > 0 {
				c.Header("Retry-After", strconv.Itoa(retryAfter))
			}
			h.errorResponse(c, status, code, message)
			return
		}
	}

	// 图片用量由异步任务记录，缺价只会写日志而不会让请求失败；这里在提交上游前
	// 校验图片单价可解析，逐模型定价分组未配置该模型图片价时直接拒绝。
	if endpoint.IsImageRequest() {
		var pricingGroupID int64
		if apiKey.GroupID != nil {
			pricingGroupID = *apiKey.GroupID
		}
		if pricingErr := h.gatewayService.ValidateMultimodalImagePricing(requestCtx, service.MultimodalImagePricingRequest{
			APIKey:  apiKey,
			UserID:  subject.UserID,
			GroupID: pricingGroupID,
			Model:   requestModel,
			Size:    requestInfo.Size,
			Count:   requestInfo.N,
		}); pricingErr != nil {
			reqLog.Warn("multimodal_media.image_pricing_unavailable",
				zap.String("model", requestModel),
				zap.String("size", requestInfo.Size),
				zap.Error(pricingErr),
			)
			status, code, message, retryAfter := billingErrorDetails(pricingErr)
			if retryAfter > 0 {
				c.Header("Retry-After", strconv.Itoa(retryAfter))
			}
			h.errorResponse(c, status, code, message)
			return
		}
	}

	for {
		releaseAccount()
		if failoverClientGone(c) {
			return
		}
		var selection *service.AccountSelectionResult
		var scheduleDecision service.OpenAIAccountScheduleDecision
		if boundLookupAccountID > 0 {
			selection, scheduleDecision, err = h.gatewayService.SelectMultimodalMediaVideoRequestAccount(
				requestCtx, apiKey.GroupID, sessionHash, boundLookupAccountID, routingModel,
			)
		} else {
			selection, scheduleDecision, err = h.gatewayService.SelectAccountWithSchedulerForCapability(
				requestCtx,
				apiKey.GroupID,
				"",
				sessionHash,
				routingModel,
				failedAccountIDs,
				service.OpenAIUpstreamTransportHTTPSSE,
				requiredCapability,
				false,
				false,
				false,
				service.PlatformMultimodal,
			)
		}
		// Own a scheduler-acquired slot before forwarding; every exit path
		// below releases it through the same once-only function.
		if selection != nil && selection.Acquired && selection.ReleaseFunc != nil {
			accountReleaseFunc = selection.ReleaseFunc
		}
		if err != nil {
			if failoverClientGone(c) {
				reqLog.Info("multimodal_media.account_select_aborted_client_disconnected", zap.Error(err))
				return
			}
			if boundLookupAccountID > 0 && errors.Is(err, service.ErrNoAvailableAccounts) {
				h.errorResponse(c, http.StatusNotFound, "not_found_error", "Video request not found")
				return
			}
			reqLog.Warn("multimodal_media.account_select_failed",
				zap.Error(err),
				zap.Int("excluded_account_count", len(failedAccountIDs)),
			)
			if errors.Is(err, service.ErrPriorityAdmissionUnavailable) {
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable, please retry later")
				return
			}
			if endpoint.IsGenerationRequest() && errors.Is(err, service.ErrNoAvailableAccounts) && len(failedAccountIDs) == 0 {
				markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				h.errorResponse(c, http.StatusServiceUnavailable, "multimodal_media_no_eligible_account", "No eligible multimodal media accounts")
				return
			}
			if len(failedAccountIDs) == 0 {
				cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, requestModel, routingModel, service.PlatformMultimodal)
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
			if endpoint.IsGenerationRequest() {
				markOpsRoutingCapacityLimited(c)
				h.errorResponse(c, http.StatusServiceUnavailable, "multimodal_media_no_eligible_account", "No eligible multimodal media accounts")
				return
			}
			cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, requestModel, routingModel, service.PlatformMultimodal)
			if !cls.ModelNotFound {
				markOpsRoutingCapacityLimited(c)
			}
			h.errorResponse(c, cls.Status, cls.ErrType, cls.Message)
			return
		}
		if failoverClientGone(c) {
			return
		}
		if boundLookupAccountID > 0 && selection.Account.ID != boundLookupAccountID {
			reqLog.Warn("multimodal_media.video_lookup_bound_account_unavailable",
				zap.Int64("bound_account_id", boundLookupAccountID),
				zap.Int64("selected_account_id", selection.Account.ID),
			)
			h.errorResponse(c, http.StatusNotFound, "not_found_error", "Video request not found")
			return
		}

		reqLog.Debug("multimodal_media.account_schedule_decision",
			zap.String("layer", scheduleDecision.Layer),
			zap.Bool("sticky_session_hit", scheduleDecision.StickySessionHit),
			zap.Int("candidate_count", scheduleDecision.CandidateCount),
			zap.Int("top_k", scheduleDecision.TopK),
			zap.Int64("latency_ms", scheduleDecision.LatencyMs),
			zap.Float64("load_skew", scheduleDecision.LoadSkew),
		)

		account := selection.Account
		if failoverClientGone(c) {
			return
		}
		sessionHash = ensureOpenAIPoolModeSessionHash(sessionHash, account)
		setOpsSelectedAccount(c, account.ID, account.Platform)
		c.Request = c.Request.WithContext(service.WithAccountEgressContext(c.Request.Context(), account, h.cfg))

		admissionSessionHash := sessionHash
		if boundLookupAccountID > 0 {
			// Video ownership already has its own TTL; do not replace it with a
			// generic text-session binding during admission.
			admissionSessionHash = ""
		}
		var acquireOutcome accountSlotAcquireOutcome
		accountReleaseFunc, acquireOutcome = h.acquireResponsesAccountSlot(c, apiKey.GroupID, admissionSessionHash, selection, false, &streamStarted, reqLog)
		if acquireOutcome == accountSlotAcquireReschedule {
			addFailedAccountID(&failedAccountIDs, account.ID)
			continue
		}
		if acquireOutcome != accountSlotAcquireReady {
			return
		}

		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		if videoHoldEnabled && videoHold == nil {
			var groupID int64
			if apiKey.GroupID != nil {
				groupID = *apiKey.GroupID
			}
			hold, holdErr := h.gatewayService.ReserveMultimodalVideoBalance(requestCtx, service.VideoBalanceHoldRequest{
				APIKey:     apiKey,
				UserID:     subject.UserID,
				GroupID:    groupID,
				AccountID:  account.ID,
				Model:      requestModel,
				Resolution: requestInfo.Resolution,
				// 与结算阶段使用同一口径的时长（duration=-1 按上限），否则冻结额偏小，
				// 结算时会被封顶导致少扣费。
				DurationSeconds: service.MultimodalVideoBillingDurationSeconds(requestInfo, body),
			})
			if holdErr != nil {
				reqLog.Warn("multimodal_media.reserve_video_balance_failed",
					zap.Int64("account_id", account.ID),
					zap.Error(holdErr),
				)
				status, code, message, retryAfter := billingErrorDetails(holdErr)
				if retryAfter > 0 {
					c.Header("Retry-After", strconv.Itoa(retryAfter))
				}
				h.errorResponse(c, status, code, message)
				return
			}
			videoHold = hold
		}
		forwardStart := time.Now()
		writerSizeBeforeForward := c.Writer.Size()
		result, err := func() (*service.OpenAIForwardResult, error) {
			defer releaseAccount()
			return h.gatewayService.ForwardMultimodalMedia(requestCtx, c, account, endpoint, requestID, body, contentType, videoHold)
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
				if failoverClientGone(c) {
					reqLog.Info("multimodal_media.failover_aborted_client_disconnected",
						zap.Int64("account_id", account.ID),
						zap.Int("upstream_status", failoverErr.StatusCode),
					)
					return
				}
				if failoverErr.ShouldReportAccountScheduleFailure() {
					h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, multimodalMediaScheduleModel(account, routingModel, nil), false, nil)
				}
				if c.Writer.Size() != writerSizeBeforeForward {
					h.handleFailoverExhausted(c, failoverErr, true)
					return
				}
				if !failoverErr.ShouldRetryNextAccount() {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
				}
				if endpoint.IsVideoLookupRequest() {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
				}
				if failoverErr.RetryableOnSameAccount {
					retryLimit := account.GetPoolModeRetryCount()
					if retryCount, retry := tryIncrementSameAccountRetry(&sameAccountRetryCount, account.ID, retryLimit); retry {
						retryDelay := sameAccountRetryDelayFor(failoverErr, retryCount)
						reqLog.Warn("multimodal_media.pool_mode_same_account_retry",
							zap.Int64("account_id", account.ID),
							zap.Int("upstream_status", failoverErr.StatusCode),
							zap.Int("retry_limit", retryLimit),
							zap.Int("retry_count", retryCount),
							zap.Duration("retry_delay", retryDelay),
						)
						if !sleepWithContext(requestCtx, retryDelay) {
							return
						}
						continue
					}
				}
				h.gatewayService.RecordOpenAIAccountSwitch()
				addFailedAccountID(&failedAccountIDs, account.ID)
				lastFailoverErr = failoverErr
				if switchCount >= maxAccountSwitches {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
				}
				switchCount++
				if !failoverErr.IsOpenAIOAuthGatewayRateLimit() && h.gatewayService.ShouldStopOpenAIOAuth429Failover(account, failoverErr.StatusCode, switchCount, &oauth429FailoverState) {
					h.handleFailoverExhausted(c, failoverErr, false)
					return
				}
				reqLog.Warn("multimodal_media.upstream_failover_switching",
					zap.Int64("account_id", account.ID),
					zap.Int("upstream_status", failoverErr.StatusCode),
					zap.Int("switch_count", switchCount),
					zap.Int("max_switches", maxAccountSwitches),
				)
				continue
			}
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, multimodalMediaScheduleModel(account, routingModel, nil), false, nil)
			if !service.IsResponseCommitted(c) && c.Writer.Size() == writerSizeBeforeForward {
				h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
			}
			reqLog.Warn("multimodal_media.forward_failed",
				zap.Int64("account_id", account.ID),
				zap.Error(err),
			)
			return
		}

		// 上游已接受请求：冻结记录已绑定任务 id，后续由轮询结算，这里不再释放。
		videoHoldBound = true
		h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, multimodalMediaScheduleModel(account, routingModel, result), true, nil)
		// 两段式计费路径已在服务内、写响应前完成归属绑定（失败会直接返回错误）。
		// 只有没有冻结记录的路径（订阅制、简单模式）才在这里补写，失败仅记录日志。
		if videoHold == nil && endpoint.IsGenerationRequest() && strings.TrimSpace(result.ResponseID) != "" {
			if err := h.gatewayService.BindMultimodalMediaVideoRequestAccount(
				requestCtx, apiKey.GroupID, result.ResponseID, subject.UserID, apiKey.ID, account.ID,
			); err != nil {
				reqLog.Warn("multimodal_media.bind_video_request_account_failed",
					zap.Int64("account_id", account.ID),
					zap.String("request_id", result.ResponseID),
					zap.Error(err),
				)
			}
		}
		// 视频生成走两段式计费：创建阶段只冻结、不写用量，结算在轮询完成时按
		// 实际秒数落唯一一条 usage_log，避免同一请求被计费两次、失败仍然扣费。
		if shouldRecordMultimodalMediaUsage(endpoint, requestModel) && !videoHoldEnabled {
			recordMultimodalMediaUsage(c, h, reqLog, apiKey, subject, subscription, account, result, requestModel, body, requestID)
		}
		reqLog.Debug("multimodal_media.request_completed",
			zap.Int64("account_id", account.ID),
			zap.Int("switch_count", switchCount),
		)
		return
	}
}

func multimodalMediaRequiredCapability(endpoint service.MultimodalMediaEndpoint) service.OpenAIEndpointCapability {
	if endpoint.IsGenerationRequest() {
		return service.OpenAIEndpointCapabilityMultimodalMediaGeneration
	}
	return ""
}

func multimodalMediaScheduleModel(account *service.Account, routingModel string, result *service.OpenAIForwardResult) string {
	if result != nil && strings.TrimSpace(result.UpstreamModel) != "" {
		return result.UpstreamModel
	}
	if account == nil {
		return strings.TrimSpace(routingModel)
	}
	return account.GetMappedModel(routingModel)
}

func shouldRecordMultimodalMediaUsage(endpoint service.MultimodalMediaEndpoint, requestModel string) bool {
	return endpoint.IsGenerationRequest() && strings.TrimSpace(requestModel) != ""
}

func recordMultimodalMediaUsage(
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
	requestID string,
) {
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	sessionID := service.ExtractClientSessionID(c)
	payloadForHash := body
	if len(payloadForHash) == 0 && strings.TrimSpace(requestID) != "" {
		payloadForHash = []byte(requestID)
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
				zap.String("component", "handler.openai_gateway.multimodal_media"),
				zap.Int64("user_id", subject.UserID),
				zap.Int64("api_key_id", apiKey.ID),
				zap.Any("group_id", apiKey.GroupID),
				zap.String("model", requestModel),
				zap.Int64("account_id", account.ID),
			).Error("multimodal_media.record_usage_failed", zap.Error(err))
			reqLog.Debug("multimodal_media.record_usage_failed", zap.Error(err))
		}
	})
}

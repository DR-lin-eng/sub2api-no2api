package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"

	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

type openAIWSTurnPricing struct {
	mu sync.Mutex
	at time.Time
}

func (p *openAIWSTurnPricing) freeze(at time.Time) {
	p.mu.Lock()
	p.at = at
	p.mu.Unlock()
}

func (p *openAIWSTurnPricing) current() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.at
}

func openAIWSConcurrencyCloseStatus(err error) coderws.StatusCode {
	status, _, _ := concurrencyErrorResponse(err, "request")
	if status == http.StatusTooManyRequests {
		return coderws.StatusTryAgainLater
	}
	return coderws.StatusInternalError
}

// ResponsesWebSocket handles OpenAI Responses API WebSocket ingress endpoint
// GET /openai/v1/responses (Upgrade: websocket)
func (h *OpenAIGatewayHandler) ResponsesWebSocket(c *gin.Context) {
	if !isOpenAIWSUpgradeRequest(c.Request) {
		c.Header("Upgrade", "websocket")
		c.Header("X-OpenAI-Responses-SSE-Fallback", "supported")
		h.errorResponse(c, http.StatusUpgradeRequired, "invalid_request_error", "WebSocket upgrade required (Upgrade: websocket); retry with POST /v1/responses for HTTP SSE")
		return
	}
	setOpenAIClientTransportWS(c)

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
		"handler.openai_gateway.responses_ws",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
		zap.Bool("openai_ws_mode", true),
	)
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}
	reqLog.Info("openai.websocket_ingress_started")
	clientIP := ip.GetClientIP(c)
	userAgent := strings.TrimSpace(c.GetHeader("User-Agent"))
	clientLifecycleCtx := c.Request.Context()
	ctx := clientLifecycleCtx
	maxIngressConnections := 0
	if h.cfg != nil {
		maxIngressConnections = h.cfg.Gateway.OpenAIWS.MaxIngressConnectionsPerAPIKey
	}
	ingressLease, ingressLeaseAcquired, ingressLeaseErr := h.concurrencyHelper.AcquireOpenAIWSIngressLease(ctx, apiKey.ID, maxIngressConnections)
	if ingressLeaseErr != nil {
		reqLog.Error("openai.websocket_ingress_lease_acquire_failed", zap.Error(ingressLeaseErr))
		h.errorResponse(c, http.StatusServiceUnavailable, "service_unavailable", "WebSocket ingress capacity is temporarily unavailable")
		return
	}
	if !ingressLeaseAcquired {
		reqLog.Info("openai.websocket_ingress_capacity_rejected", zap.Int("max_ingress_connections_per_api_key", maxIngressConnections))
		c.Header("Retry-After", "5")
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Too many open WebSocket connections, please retry later")
		return
	}
	if ingressLease != nil {
		defer ingressLease.Release()
		ctx = ingressLease.Context()
		c.Request = c.Request.WithContext(ctx)
	}

	wsConn, err := coderws.Accept(c.Writer, c.Request, &coderws.AcceptOptions{
		CompressionMode: coderws.CompressionContextTakeover,
	})
	if err != nil {
		reqLog.Warn("openai.websocket_accept_failed",
			zap.Error(err),
			zap.String("client_ip", clientIP),
			zap.String("request_user_agent", userAgent),
			zap.String("upgrade_header", strings.TrimSpace(c.GetHeader("Upgrade"))),
			zap.String("connection_header", strings.TrimSpace(c.GetHeader("Connection"))),
			zap.String("sec_websocket_version", strings.TrimSpace(c.GetHeader("Sec-WebSocket-Version"))),
			zap.Bool("has_sec_websocket_key", strings.TrimSpace(c.GetHeader("Sec-WebSocket-Key")) != ""),
		)
		return
	}
	defer func() {
		_ = wsConn.CloseNow()
	}()
	wsConn.SetReadLimit(service.ResolveOpenAIWSClientReadLimitBytes(h.cfg))

	firstMessageTimeout := service.ResolveOpenAIWSClientFirstMessageTimeout(h.cfg)
	msgType, firstMessage, err := service.ReadOpenAIWSClientMessage(
		ctx,
		wsConn,
		firstMessageTimeout,
		coderws.StatusPolicyViolation,
		"missing first response.create message",
	)
	if err != nil {
		if errors.Is(context.Cause(ctx), service.ErrOpenAIWSIngressLeaseLost) {
			reqLog.Warn("openai.websocket_ingress_lease_lost_before_first_message", zap.Error(err))
			closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "websocket ingress capacity lease lost; please reconnect")
			return
		}
		closeStatus, closeReason := summarizeWSCloseErrorForLog(err)
		reqLog.Warn("openai.websocket_read_first_message_failed",
			zap.Error(err),
			zap.String("client_ip", clientIP),
			zap.String("close_status", closeStatus),
			zap.String("close_reason", closeReason),
			zap.Duration("read_timeout", firstMessageTimeout),
		)
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "missing first response.create message")
		return
	}
	if msgType != coderws.MessageText && msgType != coderws.MessageBinary {
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "unsupported websocket message type")
		return
	}
	if !gjson.ValidBytes(firstMessage) {
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "invalid JSON payload")
		return
	}
	h.concurrencyHelper.SetPriorityAdmissionPendingBytes(c, int64(len(firstMessage)))
	reqModel := strings.TrimSpace(gjson.GetBytes(firstMessage, "model").String())
	if reqModel == "" {
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "model is required in first response.create payload")
		return
	}
	ensureCompositeTargetPlatform(c, apiKey, reqModel)
	ctx = c.Request.Context()
	if apiKey.Group != nil && apiKey.Group.Platform == service.PlatformComposite {
		platform, ok := service.ResolvedTargetPlatformFromContext(ctx)
		if !ok || (platform != service.PlatformOpenAI && platform != service.PlatformGrok) {
			closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "Responses WebSocket API only supports OpenAI-compatible models for composite groups")
			return
		}
	}
	previousResponseID := strings.TrimSpace(gjson.GetBytes(firstMessage, "previous_response_id").String())
	previousResponseIDKind := service.ClassifyOpenAIPreviousResponseIDKind(previousResponseID)
	if previousResponseID != "" && previousResponseIDKind == service.OpenAIPreviousResponseIDKindMessageID {
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "previous_response_id must be a response.id (resp_*), not a message id")
		return
	}
	firstMessageToolCoverage := service.AnalyzeToolCallOutputContextCoverageBytes(firstMessage)
	previousResponseCanMove := !firstMessageToolCoverage.HasFunctionCallOutput || firstMessageToolCoverage.ContextCoversAllCallIDs
	reqLog = reqLog.With(
		zap.Bool("ws_ingress", true),
		zap.String("session_initial_model", reqModel),
		zap.Bool("has_previous_response_id", previousResponseID != ""),
		zap.String("previous_response_id_kind", previousResponseIDKind),
	)
	setOpsRequestContext(c, reqModel, true)
	setOpsEndpointContext(c, "", int16(service.RequestTypeWSV2))

	if decision := h.checkSecurityAuditStage(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIResponses, reqModel, firstMessage, "first_turn"); decision != nil && !decision.AllowNextStage {
		writeSecurityAuditWSError(ctx, wsConn, decision)
		closeOpenAIClientWS(wsConn, securityAuditWSCloseStatus(decision), securityAuditWSCloseReason(decision))
		return
	}

	forceImageTool := groupForcesOpenAIImageTool(c, apiKey)
	imageIntent := forceImageTool || service.IsExplicitImageGenerationIntent("/v1/responses", reqModel, firstMessage)
	if imageIntent && !service.GroupAllowsImageGeneration(apiKey.Group) {
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, service.ImageGenerationPermissionMessage())
		return
	}

	// F5a: 握手层会话屏蔽检查。WS 握手无 body，显式标识仅来自握手 header
	// （session_id / conversation_id）；无标识则放行，连接内仍有本地 flag 兜底。
	cyberBlockKey := service.CyberSessionBlockKey(apiKey.ID, c, nil)
	if cyberBlockKey != "" && h.gatewayService.IsCyberSessionBlocked(c.Request.Context(), cyberBlockKey) {
		writeCyberSessionBlockedWSError(c.Request.Context(), wsConn)
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "session blocked by cyber-security policy")
		h.enqueueCyberSessionBlockedOpsEntry(c, apiKey, reqModel, cyberBlockKey)
		return
	}
	cyberBlockedThisConn := false

	// 解析渠道级模型映射
	channelMappingWS, _ := h.gatewayService.ResolveChannelMappingAndRestrict(ctx, apiKey.GroupID, reqModel)

	var currentUserRelease func()
	var currentAccountRelease func()
	acquireUserTurnSlot := func() (func(), bool, error) {
		turnCtx := c.Request.Context()
		if !h.concurrencyHelper.concurrencyService.PriorityAdmissionEnabledForRequest(turnCtx) {
			return h.concurrencyHelper.TryAcquireUserSlotForAPIKey(turnCtx, subject.UserID, subject.Concurrency, apiKey.ID, apiKey.ConcurrencyLimit)
		}
		streamStarted := false
		release, err := h.concurrencyHelper.AcquireUserSlotWithWait(c, subject.UserID, subject.Concurrency, false, &streamStarted)
		if err != nil {
			return nil, false, err
		}
		return release, true, nil
	}
	releaseAccountSlot := func() {
		if currentAccountRelease != nil {
			currentAccountRelease()
			currentAccountRelease = nil
		}
	}
	releaseTurnSlots := func() {
		releaseAccountSlot()
		if currentUserRelease != nil {
			currentUserRelease()
			currentUserRelease = nil
		}
	}
	// 必须尽早注册，确保任何 early return 都能释放已获取的并发槽位。
	defer releaseTurnSlots()

	userReleaseFunc, userAcquired, err := acquireUserTurnSlot()
	if err != nil {
		if shouldLogConcurrencyAcquireError(err) {
			reqLog.Warn("openai.websocket_user_slot_acquire_failed", zap.Error(err))
		}
		closeOpenAIClientWS(wsConn, openAIWSConcurrencyCloseStatus(err), "failed to acquire user concurrency slot")
		return
	}
	if !userAcquired {
		closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "too many concurrent requests, please retry later")
		return
	}
	currentUserRelease = wrapReleaseOnDone(ctx, userReleaseFunc)
	ensureUserSlotHeld := func() bool {
		if currentUserRelease != nil {
			return true
		}
		userReleaseFunc, userAcquired, err := acquireUserTurnSlot()
		if err != nil {
			if shouldLogConcurrencyAcquireError(err) {
				reqLog.Warn("openai.websocket_user_slot_reacquire_failed", zap.Error(err))
			}
			closeOpenAIClientWS(wsConn, openAIWSConcurrencyCloseStatus(err), "failed to acquire user concurrency slot")
			return false
		}
		if !userAcquired {
			closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "too many concurrent requests, please retry later")
			return false
		}
		currentUserRelease = wrapReleaseOnDone(ctx, userReleaseFunc)
		return true
	}

	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	requestPlatform := openAICompatibleRequestPlatform(ctx, apiKey)
	requiredTransport := service.OpenAIUpstreamTransportResponsesWebsocketV2Ingress
	if requestPlatform == service.PlatformGrok {
		requiredTransport = service.OpenAIUpstreamTransportHTTPSSE
	}
	if err := h.billingCacheService.CheckBillingEligibility(ctx, apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		reqLog.Info("openai.websocket_billing_eligibility_check_failed", zap.Error(err))
		closeOpenAIClientWS(wsConn, coderws.StatusPolicyViolation, "billing check failed")
		return
	}

	sessionHash := h.gatewayService.GenerateSessionHashWithFallbackForRequest(
		c,
		apiKey.GroupID,
		firstMessage,
		openAIWSIngressFallbackSessionSeed(subject.UserID, apiKey.ID, apiKey.GroupID),
	)
	defer h.gatewayService.ReleaseOpenAIContentSessionRequest(c.Request.Context(), apiKey.GroupID, sessionHash)
	ctx = c.Request.Context()
	if forceImageTool {
		h.handleForcedOpenAIImageResponsesWebSocket(
			service.WithOpenAIStreamScheduling(ctx, true),
			c,
			wsConn,
			firstMessage,
			reqModel,
			sessionHash,
			apiKey,
			subject,
			subscription,
			reqLog,
			releaseTurnSlots,
			ensureUserSlotHeld,
		)
		return
	}
	maxAccountSwitches := h.maxAccountSwitches
	switchCount := 0
	var failedAccountIDs map[int64]struct{}
	profitVetoCount := 0
	var lastFailoverErr *service.UpstreamFailoverError
	var oauth429FailoverState service.OpenAIOAuth429FailoverState
	wsAttemptMessage := append([]byte(nil), firstMessage...)
	handleWSFailover := func(account *service.Account, failoverErr *service.UpstreamFailoverError) bool {
		if ctx.Err() != nil {
			return false
		}
		if failoverErr.ShouldReportAccountScheduleFailure() {
			h.gatewayService.ReportOpenAIAccountStreamScheduleResult(account.ID, account.GetMappedModel(reqModel), false, nil, true)
		}
		releaseAccountSlot()
		if !failoverErr.ShouldRetryNextAccount() {
			closeOpenAIWSFailoverExhausted(wsConn, failoverErr)
			return false
		}
		if ctx.Err() != nil {
			return false
		}
		if failoverErr.Scope == service.GatewayFailureScopeRequest && sessionHash != "" {
			ctx = h.gatewayService.PreserveOpenAIStickyBindingForFailover(ctx, apiKey.GroupID, sessionHash, account.ID)
		}
		h.gatewayService.RecordOpenAIAccountSwitch()
		addFailedAccountID(&failedAccountIDs, account.ID)
		lastFailoverErr = failoverErr
		if switchCount >= maxAccountSwitches {
			closeOpenAIWSFailoverExhausted(wsConn, failoverErr)
			return false
		}
		switchCount++
		if h.gatewayService.ShouldStopOpenAIOAuth429Failover(account, failoverErr.StatusCode, switchCount, &oauth429FailoverState) {
			closeOpenAIWSFailoverExhausted(wsConn, failoverErr)
			return false
		}
		reqLog.Warn("openai.websocket_upstream_failover_switching",
			zap.Int64("account_id", account.ID),
			zap.Int("upstream_status", failoverErr.StatusCode),
			zap.Int("switch_count", switchCount),
			zap.Int("max_switches", maxAccountSwitches),
		)
		if ctx.Err() != nil {
			return false
		}
		return ensureUserSlotHeld()
	}

	// 与 HTTP Responses 路径保持一致：生图意图请求要求账号支持 Responses API（#4417）。
	// WSv2 传输本身已隐含 Responses 支持，此处为防御性对齐。
	// 使用 IsExplicitImageGenerationIntent 排除被动 namespace 声明（#4476）。
	requiredCapability := service.OpenAIEndpointCapabilityChatCompletions
	if imageIntent && requestPlatform == service.PlatformOpenAI {
		requiredCapability = service.OpenAIEndpointCapabilityResponses
	}
	wsPricingCtx, _ := h.gatewayService.WithOpenAIRequestPricingContext(ctx, apiKey.GroupID)
	ctx = wsPricingCtx

	for {
		if ctx.Err() != nil {
			return
		}
		reqLog.Debug("openai.websocket_account_selecting", zap.Int("excluded_account_count", len(failedAccountIDs)))
		selection, scheduleDecision, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
			ctx,
			apiKey.GroupID,
			previousResponseID,
			sessionHash,
			reqModel,
			failedAccountIDs,
			requiredTransport,
			requiredCapability,
			false,
			previousResponseCanMove,
			!imageIntent,
			requestPlatform,
		)
		if err != nil {
			reqLog.Warn("openai.websocket_account_select_failed",
				zap.Error(openAICompatibleSelectionErrorForLog(err, requestPlatform)),
				zap.Int("excluded_account_count", len(failedAccountIDs)),
			)
			if errors.Is(err, service.ErrPriorityAdmissionUnavailable) {
				closeOpenAIClientWS(wsConn, coderws.StatusInternalError, "service temporarily unavailable")
				return
			}
			if lastFailoverErr != nil {
				closeOpenAIWSFailoverExhausted(wsConn, lastFailoverErr)
			} else {
				closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "no available account")
			}
			return
		}
		if selection == nil || selection.Account == nil {
			if lastFailoverErr != nil {
				closeOpenAIWSFailoverExhausted(wsConn, lastFailoverErr)
			} else {
				closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "no available account")
			}
			return
		}

		account := selection.Account
		accountMaxConcurrency := account.Concurrency
		accountMaxWaiting := 0
		accountWaitTimeout := time.Duration(0)
		if selection.WaitPlan != nil && selection.WaitPlan.MaxConcurrency > 0 {
			accountMaxConcurrency = selection.WaitPlan.MaxConcurrency
			accountMaxWaiting = selection.WaitPlan.MaxWaiting
			accountWaitTimeout = selection.WaitPlan.Timeout
		} else if h.cfg != nil {
			accountMaxWaiting = h.cfg.Gateway.Scheduling.StickySessionMaxWaiting
			accountWaitTimeout = h.cfg.Gateway.Scheduling.StickySessionWaitTimeout
		}
		admissionCtx := service.ContextWithSelectionProfitGate(ctx, selection)
		accountReleaseFunc := selection.ReleaseFunc
		if !selection.Acquired {
			if selection.WaitPlan == nil {
				closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "account is busy, please retry later")
				return
			}
			var fastReleaseFunc func()
			var fastAcquired bool
			if h.concurrencyHelper.concurrencyService.PriorityAdmissionEnabledForRequest(c.Request.Context()) {
				streamStarted := false
				fastReleaseFunc, err = h.concurrencyHelper.AcquireAccountSlotWithPriorityWaitTimeout(
					c,
					account.ID,
					selection.WaitPlan.MaxConcurrency,
					selection.WaitPlan.MaxWaiting,
					selection.WaitPlan.Timeout,
					int64(len(firstMessage)),
					false,
					&streamStarted,
				)
				fastAcquired = err == nil
			} else {
				fastReleaseFunc, fastAcquired, err = h.concurrencyHelper.TryAcquireAccountSlot(
					ctx,
					account.ID,
					selection.WaitPlan.MaxConcurrency,
				)
			}
			if err != nil {
				if shouldLogConcurrencyAcquireError(err) {
					reqLog.Warn("openai.websocket_account_slot_acquire_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				}
				closeOpenAIClientWS(wsConn, openAIWSConcurrencyCloseStatus(err), "failed to acquire account concurrency slot")
				return
			}
			if !fastAcquired {
				closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "account is busy, please retry later")
				return
			}
			accountReleaseFunc = fastReleaseFunc
			if err := h.gatewayService.EnsureAccountSchedulableAfterWait(ctx, apiKey.GroupID, sessionHash, account.ID); err != nil {
				if accountReleaseFunc != nil {
					accountReleaseFunc()
				}
				if errors.Is(err, service.ErrAccountSchedulingChanged) {
					reqLog.Info("openai.websocket_account_reschedule_after_wait", zap.Int64("account_id", account.ID))
					addFailedAccountID(&failedAccountIDs, account.ID)
					continue
				}
				reqLog.Warn("openai.websocket_account_recheck_after_wait_failed", zap.Int64("account_id", account.ID), zap.Error(err))
				closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "account scheduling state is unavailable")
				return
			}
		}
		latest, vetoed, reason := h.gatewayService.ProfitControlVetoLatest(admissionCtx, account)
		if vetoed {
			if accountReleaseFunc != nil {
				accountReleaseFunc()
			}
			reqLog.Debug("openai.websocket_account_slot_profit_vetoed", zap.Int64("account_id", account.ID), zap.String("reason", reason))
			if !recordOpenAIProfitVeto(&failedAccountIDs, account.ID, &profitVetoCount) {
				reqLog.Warn("openai.websocket_profit_veto_attempts_exhausted", zap.Int("profit_veto_count", profitVetoCount))
				closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "no available account")
				return
			}
			continue
		}
		account = latest
		selection.Account = latest
		ctx = admissionCtx
		currentAccountRelease = wrapReleaseOnDone(ctx, accountReleaseFunc)
		if err := h.gatewayService.BindStickySessionAfterProfitAdmission(ctx, apiKey.GroupID, sessionHash, account.ID); err != nil {
			reqLog.Warn("openai.websocket_bind_sticky_session_after_profit_admission_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		}

		token, _, err := h.gatewayService.GetRequestCredential(ctx, c, account)
		if err != nil {
			reqLog.Warn("openai.websocket_get_access_token_failed", zap.Int64("account_id", account.ID), zap.Error(err))
			if ctx.Err() != nil {
				return
			}
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				if handleWSFailover(account, failoverErr) {
					continue
				}
				return
			}
			closeOpenAIClientWS(wsConn, coderws.StatusInternalError, "failed to get access token")
			return
		}

		reqLog.Debug("openai.websocket_account_selected",
			zap.Int64("account_id", account.ID),
			zap.String("account_name", account.Name),
			zap.String("schedule_layer", scheduleDecision.Layer),
			zap.Int("candidate_count", scheduleDecision.CandidateCount),
		)

		maxReasoningEffort, reasoningEffortMappings, _ := openAIReasoningEffortPolicyForRequest(c, apiKey)
		var requestPayloadHash string
		var turnChannelMapping openAIWSTurnChannelMappingState
		turnChannelMapping.Store(1, reqModel, channelMappingWS)
		var turnPricing openAIWSTurnPricing
		hooks := &service.OpenAIWSIngressHooks{
			ClientLifecycleContext:  clientLifecycleCtx,
			InitialRequestModel:     reqModel,
			MaxReasoningEffort:      maxReasoningEffort,
			ReasoningEffortMappings: reasoningEffortMappings,
			BeforeRequest: func(turn int, payload []byte, originalModel string) error {
				c.Set(securityAuditWSTurnContextKey, turn)
				if turn > 1 {
					h.concurrencyHelper.RefreshPriorityAdmissionRequestSnapshot(c)
				}
				h.concurrencyHelper.SetPriorityAdmissionPendingBytes(c, int64(len(payload)))
				if turn == 1 {
					return nil
				}
				if !gjson.ValidBytes(payload) {
					return service.NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "invalid websocket request payload", errors.New("invalid json"))
				}
				model := strings.TrimSpace(originalModel)
				if model == "" {
					model = strings.TrimSpace(gjson.GetBytes(payload, "model").String())
				}
				if model == "" {
					model = reqModel
				}
				if decision := h.checkSecurityAuditStage(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIResponses, model, payload, "subsequent_turn"); decision != nil && !decision.AllowNextStage {
					writeSecurityAuditWSError(ctx, wsConn, decision)
					return service.NewOpenAIWSClientCloseError(securityAuditWSCloseStatus(decision), securityAuditWSCloseReason(decision), nil)
				}
				return nil
			},
			MapRequestModel: func(turn int, originalModel string) (string, error) {
				model := strings.TrimSpace(originalModel)
				if model == "" {
					model = reqModel
				}
				mapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(ctx, apiKey.GroupID, model)
				mappedModelUnchanged := false
				if previous, ok := turnChannelMapping.Latest(); ok && previous.turn < turn {
					mappedModelUnchanged = strings.TrimSpace(previous.mapping.MappedModel) == strings.TrimSpace(mapping.MappedModel)
				}
				if turn > 1 && !mappedModelUnchanged && !account.IsModelSupported(model) && !account.IsModelSupported(mapping.MappedModel) {
					return "", newOpenAIWSUnsupportedModelSwitchError(mapping.MappedModel)
				}
				turnChannelMapping.Store(turn, model, mapping)
				return mapping.MappedModel, nil
			},
			BeforeTurn: func(turn int) error {
				// turn==1 的会话屏蔽已由握手层检查覆盖；连接内 flag 只拦截后续 turn。
				if cyberBlockedThisConn {
					return service.NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, cyberSessionBlockedClientMsg, nil)
				}
				turnCtx, turnAt := h.gatewayService.WithOpenAITurnPricingContext(ctx, apiKey.GroupID)
				if _, vetoed, reason := h.gatewayService.ProfitControlVetoLatest(turnCtx, account); vetoed {
					reqLog.Info("openai.websocket_turn_profit_vetoed", zap.Int("turn", turn), zap.Int64("account_id", account.ID), zap.String("reason", reason))
					return service.NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "account is no longer eligible for this connection, please reconnect", nil)
				}
				turnPricing.freeze(turnAt)
				if turn == 1 {
					return nil
				}
				// 防御式清理：避免异常路径下旧槽位覆盖导致泄漏。
				releaseTurnSlots()
				// 非首轮 turn 需要重新抢占并发槽位，避免长连接空闲占槽。
				userReleaseFunc, userAcquired, err := acquireUserTurnSlot()
				if err != nil {
					return service.NewOpenAIWSClientCloseError(openAIWSConcurrencyCloseStatus(err), "failed to acquire user concurrency slot", err)
				}
				if !userAcquired {
					return service.NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "too many concurrent requests, please retry later", nil)
				}
				var accountReleaseFunc func()
				var accountAcquired bool
				if h.concurrencyHelper.concurrencyService.PriorityAdmissionEnabledForRequest(c.Request.Context()) {
					streamStarted := false
					accountReleaseFunc, err = h.concurrencyHelper.AcquireAccountSlotWithPriorityWaitTimeout(
						c,
						account.ID,
						accountMaxConcurrency,
						accountMaxWaiting,
						accountWaitTimeout,
						priorityAdmissionPendingBytes(c),
						false,
						&streamStarted,
					)
					accountAcquired = err == nil
				} else {
					accountReleaseFunc, accountAcquired, err = h.concurrencyHelper.TryAcquireAccountSlot(c.Request.Context(), account.ID, accountMaxConcurrency)
				}
				if err != nil {
					if userReleaseFunc != nil {
						userReleaseFunc()
					}
					return service.NewOpenAIWSClientCloseError(openAIWSConcurrencyCloseStatus(err), "failed to acquire account concurrency slot", err)
				}
				if !accountAcquired {
					if userReleaseFunc != nil {
						userReleaseFunc()
					}
					return service.NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "account is busy, please retry later", nil)
				}
				if err := h.gatewayService.EnsureAccountSchedulableAfterWait(c.Request.Context(), apiKey.GroupID, sessionHash, account.ID); err != nil {
					if accountReleaseFunc != nil {
						accountReleaseFunc()
					}
					if userReleaseFunc != nil {
						userReleaseFunc()
					}
					return service.NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "account scheduling changed; reconnect to reschedule", err)
				}
				if _, vetoed, reason := h.gatewayService.ProfitControlVetoLatest(turnCtx, account); vetoed {
					if accountReleaseFunc != nil {
						accountReleaseFunc()
					}
					if userReleaseFunc != nil {
						userReleaseFunc()
					}
					reqLog.Info("openai.websocket_turn_post_slot_profit_vetoed", zap.Int("turn", turn), zap.Int64("account_id", account.ID), zap.String("reason", reason))
					return service.NewOpenAIWSClientCloseError(coderws.StatusTryAgainLater, "account is no longer eligible for this connection, please reconnect", nil)
				}
				currentUserRelease = wrapReleaseOnDone(turnCtx, userReleaseFunc)
				currentAccountRelease = wrapReleaseOnDone(turnCtx, accountReleaseFunc)
				return nil
			},
			AfterTurn: func(turn int, result *service.OpenAIForwardResult, turnErr error) {
				// F1: cyber 标记按 turn 生命周期清理——defer 保证任意早返回路径都执行；
				// CyberBlocked 必须在 submit 前同步预捕获（task 闭包由 worker 池异步执行，
				// 届时 defer 已清除标记）。
				defer clearCyberPolicyTurnState(c)
				releaseTurnSlots()
				turnRequestedModel := reqModel
				turnMapping := channelMappingWS
				if snapshot, ok := turnChannelMapping.Load(turn); ok {
					if snapshot.requestedModel != "" {
						turnRequestedModel = snapshot.requestedModel
					}
					turnMapping = snapshot.mapping
				}
				turnUpstreamModel := ""
				if result != nil {
					turnUpstreamModel = strings.TrimSpace(result.UpstreamModel)
				}
				if turnUpstreamModel == "" {
					mappedModel := turnMapping.MappedModel
					if strings.TrimSpace(mappedModel) == "" {
						mappedModel = turnRequestedModel
					}
					turnUpstreamModel = account.GetMappedModel(mappedModel)
				}
				turnUsageFields := turnMapping.ToUsageFields(turnRequestedModel, turnUpstreamModel)
				h.recordCyberPolicyIfMarked(c, apiKey, account, subscription, turnRequestedModel, turnErr != nil, cyberBlockKey, turnUsageFields, requestPayloadHash)
				if service.GetOpsCyberPolicy(c) != nil {
					cyberBlockedThisConn = true
				}
				if turnErr != nil {
					if result == nil || result.ImageCount <= 0 {
						return
					}
					// cyber 命中时该 turn 的用量已由 recordCyberPolicyIfMarked(forwardErrored=true)
					// 按真实 token 记录，这里不再走下方 RecordUsage，避免对同一 turn 双写/双扣费。
					if service.GetOpsCyberPolicy(c) != nil {
						return
					}
					reqLog.Warn("openai.websocket_partial_error_with_image_result",
						zap.Int64("account_id", account.ID),
						zap.Int("image_count", result.ImageCount),
						zap.Error(turnErr),
					)
				}
				if result == nil {
					return
				}
				result.BillingModel = openAIWSTurnBillingModel(result, turnMapping, turnRequestedModel, turnUpstreamModel)
				reqLog.Debug("openai.websocket_turn_billing",
					zap.Int("turn", turn),
					zap.String("turn_requested_model", turnRequestedModel),
					zap.String("turn_upstream_model", turnUpstreamModel),
					zap.String("billing_model", result.BillingModel),
				)
				// 排除 spark 影子:其 codex_* 仅由 QueryUsage(/wham/usage bengalfox)更新(外审第7轮 P1)。
				if account.Type == service.AccountTypeOAuth && !account.IsShadow() {
					h.gatewayService.UpdateCodexUsageSnapshotFromHeaders(ctx, account.ID, result.ResponseHeaders)
				}
				h.gatewayService.ReportOpenAIAccountStreamScheduleResult(account.ID, turnUpstreamModel, openAIForwardSucceededForScheduling(result), openAIFirstTokenForTTFT(result, imageIntent), true)
				inboundEndpoint := GetInboundEndpoint(c)
				upstreamEndpoint := resolveOpenAIUpstreamEndpoint(c, account, result)
				quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
				sessionID := service.ExtractClientSessionID(c)
				turnRecordPricingAt := turnPricing.current()
				cyberBlocked := service.GetOpsCyberPolicy(c) != nil
				h.submitOpenAIUsageRecordTask(ctx, result, func(taskCtx context.Context) {
					if err := h.gatewayService.RecordUsage(taskCtx, &service.OpenAIRecordUsageInput{
						Result:             result,
						APIKey:             apiKey,
						User:               apiKey.User,
						Account:            account,
						Subscription:       subscription,
						InboundEndpoint:    inboundEndpoint,
						UpstreamEndpoint:   upstreamEndpoint,
						UserAgent:          userAgent,
						IPAddress:          clientIP,
						SessionID:          sessionID,
						RequestPayloadHash: requestPayloadHash,
						APIKeyService:      h.apiKeyService,
						QuotaPlatform:      quotaPlatform,
						PricingAt:          turnRecordPricingAt,
						ChannelUsageFields: turnUsageFields,
						CyberBlocked:       cyberBlocked,
					}); err != nil {
						reqLog.Error("openai.websocket_record_usage_failed",
							zap.Int64("account_id", account.ID),
							zap.String("request_id", result.RequestID),
							zap.Error(err),
						)
					}
				})
			},
		}

		wsFirstMessage := wsAttemptMessage
		// 切组/会话失配防护：previous_response_id 未在当前分组命中粘连账号（StickyPreviousHit=false），
		// 说明该会话链不属于本次调度到的账号，原样转发会触发上游会话链鉴权失败（“鉴权失败，请检查 API Key”）。
		// 故剥离首包里的 previous_response_id，改用首包内 input 重建上下文；带 function_call_output 的
		// 工具续链无法重建，保持原样。仅作用于首轮首包，后续 turn 的续链由 WS 转发层既有逻辑处理。
		if previousResponseID != "" && !scheduleDecision.StickyPreviousHit && previousResponseCanMove {
			wsFirstMessage = service.RemovePreviousResponseIDFromBody(wsFirstMessage)
			wsAttemptMessage = append([]byte(nil), wsFirstMessage...)
			reqLog.Debug("openai.websocket_previous_response_id_stripped_cross_group",
				zap.Int64("account_id", account.ID),
				zap.String("schedule_layer", scheduleDecision.Layer),
			)
		}

		// WebSocket 首包可能很大，hash 必须在 hooks 外算成字符串，避免 AfterTurn 闭包保活请求体。
		requestPayloadHash = service.HashUsageRequestPayload(wsFirstMessage)

		if err := h.gatewayService.ProxyResponsesWebSocketFromClient(ctx, c, wsConn, account, token, wsFirstMessage, hooks); err != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(err, &failoverErr) {
				retryPayload, retryCurrentTurn := service.OpenAIWSCurrentTurnRetryPayload(err)
				nextAttemptMessage, retrySafe := openAIWSNextAttemptMessage(wsAttemptMessage, retryPayload, retryCurrentTurn)
				if !retrySafe {
					closeOpenAIWSFailoverExhausted(wsConn, failoverErr)
					return
				}
				wsAttemptMessage = nextAttemptMessage
				if retryCurrentTurn {
					previousResponseID = ""
					reqLog.Warn("openai.websocket_current_turn_failover_retry",
						zap.Int64("account_id", account.ID),
						zap.Int("upstream_status", failoverErr.StatusCode),
						zap.Int("retry_payload_bytes", len(retryPayload)),
					)
				}
				if handleWSFailover(account, failoverErr) {
					continue
				}
				return
			}

			if errors.Is(context.Cause(ctx), service.ErrOpenAIWSIngressLeaseLost) {
				reqLog.Warn("openai.websocket_ingress_lease_lost",
					zap.Int64("account_id", account.ID),
					zap.Error(err),
				)
				closeOpenAIClientWS(wsConn, coderws.StatusTryAgainLater, "websocket ingress capacity lease lost; please reconnect")
				return
			}

			var closeErr *service.OpenAIWSClientCloseError
			hasClientCloseErr := errors.As(err, &closeErr)
			if openAIWSIngressEndedByClient(err) {
				fields := []zap.Field{zap.Int64("account_id", account.ID)}
				if hasClientCloseErr {
					fields = append(fields, zap.String("reason", closeErr.Reason()))
					closeOpenAIClientWS(wsConn, closeErr.StatusCode(), closeErr.Reason())
				} else {
					fields = append(fields, zap.Error(err))
					closeOpenAIClientWS(wsConn, coderws.StatusNormalClosure, "")
				}
				reqLog.Info("openai.websocket_ingress_closed_normally", fields...)
				return
			}

			if shouldReportOpenAIWSProxyAccountFailure(err) {
				h.gatewayService.ReportOpenAIAccountStreamScheduleResult(account.ID, account.GetMappedModel(reqModel), false, nil, true)
			}
			closeStatus, closeReason := summarizeWSCloseErrorForLog(err)
			proxyFailedFields := []zap.Field{
				zap.Int64("account_id", account.ID),
				zap.Error(err),
				zap.String("close_status", closeStatus),
				zap.String("close_reason", closeReason),
			}
			if account.Proxy != nil {
				proxyFailedFields = append(proxyFailedFields,
					zap.Int64("proxy_id", account.Proxy.ID),
					zap.String("proxy_name", account.Proxy.Name),
					zap.String("proxy_host", account.Proxy.Host),
					zap.Int("proxy_port", account.Proxy.Port),
				)
			} else if account.ProxyID != nil {
				proxyFailedFields = append(proxyFailedFields, zap.Int64p("proxy_id", account.ProxyID))
			}
			reqLog.Warn("openai.websocket_proxy_failed", proxyFailedFields...)
			if hasClientCloseErr {
				closeOpenAIClientWS(wsConn, closeErr.StatusCode(), closeErr.Reason())
				return
			}
			closeOpenAIClientWS(wsConn, coderws.StatusInternalError, "upstream websocket proxy failed")
			return
		}
		reqLog.Info("openai.websocket_ingress_closed", zap.Int64("account_id", account.ID))
		return
	}

}

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/shared/httputil"
	"github.com/Wei-Shaw/sub2api/internal/shared/ip"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/Wei-Shaw/sub2api/internal/shared/xai"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// XSearch exposes xAI's native x_search tool as a bounded, non-streaming
// endpoint while retaining the normal gateway scheduling and billing contract.
func (h *OpenAIGatewayHandler) XSearch(c *gin.Context) {
	streamStarted := false
	defer h.recoverResponsesPanic(c, &streamStarted)
	setOpenAIClientTransportHTTP(c)
	requestStart := time.Now()

	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok || apiKey == nil || apiKey.Group == nil {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}
	if apiKey.Group.Platform != service.PlatformGrok {
		service.MarkOpsClientBusinessLimited(c, service.OpsClientBusinessLimitedReasonLocalFeatureGate)
		h.errorResponse(c, http.StatusNotFound, "not_found_error", "X Search API is only available for Grok groups")
		return
	}
	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(
		c,
		"handler.openai_gateway.x_search",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
	if !h.ensureResponsesDependencies(c, reqLog) {
		return
	}

	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if len(body) == 0 || !gjson.ValidBytes(body) {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}
	h.concurrencyHelper.SetPriorityAdmissionPendingBytes(c, int64(len(body)))

	var req grokXSearchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		logRequestBodyParseFailure(reqLog, body, err)
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		query = strings.TrimSpace(req.Input)
	}
	if query == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "query is required")
		return
	}
	req.Query = query
	maxResults := 0
	if req.MaxResults != nil {
		maxResults = *req.MaxResults
	}
	maxResults = normalizeGrokXSearchMaxResults(maxResults)

	requestedModel := xai.DefaultTextModel
	setOpsRequestContext(c, requestedModel, false)
	setOpsEndpointContext(c, "", int16(service.RequestTypeSync))
	pricingCtx, pricingAt := h.gatewayService.WithOpenAIRequestPricingContext(c.Request.Context(), apiKey.GroupID)
	c.Request = c.Request.WithContext(pricingCtx)
	auditBody, _ := json.Marshal(map[string]any{"model": requestedModel, "input": query})
	if decision := h.checkSecurityAudit(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIResponses, requestedModel, auditBody); decision != nil && !decision.AllowNextStage {
		h.openAISecurityAuditError(c, decision)
		return
	}

	searchBody, err := buildGrokXSearchResponsesBody(req, requestedModel, maxResults)
	if err != nil {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to build x_search request")
		return
	}
	channelMapping, _ := h.gatewayService.ResolveChannelMappingAndRestrict(c.Request.Context(), apiKey.GroupID, requestedModel)
	forwardBody := openAIModelMappedBody(searchBody, channelMapping.Mapped, channelMapping.MappedModel, h.gatewayService.ReplaceModelInBody)
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	service.SetOpsLatencyMs(c, service.OpsAuthLatencyMsKey, time.Since(requestStart).Milliseconds())

	userRelease, acquired := h.acquireResponsesUserSlot(c, subject.UserID, subject.Concurrency, false, &streamStarted, reqLog)
	if !acquired {
		return
	}
	if userRelease != nil {
		defer userRelease()
	}
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	requestID := "x_search:" + uuid.NewString()
	sessionHash := h.gatewayService.GenerateSessionHashWithFallbackForRequest(c, apiKey.GroupID, forwardBody, requestID)
	defer h.gatewayService.ReleaseOpenAIContentSessionRequest(c.Request.Context(), apiKey.GroupID, sessionHash)
	routingStart := time.Now()
	var failedAccountIDs map[int64]struct{}
	var sameAccountRetryCount map[int64]int
	profitVetoCount := 0
	switchCount := 0
	var lastFailoverErr *service.UpstreamFailoverError
	var oauth429FailoverState service.OpenAIOAuth429FailoverState

	for {
		selection, _, err := h.gatewayService.SelectAccountWithSchedulerForCapability(
			c.Request.Context(), apiKey.GroupID, "", sessionHash, requestedModel, failedAccountIDs,
			service.OpenAIUpstreamTransportHTTPSSE, service.OpenAIEndpointCapabilityChatCompletions,
			false, false, false, service.PlatformGrok,
		)
		if err != nil || selection == nil || selection.Account == nil {
			if failoverClientGone(c) {
				return
			}
			if errors.Is(err, service.ErrPriorityAdmissionUnavailable) {
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "Service temporarily unavailable, please retry later")
				return
			}
			if len(failedAccountIDs) == 0 {
				cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, requestedModel, requestedModel, service.PlatformGrok)
				if !cls.ModelNotFound {
					markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
				}
				h.errorResponse(c, cls.Status, cls.ErrType, cls.Message)
				return
			}
			if lastFailoverErr != nil {
				h.handleFailoverExhausted(c, lastFailoverErr, false)
			} else {
				h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
			}
			return
		}

		account := selection.Account
		setOpsSelectedAccount(c, account.ID, account.Platform)
		accountRelease, acquireOutcome := h.acquireResponsesAccountSlot(c, apiKey.GroupID, sessionHash, selection, false, &streamStarted, reqLog)
		if acquireOutcome == accountSlotAcquireProfitVeto {
			if !recordOpenAIProfitVeto(&failedAccountIDs, account.ID, &profitVetoCount) {
				h.handleOpenAIProfitVetoExhausted(c, streamStarted, reqLog, profitVetoCount)
				return
			}
			continue
		}
		if acquireOutcome == accountSlotAcquireReschedule {
			addFailedAccountID(&failedAccountIDs, account.ID)
			continue
		}
		if acquireOutcome != accountSlotAcquireReady {
			return
		}
		account = selection.Account
		service.SetOpsLatencyMs(c, service.OpsRoutingLatencyMsKey, time.Since(routingStart).Milliseconds())
		forwardStart := time.Now()
		forwardResult, err := func() (*service.GrokXSearchForwardResult, error) {
			if accountRelease != nil {
				defer accountRelease()
			}
			return h.gatewayService.ForwardGrokXSearch(c.Request.Context(), c, account, forwardBody, requestID)
		}()
		service.SetOpsLatencyMs(c, service.OpsResponseLatencyMsKey, time.Since(forwardStart).Milliseconds())
		if err != nil {
			if h.handleGrokXSearchFailover(c, reqLog, account, requestedModel, err, &failedAccountIDs, &sameAccountRetryCount, &switchCount, &lastFailoverErr, &oauth429FailoverState) {
				continue
			}
			return
		}
		if forwardResult == nil || forwardResult.Result == nil {
			h.errorResponse(c, http.StatusBadGateway, "upstream_error", "X Search returned no result")
			return
		}

		result := forwardResult.Result
		h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, account.GetMappedModel(requestedModel), true, nil)
		results := extractGrokSearchSources(forwardResult.Body, maxResults)
		h.recordGrokXSearchUsage(c, apiKey, account, subscription, channelMapping, requestedModel, body, result, subject.UserID, pricingAt)
		c.JSON(http.StatusOK, gin.H{
			"query": query, "results": results, "provider": "grok-native", "max_results": maxResults,
		})
		return
	}
}

func (h *OpenAIGatewayHandler) handleGrokXSearchFailover(
	c *gin.Context,
	reqLog *zap.Logger,
	account *service.Account,
	requestedModel string,
	err error,
	failedAccountIDs *map[int64]struct{},
	sameAccountRetryCount *map[int64]int,
	switchCount *int,
	lastFailoverErr **service.UpstreamFailoverError,
	oauth429FailoverState *service.OpenAIOAuth429FailoverState,
) bool {
	var failoverErr *service.UpstreamFailoverError
	if !errors.As(err, &failoverErr) {
		h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, account.GetMappedModel(requestedModel), false, nil)
		if !service.IsResponseCommitted(c) {
			h.errorResponse(c, http.StatusBadGateway, "upstream_error", "X Search request failed")
		}
		reqLog.Warn("openai_x_search.forward_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		return false
	}
	if failoverClientGone(c) {
		return false
	}
	if failoverErr.ShouldReportAccountScheduleFailure() {
		h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, account.GetMappedModel(requestedModel), false, nil)
	}
	if !failoverErr.ShouldRetryNextAccount() {
		h.handleFailoverExhausted(c, failoverErr, false)
		return false
	}
	if failoverErr.RetryableOnSameAccount {
		retryLimit := account.GetPoolModeRetryCount()
		if retryCount, retry := tryIncrementSameAccountRetry(sameAccountRetryCount, account.ID, retryLimit); retry {
			return sleepWithContext(c.Request.Context(), sameAccountRetryDelayFor(failoverErr, retryCount))
		}
	}
	h.gatewayService.RecordOpenAIAccountSwitch()
	addFailedAccountID(failedAccountIDs, account.ID)
	*lastFailoverErr = failoverErr
	if *switchCount >= h.maxAccountSwitches {
		h.handleFailoverExhausted(c, failoverErr, false)
		return false
	}
	*switchCount++
	if h.gatewayService.ShouldStopOpenAIOAuth429Failover(account, failoverErr.StatusCode, *switchCount, oauth429FailoverState) {
		h.handleFailoverExhausted(c, failoverErr, false)
		return false
	}
	return true
}

func (h *OpenAIGatewayHandler) recordGrokXSearchUsage(
	c *gin.Context,
	apiKey *service.APIKey,
	account *service.Account,
	subscription *service.UserSubscription,
	channelMapping service.ChannelMappingResult,
	requestedModel string,
	body []byte,
	result *service.OpenAIForwardResult,
	userID int64,
	pricingAt time.Time,
) {
	userAgent := c.GetHeader("User-Agent")
	clientIP := ip.GetClientIP(c)
	sessionID := service.ExtractClientSessionID(c)
	requestPayloadHash := service.HashUsageRequestPayload(body)
	inboundEndpoint := GetInboundEndpoint(c)
	upstreamEndpoint := resolveOpenAIUpstreamEndpoint(c, account, result)
	quotaPlatform := service.QuotaPlatform(c.Request.Context(), apiKey)
	h.submitMandatoryUsageRecordTask(c.Request.Context(), func(ctx context.Context) {
		if err := h.gatewayService.RecordUsage(ctx, &service.OpenAIRecordUsageInput{
			Result: result, APIKey: apiKey, User: apiKey.User, Account: account, Subscription: subscription,
			InboundEndpoint: inboundEndpoint, UpstreamEndpoint: upstreamEndpoint, UserAgent: userAgent,
			IPAddress: clientIP, SessionID: sessionID, RequestPayloadHash: requestPayloadHash,
			APIKeyService: h.apiKeyService, QuotaPlatform: quotaPlatform, PricingAt: pricingAt,
			ChannelUsageFields: channelMapping.ToUsageFields(requestedModel, result.UpstreamModel),
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.x_search"),
				zap.Int64("user_id", userID), zap.Int64("api_key_id", apiKey.ID), zap.Int64("account_id", account.ID),
			).Error("openai_x_search.record_usage_failed", zap.Error(err))
		}
	})
}

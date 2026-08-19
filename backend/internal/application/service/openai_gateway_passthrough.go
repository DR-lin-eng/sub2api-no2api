package service

// 本文件承载 /v1/responses 透传转发及其流式、非流式响应与错误处理。

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/ctxkey"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/Wei-Shaw/sub2api/internal/shared/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

const openAIPassthroughTransportRetryReason GatewayFailureReason = "openai_passthrough_transport_retry"

func markOpenAIPassthroughTransportRetry(err *UpstreamFailoverError) {
	if err == nil {
		return
	}
	err.RetryableOnSameAccount = true
	err.Reason = openAIPassthroughTransportRetryReason
}

const openAIPassthroughPreOutputBufferLimit = openAIStreamPreOutputBufferLimit

func (s *OpenAIGatewayService) forwardOpenAIPassthrough(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	canonicalImageIntentBody []byte,
	reqModel string,
	attemptImageIntentInvalidated bool,
	reasoningEffort *string,
	reqStream bool,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	responseHeaderSnapshot := cloneOpenAIUncommittedResponseHeaders(c)
	agentTaskRecoveryTried := false
	var fingerprintIDs *codexFingerprintIDs
	if account != nil && account.Type == AccountTypeOAuth &&
		(!isOpenAIResponsesCompactPath(c) || s.codexFullSimulationEnabledForAccount(c, account)) {
		fingerprintIDs = resolveCodexFingerprintIDsFromGinContext(account, c)
	}
	// Reset the request-scoped plan even when this attempt uses an API-key or
	// legacy compact account. Failover reuses the Gin context across attempts.
	stageCodexFingerprintIDs(c, fingerprintIDs)
	for completedRetries := 0; ; completedRetries++ {
		if completedRetries > 0 && ctx != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
		}
		result, err := s.forwardOpenAIPassthroughOnce(
			ctx,
			c,
			account,
			body,
			canonicalImageIntentBody,
			reqModel,
			attemptImageIntentInvalidated,
			reasoningEffort,
			reqStream,
			startTime,
			&agentTaskRecoveryTried,
			fingerprintIDs,
		)
		if err == nil {
			return result, nil
		}
		restoreOpenAIUncommittedResponseHeaders(c, responseHeaderSnapshot)

		if ctx != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return result, ctxErr
			}
		}
		var failoverErr *UpstreamFailoverError
		if !errors.As(err, &failoverErr) || !failoverErr.RetryableOnSameAccount {
			return result, err
		}
		if failoverErr.Reason != openAIPassthroughTransportRetryReason {
			return result, err
		}
		if completedRetries >= len(openAIPassthroughTransportRetryBackoffs) {
			// The service has exhausted the same-account retry budget. The handler
			// must switch accounts immediately instead of applying its pool retry.
			failoverErr.RetryableOnSameAccount = false
			return result, err
		}

		delay := openAIPassthroughTransportRetryBackoffs[completedRetries]
		logger.FromContext(ctx).With(
			zap.String("component", "service.openai_gateway"),
			zap.Int64("account_id", account.ID),
			zap.Int("retry_count", completedRetries+1),
			zap.Int("retry_max", len(openAIPassthroughTransportRetryBackoffs)),
			zap.Duration("retry_delay", delay),
			zap.Error(err),
		).Warn("openai.passthrough_response_retry")
		if waitErr := waitOpenAITransportRetry(ctx, delay); waitErr != nil {
			return result, waitErr
		}
	}
}

func (s *OpenAIGatewayService) forwardOpenAIPassthroughOnce(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	canonicalImageIntentBody []byte,
	reqModel string,
	attemptImageIntentInvalidated bool,
	reasoningEffort *string,
	reqStream bool,
	startTime time.Time,
	agentTaskRecoveryTried *bool,
	fingerprintIDs *codexFingerprintIDs,
) (*OpenAIForwardResult, error) {
	upstreamPassthroughModel := ""
	if isOpenAIResponsesCompactPath(c) {
		compactMappedModel := resolveOpenAICompactForwardModel(account, reqModel)
		if compactMappedModel != "" && compactMappedModel != reqModel {
			nextBody, setErr := sjson.SetBytes(body, "model", compactMappedModel)
			if setErr != nil {
				return nil, fmt.Errorf("set compact passthrough model: %w", setErr)
			}
			body = nextBody
			upstreamPassthroughModel = compactMappedModel
			attemptImageIntentInvalidated = true
		}
	}

	if account != nil && account.Type == AccountTypeOAuth {
		if rejectReason := detectOpenAIPassthroughInstructionsRejectReason(reqModel, body); rejectReason != "" {
			rejectMsg := "OpenAI codex passthrough requires a non-empty instructions field"
			MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
			logOpenAIPassthroughInstructionsRejected(ctx, c, account, reqModel, rejectReason, body)
			c.JSON(http.StatusForbidden, gin.H{
				"error": gin.H{
					"type":    "forbidden_error",
					"message": rejectMsg,
				},
			})
			return nil, fmt.Errorf("openai passthrough rejected before upstream: %s", rejectReason)
		}
		if isOpenAICodexModel(reqModel) && !gjson.GetBytes(body, "instructions").Exists() {
			nextBody, setErr := sjson.SetBytes(body, "instructions", defaultCodexSynthInstructions(reqModel))
			if setErr != nil {
				return nil, fmt.Errorf("set passthrough codex instructions: %w", setErr)
			}
			body = nextBody
		}

		normalizedBody, normalized, err := normalizeOpenAIPassthroughOAuthBody(body, isOpenAIResponsesCompactPath(c))
		if err != nil {
			return nil, err
		}
		if normalized {
			body = normalizedBody
		}
		if !isOpenAIResponsesCompactPath(c) || (fingerprintIDs != nil && fingerprintIDs.fullSimulation) {
			fingerprintedBody, fingerprinted, fingerprintErr := applyCodexFingerprintClientMetadataToBody(body, fingerprintIDs)
			if fingerprintErr != nil {
				return nil, fingerprintErr
			}
			if fingerprinted {
				body = fingerprintedBody
			}
		}
		reqStream = gjson.GetBytes(body, "stream").Bool()
	}

	sanitizedBody, sanitized, err := sanitizeEmptyBase64InputImagesInOpenAIBody(body)
	if err != nil {
		return nil, err
	}
	if sanitized {
		body = sanitizedBody
	}

	// Apply OpenAI fast policy to the passthrough body (filter/block by service_tier).
	// 统一使用 upstream 视角的 model：透传路径下 body 已经过 compact 映射 +
	// OAuth normalize，body 中的 model 字段即上游真正会看到的 slug。
	// 这样可以与 chat-completions / messages / native /responses 入口的
	// upstreamModel 保持一致，避免 whitelist 命中差异。当 body 中没有
	// model 字段时退回 reqModel。
	policyModel := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if policyModel == "" {
		policyModel = reqModel
	}
	updatedBody, policyErr := s.applyOpenAIFastPolicyToBody(ctx, account, policyModel, body)
	if policyErr != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(policyErr, &blocked) {
			writeOpenAIFastPolicyBlockedResponse(c, blocked)
		}
		return nil, policyErr
	}
	body = updatedBody

	apiKey := getAPIKeyFromContext(c)
	// 同一 attempt 的最终 model/body 只判定一次，权限检查与后续图片状态设置共用该结果。
	imageIntent := resolveOpenAIPassthroughImageIntent(
		c,
		reqModel,
		canonicalImageIntentBody,
		policyModel,
		body,
		attemptImageIntentInvalidated,
		IsImageGenerationIntent,
	)
	if imageIntent && !GroupAllowsImageGeneration(apiKeyGroup(apiKey)) {
		MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalFeatureGate)
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"type":    "permission_error",
				"message": ImageGenerationPermissionMessage(),
			},
		})
		return nil, errors.New("image generation disabled for group")
	}
	imageBillingModel := ""
	imageSizeTier := ""
	imageInputSize := ""
	if imageIntent {
		var imageCfgErr error
		imageCfg, imageCfgErr := resolveOpenAIResponsesImageBillingConfigDetailedFromBody(body, reqModel)
		if imageCfgErr != nil {
			setOpsUpstreamError(c, http.StatusBadRequest, imageCfgErr.Error(), "")
			c.JSON(http.StatusBadRequest, gin.H{
				"error": gin.H{
					"type":    "invalid_request_error",
					"message": imageCfgErr.Error(),
					"param":   "size",
				},
			})
			return nil, imageCfgErr
		}
		imageBillingModel = imageCfg.Model
		imageSizeTier = imageCfg.SizeTier
		imageInputSize = imageCfg.InputSize
	}

	logger.LegacyPrintf("service.openai_gateway",
		"[OpenAI 自动透传] 命中自动透传分支: account=%d name=%s type=%s model=%s stream=%v",
		account.ID,
		account.Name,
		account.Type,
		reqModel,
		reqStream,
	)
	if reqStream && c != nil && c.Request != nil {
		if timeoutHeaders := collectOpenAIPassthroughTimeoutHeaders(c.Request.Header); len(timeoutHeaders) > 0 {
			streamWarnLogger := logger.FromContext(ctx).With(
				zap.String("component", "service.openai_gateway"),
				zap.Int64("account_id", account.ID),
				zap.Strings("timeout_headers", timeoutHeaders),
			)
			if s.isOpenAIPassthroughTimeoutHeadersAllowed() {
				streamWarnLogger.Warn("OpenAI passthrough 透传请求包含超时相关请求头，且当前配置为放行，可能导致上游提前断流")
			} else {
				streamWarnLogger.Warn("OpenAI passthrough 检测到超时相关请求头，将按配置过滤以降低断流风险")
			}
		}
	}

	// Get access token
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}

	proxyURL := ""
	if account.ProxyID != nil && account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}

	if c != nil {
		c.Set("openai_passthrough", true)
	}

	var resp *http.Response
	reasoningEffortValue := ""
	if reasoningEffort != nil {
		reasoningEffortValue = strings.TrimSpace(*reasoningEffort)
	}
	firstOutputTimeout := time.Duration(0)
	if reqStream && account != nil && account.Platform == PlatformOpenAI {
		firstOutputTimeout = s.openAIFirstOutputTimeoutWithContext(ctx, reasoningEffortValue)
	}
	for {
		upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
		var headerGuard *openAIFirstOutputHeaderGuard
		if firstOutputTimeout > 0 {
			upstreamCtx, headerGuard = newOpenAIFirstOutputHeaderGuard(
				upstreamCtx, releaseUpstreamCtx, startTime.Add(firstOutputTimeout),
			)
		}
		upstreamReq, buildErr := s.buildUpstreamRequestOpenAIPassthroughWithFingerprint(upstreamCtx, c, account, body, token, fingerprintIDs)
		if headerGuard == nil {
			releaseUpstreamCtx()
		}
		if buildErr != nil {
			if headerGuard != nil {
				headerGuard.close()
			}
			return nil, buildErr
		}

		upstreamStart := time.Now()
		resp, err = doAccountHTTPUpstream(s.httpUpstream, upstreamReq, proxyURL, account)
		SetOpsLatencyMs(c, OpsUpstreamLatencyMsKey, time.Since(upstreamStart).Milliseconds())
		if headerGuard != nil && headerGuard.stopHeaderWait() {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			headerGuard.close()
			return nil, s.newOpenAIFirstOutputTimeoutError(
				ctx, c, account, startTime, reqModel, reasoningEffortValue,
				firstOutputTimeout, "passthrough_response_headers", nil,
			)
		}
		if err != nil {
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
			if headerGuard != nil {
				headerGuard.close()
			}
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			// Transport-level failure (proxy/DNS/TCP/TLS — no HTTP response). Convert to
			// a failover so the handler switches to a healthy account, and temporarily
			// unschedule the account on durable faults (e.g. rejected proxy credentials).
			failoverErr := s.handleOpenAIUpstreamTransportError(ctx, c, account, err, true)
			if typed, ok := failoverErr.(*UpstreamFailoverError); ok && !classifyOpenAITransportError(err).Persistent {
				markOpenAIPassthroughTransportRetry(typed)
			}
			return nil, failoverErr
		}
		if headerGuard != nil {
			resp.Body = &openAIRequestContextReadCloser{ReadCloser: resp.Body, cleanup: headerGuard.close}
		}
		if resp.StatusCode < http.StatusBadRequest {
			break
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			_ = resp.Body.Close()
			return nil, ctxErr
		}

		// Peek only to identify an invalid task. Restore the body so the existing
		// passthrough error handling sees the same response after recovery fails.
		probeBody := s.readUpstreamErrorBody(resp)
		_ = resp.Body.Close()
		resp.Body = io.NopCloser(bytes.NewReader(probeBody))
		if agentTaskRecoveryTried != nil && !*agentTaskRecoveryTried && s.isAgentIdentityAccount(ctx, account) && isAgentIdentityTaskInvalidHTTPResponse(resp.StatusCode, probeBody) {
			*agentTaskRecoveryTried = true
			expectedTaskID := account.GetCredential("task_id")
			if recoveryErr := s.recoverAgentIdentityTask(ctx, account, expectedTaskID); recoveryErr != nil {
				return nil, fmt.Errorf("agent identity task recovery failed: %w", recoveryErr)
			}
			continue
		}

		// Any HTTP error received from OpenAI is handed to account failover before
		// downstream output is committed. The handler preserves the final upstream
		// status, headers, and body after all candidate accounts are exhausted.
		if shouldFailoverOpenAIPassthroughResponse(account, resp.StatusCode, probeBody) {
			return nil, s.handleFailoverErrorResponsePassthrough(ctx, resp, c, account, body, probeBody)
		}
		return nil, s.handleErrorResponsePassthrough(ctx, resp, c, account, body, probeBody)
	}
	defer func() { _ = resp.Body.Close() }()
	serviceTier := extractOpenAIServiceTierFromBody(body)

	var usage *OpenAIUsage
	var firstTokenMs *int
	responseID := ""
	imageCount := 0
	var imageOutputSizes []string
	if reqStream {
		result, err := s.handleStreamingResponsePassthrough(
			ctx, resp, c, account, startTime, reqModel, upstreamPassthroughModel, reasoningEffortValue,
		)
		if err != nil {
			return nil, err
		}
		usage = result.usage
		firstTokenMs = result.firstTokenMs
		responseID = strings.TrimSpace(result.responseID)
		imageCount = result.imageCount
		imageOutputSizes = result.imageOutputSizes
	} else {
		result, err := s.handleNonStreamingResponsePassthrough(ctx, resp, c, account, reqModel, upstreamPassthroughModel)
		if err != nil {
			return nil, err
		}
		usage = result.usage
		responseID = strings.TrimSpace(result.responseID)
		imageCount = result.imageCount
		imageOutputSizes = result.imageOutputSizes
	}
	s.bindHTTPResponseAccount(ctx, c, account, responseID)

	// 排除 spark 影子:其 codex_* 仅由 QueryUsage(/wham/usage bengalfox)更新(外审第7轮 P1)。
	if !account.IsShadow() {
		if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
			s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
		}
	}

	if usage == nil {
		usage = &OpenAIUsage{}
	}

	forwardResult := &OpenAIForwardResult{
		RequestID:                     resp.Header.Get("x-request-id"),
		ResponseID:                    responseID,
		Usage:                         *usage,
		Model:                         reqModel,
		UpstreamModel:                 upstreamPassthroughModel,
		UpstreamResponseModel:         observedUpstreamResponseModel(c),
		UpstreamResponseModelConflict: observedUpstreamResponseModelConflict(c),
		ServiceTier:                   serviceTier,
		ReasoningEffort:               reasoningEffort,
		Stream:                        reqStream,
		OpenAIWSMode:                  false,
		Duration:                      time.Since(startTime),
		FirstTokenMs:                  firstTokenMs,
	}
	if imageCount > 0 {
		forwardResult.ImageCount = imageCount
		forwardResult.ImageSize = imageSizeTier
		forwardResult.ImageInputSize = imageInputSize
		forwardResult.ImageOutputSizes = imageOutputSizes
		forwardResult.BillingModel = imageBillingModel
	}
	return forwardResult, nil
}

func logOpenAIPassthroughInstructionsRejected(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	reqModel string,
	rejectReason string,
	body []byte,
) {
	if ctx == nil {
		ctx = context.Background()
	}
	accountID := int64(0)
	accountName := ""
	accountType := ""
	if account != nil {
		accountID = account.ID
		accountName = strings.TrimSpace(account.Name)
		accountType = strings.TrimSpace(string(account.Type))
	}
	fields := []zap.Field{
		zap.String("component", "service.openai_gateway"),
		zap.Int64("account_id", accountID),
		zap.String("account_name", accountName),
		zap.String("account_type", accountType),
		zap.String("request_model", strings.TrimSpace(reqModel)),
		zap.String("reject_reason", strings.TrimSpace(rejectReason)),
	}
	fields = appendCodexCLIOnlyRejectedRequestFields(fields, c, body)
	logger.FromContext(ctx).With(fields...).Warn("OpenAI passthrough 本地拦截：Codex 请求缺少有效 instructions")
}

func (s *OpenAIGatewayService) buildUpstreamRequestOpenAIPassthrough(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
) (*http.Request, error) {
	return s.buildUpstreamRequestOpenAIPassthroughWithFingerprint(ctx, c, account, body, token, nil)
}

func (s *OpenAIGatewayService) buildUpstreamRequestOpenAIPassthroughWithFingerprint(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	token string,
	fingerprintIDs *codexFingerprintIDs,
) (*http.Request, error) {
	// Failover reuses the request context; replace any prior account's plan
	// before headers are assembled so identities cannot cross account attempts.
	stageCodexFingerprintIDs(c, fingerprintIDs)
	targetURL := openaiPlatformAPIURL
	switch account.Type {
	case AccountTypeOAuth:
		targetURL = chatgptCodexURL
	case AccountTypeAPIKey:
		baseURL := account.GetOpenAIBaseURL()
		if baseURL != "" {
			validatedURL, err := s.validateUpstreamBaseURL(baseURL)
			if err != nil {
				return nil, err
			}
			targetURL = buildOpenAIResponsesURL(validatedURL)
		}
	}
	targetURL = appendOpenAIResponsesRequestPathSuffix(targetURL, openAIResponsesRequestPathSuffix(c))

	promptCacheKey := strings.TrimSpace(gjson.GetBytes(body, "prompt_cache_key").String())
	outboundBody := body
	var codexSessionIDs *codexOutboundSessionIDs
	if account.IsOpenAIOAuth() && (fingerprintIDs == nil || fingerprintIDs.mode == codexFingerprintOff) {
		codexSessionIDs = resolveCodexOutboundSessionIDs(c, account, body, promptCacheKey)
		var rewriteErr error
		outboundBody, rewriteErr = rewriteCodexOutboundSessionMetadata(body, codexSessionIDs)
		if rewriteErr != nil {
			return nil, rewriteErr
		}
	}

	req, err := newOpenAIHTTPUpstreamRequest(ctx, http.MethodPost, targetURL, account, outboundBody)
	if err != nil {
		return nil, err
	}
	stream := gjson.GetBytes(outboundBody, "stream").Bool()
	req = req.WithContext(WithHTTPUpstreamProfile(req.Context(), openAIHTTPUpstreamProfile(ctx, account, stream)))

	// 透传客户端请求头（安全白名单）。
	allowTimeoutHeaders := s.isOpenAIPassthroughTimeoutHeadersAllowed()
	if c != nil && c.Request != nil {
		for key, values := range c.Request.Header {
			lower := strings.ToLower(strings.TrimSpace(key))
			if !isOpenAIPassthroughAllowedRequestHeader(lower, allowTimeoutHeaders) {
				continue
			}
			for _, v := range values {
				req.Header.Add(key, v)
			}
		}
	}
	if account.IsOpenAIOAuth() && s.CodexSimulationRequestEnabled(c) {
		req.Header.Del(CodexProjectIDHeader)
	}
	// Failover can reuse the downstream turn-state with a different account.
	// Strip only values whose provenance is known to be cross-account.
	s.guardOpenAICodexTurnStateEcho(c, account, req.Header)

	// 覆盖入站鉴权残留，并注入上游认证
	req.Header.Del("authorization")
	req.Header.Del("x-api-key")
	req.Header.Del("x-goog-api-key")
	authHeaders, err := s.buildOpenAIAuthenticationHeaders(ctx, account, token)
	if err != nil {
		return nil, fmt.Errorf("build openai authentication headers: %w", err)
	}
	for key, values := range authHeaders {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// OAuth 透传到 ChatGPT internal API 时补齐必要头。
	if account.Type == AccountTypeOAuth {
		// Current Codex OAuth HTTP no longer negotiates the legacy Responses
		// experiment. Passthrough may receive it from an older client, so remove
		// only that token while preserving any independent beta negotiation.
		stripOpenAILegacyResponsesBeta(req.Header)
		req.Host = "chatgpt.com"
		if err := resolveAndSetOpenAIChatGPTAccountHeaders(ctx, s.accountRepo, req.Header, account); err != nil {
			return nil, fmt.Errorf("resolve chatgpt account headers: %w", err)
		}
		apiKeyID := getAPIKeyIDFromContext(c)
		// 先保存客户端原始值，再做 compact 补充，避免后续统一隔离时读到已处理的值。
		clientSessionID := strings.TrimSpace(req.Header.Get("session_id"))
		clientConversationID := strings.TrimSpace(req.Header.Get("conversation_id"))
		if isOpenAIResponsesCompactPath(c) {
			req.Header.Set("accept", "application/json")
			if req.Header.Get("version") == "" {
				req.Header.Set("version", CodexCanonicalClientVersion())
			}
			if clientSessionID == "" {
				clientSessionID = resolveOpenAICompactSessionID(c)
			}
		} else if req.Header.Get("accept") == "" {
			req.Header.Set("accept", "text/event-stream")
		}
		if req.Header.Get("originator") == "" {
			req.Header.Set("originator", resolveCodexOutboundIdentity("").originator)
		}
		// 用隔离后的 session 标识符覆盖客户端透传值，防止跨用户会话碰撞。
		if clientSessionID == "" {
			clientSessionID = promptCacheKey
		}
		if clientConversationID == "" {
			clientConversationID = promptCacheKey
		}
		if clientSessionID != "" {
			req.Header.Set("session_id", isolateOpenAISessionID(apiKeyID, clientSessionID))
		}
		if clientConversationID != "" {
			req.Header.Set("conversation_id", isolateOpenAISessionID(apiKeyID, clientConversationID))
		}
		applyResolvedCodexOutboundSessionHeaders(c, account, req.Header, fingerprintIDs, codexSessionIDs)
	} else if isOpenAIResponsesCompactPath(c) {
		// 透传白名单会放行客户端的 Accept: text/event-stream；compact 上游是
		// unary JSON 协议，API-key 账号同样强制 Accept，避免上游按 SSE 返回
		// （#3777 期望行为 4）。
		req.Header.Set("accept", "application/json")
	}

	// 透传模式也支持账户自定义 User-Agent 与 ForceCodexCLI 兜底。
	customUA := account.GetOpenAIUserAgent()
	if customUA != "" {
		req.Header.Set("user-agent", customUA)
	}
	if s.cfg != nil && s.cfg.Gateway.ForceCodexCLI {
		req.Header.Set("user-agent", CodexCanonicalUserAgent())
	}
	if account.Type == AccountTypeOAuth {
		if !isOpenAIResponsesCompactPath(c) || (fingerprintIDs != nil && fingerprintIDs.fullSimulation) {
			applyStagedCodexFingerprintHeaders(c, account, req.Header)
		}
		enforceCodexIdentityHeadersWithUA(req.Header, s.codexIdentityOverrideUA(account))
	}

	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
	}

	// 账号级请求头覆写（仅 openai api_key 账号启用时生效；OAuth 路径 no-op）
	account.ApplyHeaderOverrides(req.Header)
	applyOpenAICodexBetaFeatures(c, account, req.Header)
	applyOpenAICodexRoutingHintFromBody(ctx, account, "http_passthrough", req.Header, outboundBody, "not_applicable")
	applyCodexSimulationProfileHeaders(req.Header, fingerprintIDs)

	return req, nil
}

func stripOpenAILegacyResponsesBeta(headers http.Header) {
	if headers == nil {
		return
	}

	preserved := make([]string, 0)
	for key, values := range headers {
		if !strings.EqualFold(strings.TrimSpace(key), "OpenAI-Beta") {
			continue
		}
		delete(headers, key)
		for _, value := range values {
			parts := strings.Split(value, ",")
			kept := parts[:0]
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part == "" || strings.EqualFold(part, "responses=experimental") {
					continue
				}
				kept = append(kept, part)
			}
			if len(kept) > 0 {
				preserved = append(preserved, strings.Join(kept, ", "))
			}
		}
	}
	for _, value := range preserved {
		headers.Add("OpenAI-Beta", value)
	}
}

func shouldFailoverOpenAIPassthroughResponse(account *Account, statusCode int, responseBody []byte) bool {
	if statusCode < http.StatusBadRequest {
		return false
	}
	if cyberHit, _, _ := detectOpenAICyberPolicy(responseBody); cyberHit {
		return false
	}
	if isOpenAIRequestBodyTooLargeError(statusCode, "", responseBody) {
		return true
	}
	if account != nil && account.IsPoolMode() && account.IsPoolModeRetryableStatus(statusCode) {
		return true
	}
	switch statusCode {
	case http.StatusBadRequest, http.StatusConflict, http.StatusUnprocessableEntity:
		// These normally describe the request itself. Trying every account only
		// multiplies latency and upstream traffic for a deterministic failure.
		return false
	case http.StatusUnauthorized,
		http.StatusPaymentRequired,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusRequestTimeout,
		http.StatusTooEarly,
		http.StatusTooManyRequests:
		return true
	default:
		return statusCode >= http.StatusInternalServerError
	}
}

func writeOpenAIPassthroughErrorHeaders(dst, src http.Header) {
	if dst == nil {
		return
	}
	dst.Set("Content-Type", "application/json; charset=utf-8")
	dst.Set("Cache-Control", "no-store")
	dst.Del("Retry-After")
	if src == nil {
		return
	}
	rawRetryAfter := strings.TrimSpace(src.Get("Retry-After"))
	if validOpenAIPassthroughRetryAfter(rawRetryAfter, time.Now()) {
		dst.Set("Retry-After", rawRetryAfter)
	}
}

func validOpenAIPassthroughRetryAfter(raw string, now time.Time) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	delaySeconds := true
	for i := 0; i < len(raw); i++ {
		if raw[i] < '0' || raw[i] > '9' {
			delaySeconds = false
			break
		}
	}
	if delaySeconds {
		seconds, err := strconv.ParseUint(raw, 10, 64)
		return err == nil && seconds > 0
	}
	parsed, err := http.ParseTime(raw)
	return err == nil && parsed.After(now)
}

func writeSanitizedOpenAIPassthroughError(c *gin.Context, upstreamStatus int, upstreamHeaders http.Header) {
	downstreamStatus, message := sanitizedOpenAIPassthroughErrorStatusAndMessage(upstreamStatus)
	writeOpenAIPassthroughErrorEnvelope(c, downstreamStatus, upstreamHeaders, message)
}

func sanitizedOpenAIPassthroughErrorStatusAndMessage(upstreamStatus int) (int, string) {
	downstreamStatus := upstreamStatus
	message := "Upstream request failed"
	switch upstreamStatus {
	case http.StatusUnauthorized:
		downstreamStatus = http.StatusBadGateway
		message = "Upstream authentication failed"
	case http.StatusForbidden:
		downstreamStatus = http.StatusBadGateway
		message = "Upstream access denied"
	default:
		if upstreamStatus >= http.StatusInternalServerError {
			message = "Upstream service temporarily unavailable"
		}
	}
	return downstreamStatus, message
}

func marshalOpenAIPassthroughErrorEnvelope(message string) []byte {
	body, _ := json.Marshal(gin.H{
		"error": gin.H{
			"type":    "upstream_error",
			"message": message,
		},
	})
	return body
}

// writeOpenAIPassthroughErrorEnvelope 以本地 JSON 信封 + 净化后的头策略写出
// 错误响应；message 由调用方决定（净化通用文案或脱敏后的上游消息）。
func writeOpenAIPassthroughErrorEnvelope(c *gin.Context, downstreamStatus int, upstreamHeaders http.Header, message string) {
	if c == nil {
		return
	}
	body := marshalOpenAIPassthroughErrorEnvelope(message)
	if writeOpenAICompactSSEBridge(c, downstreamStatus, body) {
		return
	}
	writeOpenAIPassthroughErrorHeaders(c.Writer.Header(), upstreamHeaders)
	c.Data(downstreamStatus, "application/json; charset=utf-8", body)
}

func (s *OpenAIGatewayService) handleFailoverErrorResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestBody []byte,
	responseBody []byte,
) error {
	body := s.redactAgentIdentitySensitiveBody(ctx, account, responseBody)
	cyberHit, cyberCode, cyberMsg := detectOpenAICyberPolicy(body)
	if cyberHit {
		MarkOpsCyberPolicy(c, CyberPolicyMark{
			Code:           cyberCode,
			Message:        cyberMsg,
			Body:           truncateString(string(body), 4096),
			UpstreamStatus: resp.StatusCode,
		})
	}

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(body))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	logOpenAIInstructionsRequiredDebug(ctx, c, account, resp.StatusCode, upstreamMsg, requestBody, body)
	if !cyberHit {
		reqModel, _, _ := extractOpenAIRequestMetaFromBody(requestBody)
		canonicalModel := canonicalOpenAIAccountSchedulingModel(account, reqModel)
		_ = s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, body, canonicalModel)
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:             account.Platform,
		AccountID:            account.ID,
		AccountName:          account.Name,
		UpstreamStatusCode:   resp.StatusCode,
		UpstreamRequestID:    resp.Header.Get("x-request-id"),
		Passthrough:          true,
		Kind:                 "failover",
		Message:              upstreamMsg,
		Detail:               upstreamDetail,
		UpstreamResponseBody: upstreamDetail,
	})
	failoverErr := newOpenAIUpstreamFailoverError(
		resp.StatusCode,
		resp.Header,
		body,
		upstreamMsg,
		account.IsPoolMode() && account.IsPoolModeRetryableStatus(resp.StatusCode),
	)
	_, clientMessage := sanitizedOpenAIPassthroughErrorStatusAndMessage(resp.StatusCode)
	if isOpenAIContextWindowError(upstreamMsg, body) && upstreamMsg != "" {
		clientMessage = upstreamMsg
	}
	clientHeaders := make(http.Header)
	writeOpenAIPassthroughErrorHeaders(clientHeaders, resp.Header)
	failoverErr.ResponseBody = marshalOpenAIPassthroughErrorEnvelope(clientMessage)
	failoverErr.ResponseHeaders = clientHeaders
	failoverErr.PreserveUpstreamResponse = true
	return failoverErr
}

func (s *OpenAIGatewayService) handleErrorResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestBody []byte,
	responseBody []byte,
) error {
	MarkResponseCommitted(c)
	body := s.redactAgentIdentitySensitiveBody(ctx, account, responseBody)

	// cyber_policy 仍按原始 body 打内部标记，供 handler 事后写风控/邮件；面向客户端的
	// 错误体在下方统一重建。cyber 是上游网络安全策略拦截，不冷却账号，
	// 故下方跳过 handleOpenAIAccountUpstreamError（避免自定义 temp-unschedulable 规则误冷却）。
	cyberHit, cyberCode, cyberMsg := detectOpenAICyberPolicy(body)
	if cyberHit {
		MarkOpsCyberPolicy(c, CyberPolicyMark{
			Code:           cyberCode,
			Message:        cyberMsg,
			Body:           truncateString(string(body), 4096),
			UpstreamStatus: resp.StatusCode,
		})
	}

	upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(body))
	upstreamMsg = sanitizeUpstreamErrorMessage(upstreamMsg)
	upstreamDetail := ""
	if s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, resp.StatusCode, upstreamMsg, upstreamDetail)
	logOpenAIInstructionsRequiredDebug(ctx, c, account, resp.StatusCode, upstreamMsg, requestBody, body)
	// 错误体虽不会原样透传，运行态账号状态仍需更新，避免粘性路由继续复用
	// 刚被限流的账号。cyber 例外：不冷却账号。
	if !cyberHit {
		reqModel, _, _ := extractOpenAIRequestMetaFromBody(requestBody)
		canonicalModel := canonicalOpenAIAccountSchedulingModel(account, reqModel)
		_ = s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, body, canonicalModel)
	}
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:             account.Platform,
		AccountID:            account.ID,
		AccountName:          account.Name,
		UpstreamStatusCode:   resp.StatusCode,
		UpstreamRequestID:    resp.Header.Get("x-request-id"),
		Passthrough:          true,
		Kind:                 "http_error",
		Message:              upstreamMsg,
		Detail:               upstreamDetail,
		UpstreamResponseBody: upstreamDetail,
	})
	// context-window 超限是确定性请求失败（shouldFailoverOpenAIPassthroughResponse
	// 已保证不切号），其文案对客户端可操作（如触发自动压缩）；在净化信封内保留
	// 脱敏后的上游消息，而不是抹成通用文案。
	if isOpenAIContextWindowError(upstreamMsg, body) && upstreamMsg != "" {
		writeOpenAIPassthroughErrorEnvelope(c, resp.StatusCode, resp.Header, upstreamMsg)
	} else {
		writeSanitizedOpenAIPassthroughError(c, resp.StatusCode, resp.Header)
	}

	return fmt.Errorf("upstream error: %d (client response sanitized)", resp.StatusCode)
}

func isOpenAIPassthroughAllowedRequestHeader(lowerKey string, allowTimeoutHeaders bool) bool {
	if lowerKey == "" {
		return false
	}
	if isOpenAIPassthroughTimeoutHeader(lowerKey) {
		return allowTimeoutHeaders
	}
	return openaiPassthroughAllowedHeaders[lowerKey]
}

func isOpenAIPassthroughTimeoutHeader(lowerKey string) bool {
	switch lowerKey {
	case "x-stainless-timeout", "x-stainless-read-timeout", "x-stainless-connect-timeout", "x-request-timeout", "request-timeout", "grpc-timeout":
		return true
	default:
		return false
	}
}

func (s *OpenAIGatewayService) isOpenAIPassthroughTimeoutHeadersAllowed() bool {
	return s != nil && s.cfg != nil && s.cfg.Gateway.OpenAIPassthroughAllowTimeoutHeaders
}

func collectOpenAIPassthroughTimeoutHeaders(h http.Header) []string {
	if h == nil {
		return nil
	}
	var matched []string
	for key, values := range h {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if isOpenAIPassthroughTimeoutHeader(lowerKey) {
			entry := lowerKey
			if len(values) > 0 {
				entry = fmt.Sprintf("%s=%s", lowerKey, strings.Join(values, "|"))
			}
			matched = append(matched, entry)
		}
	}
	sort.Strings(matched)
	return matched
}

type openaiStreamingResultPassthrough struct {
	usage            *OpenAIUsage
	firstTokenMs     *int
	responseID       string
	imageCount       int
	imageOutputSizes []string
}

type openaiNonStreamingResultPassthrough struct {
	*OpenAIUsage
	usage            *OpenAIUsage
	responseID       string
	imageCount       int
	imageOutputSizes []string
}

const openAIStreamKeepaliveBytesKey = "openai_stream_keepalive_bytes"

func recordOpenAIStreamKeepaliveBytes(c *gin.Context, written int) {
	if c == nil || written <= 0 {
		return
	}
	current := 0
	if value, ok := c.Get(openAIStreamKeepaliveBytesKey); ok {
		current, _ = value.(int)
	}
	c.Set(openAIStreamKeepaliveBytesKey, current+written)
}

func openAIStreamClientOutputStarted(c *gin.Context, localStarted bool) bool {
	if localStarted {
		return true
	}
	if c == nil || c.Writer == nil {
		return false
	}
	// Compact keepalives commit HTTP 200 but are not semantic model output.
	return OpenAICompactKeepaliveAdjustedWrittenSize(c) >= 0
}

func openAIStreamEventIsPreambleTrimmed(eventType string) bool {
	switch eventType {
	case "response.created", "response.in_progress":
		return true
	default:
		return false
	}
}

func openAIStreamEventIsReplaySafeStructureTrimmed(eventType string) bool {
	switch eventType {
	case "response.output_item.added",
		"response.output_item.done",
		"response.content_part.added",
		"response.content_part.done",
		"response.reasoning_summary_part.added",
		"response.reasoning_summary_part.done":
		return true
	default:
		return false
	}
}

var (
	openAIStreamDeltaSemanticFields = []string{
		"delta", "text", "arguments", "summary", "content", "audio",
	}
	openAIStreamToolDoneSemanticFields  = []string{"arguments", "delta", "input"}
	openAIStreamStructureSemanticFields = []string{
		"delta", "text", "arguments", "summary", "content", "audio", "encrypted_content", "result",
		"item.arguments", "item.text", "item.content", "item.summary", "item.encrypted_content", "item.result",
	}
)

func openAIStreamJSONValueHasSemanticValue(value gjson.Result) bool {
	if !value.Exists() {
		return false
	}
	switch value.Type {
	case gjson.String:
		return strings.TrimSpace(value.String()) != ""
	case gjson.Number, gjson.True:
		return true
	case gjson.JSON:
		raw := strings.TrimSpace(value.Raw)
		return raw != "" && raw != "null" && raw != "[]" && raw != "{}"
	default:
		return false
	}
}

func openAIStreamDataHasSemanticField(data string, path string) bool {
	return openAIStreamJSONValueHasSemanticValue(gjson.Get(data, path))
}

func openAIStreamDataHasAnySemanticField(data string, paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	if len(paths) == 1 {
		return openAIStreamDataHasSemanticField(data, paths[0])
	}
	for _, path := range paths {
		value := gjson.Get(data, path)
		if openAIStreamJSONValueHasSemanticValue(value) {
			return true
		}
	}
	return false
}

func openAIStreamStructureDataHasSemanticOutput(data, eventType string) bool {
	switch eventType {
	case "response.output_item.added", "response.output_item.done":
		// The common empty message preamble is structurally replay-safe. Keep a
		// compact-wire fast path; non-compact or richer items use the validated
		// structural inspection below.
		if strings.Contains(data, `"item":{"type":"message","content":[]}`) {
			return false
		}
		item := gjson.Get(data, "item")
		if !item.Exists() {
			return openAIStreamDataHasAnySemanticField(data, openAIStreamStructureSemanticFields)
		}
		itemType := strings.ToLower(strings.TrimSpace(item.Get("type").String()))
		if openAIStreamItemTypeIsReplayUnsafe(itemType) {
			return true
		}
		switch itemType {
		case "message":
			return openAIStreamJSONValueHasSemanticValue(item.Get("content")) ||
				openAIStreamJSONValueHasSemanticValue(item.Get("text"))
		case "reasoning":
			return openAIStreamJSONValueHasSemanticValue(item.Get("summary")) ||
				openAIStreamJSONValueHasSemanticValue(item.Get("encrypted_content")) ||
				openAIStreamJSONValueHasSemanticValue(item.Get("content"))
		}
		return openAIStreamJSONValueHasSemanticValue(item.Get("arguments")) ||
			openAIStreamJSONValueHasSemanticValue(item.Get("result")) ||
			openAIStreamJSONValueHasSemanticValue(item.Get("content"))
	case "response.content_part.added", "response.content_part.done":
		part := gjson.Get(data, "part")
		if part.Exists() {
			return openAIStreamJSONValueHasSemanticValue(part.Get("text")) ||
				openAIStreamJSONValueHasSemanticValue(part.Get("content")) ||
				openAIStreamJSONValueHasSemanticValue(part.Get("audio")) ||
				openAIStreamJSONValueHasSemanticValue(part.Get("transcript"))
		}
		return openAIStreamDataHasAnySemanticField(data, openAIStreamStructureSemanticFields)
	case "response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		part := gjson.Get(data, "part")
		if part.Exists() {
			return openAIStreamJSONValueHasSemanticValue(part.Get("text")) ||
				openAIStreamJSONValueHasSemanticValue(part.Get("summary")) ||
				openAIStreamJSONValueHasSemanticValue(part.Get("content"))
		}
		return openAIStreamDataHasAnySemanticField(data, openAIStreamStructureSemanticFields)
	default:
		return openAIStreamDataHasAnySemanticField(data, openAIStreamStructureSemanticFields)
	}
}

func openAIStreamItemTypeIsReplayUnsafe(itemType string) bool {
	return strings.Contains(itemType, "function_call") ||
		strings.Contains(itemType, "tool_call") ||
		strings.Contains(itemType, "computer_call") ||
		strings.Contains(itemType, "custom_tool")
}

func openAIStreamItemIsReplayUnsafe(data string) bool {
	itemType := strings.ToLower(strings.TrimSpace(gjson.Get(data, "item.type").String()))
	return openAIStreamItemTypeIsReplayUnsafe(itemType)
}

func openAIStreamItemHasVisibleOutput(item gjson.Result) bool {
	if openAIStreamJSONValueHasSemanticValue(item.Get("arguments")) ||
		openAIStreamJSONValueHasSemanticValue(item.Get("input")) ||
		openAIStreamJSONValueHasSemanticValue(item.Get("result")) {
		return true
	}
	for _, path := range []string{"content", "summary"} {
		for _, part := range item.Get(path).Array() {
			if openAIStreamJSONValueHasSemanticValue(part.Get("text")) ||
				openAIStreamJSONValueHasSemanticValue(part.Get("transcript")) {
				return true
			}
		}
	}
	return false
}

// Output progress and replay safety remain separate from TTFT. This predicate
// records latency only when a client-usable value is present.
func openAIStreamDataStartsVisibleOutput(data, eventType string) bool {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" || trimmed == "[DONE]" {
		return false
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" {
		eventType = strings.TrimSpace(gjson.Get(trimmed, "type").String())
	}
	if strings.HasSuffix(eventType, ".delta") {
		return openAIStreamJSONValueHasSemanticValue(gjson.Get(trimmed, "delta"))
	}
	switch eventType {
	case "response.output_text.done",
		"response.reasoning_summary_text.done",
		"response.reasoning_text.done",
		"response.audio_transcript.done":
		return openAIStreamJSONValueHasSemanticValue(gjson.Get(trimmed, "text"))
	case "response.function_call_arguments.done":
		return openAIStreamJSONValueHasSemanticValue(gjson.Get(trimmed, "arguments"))
	case "response.custom_tool_call_input.done":
		return openAIStreamJSONValueHasSemanticValue(gjson.Get(trimmed, "input"))
	case "response.image_generation_call.partial_image":
		return openAIStreamJSONValueHasSemanticValue(gjson.Get(trimmed, "partial_image_b64"))
	case "response.content_part.added", "response.content_part.done",
		"response.reasoning_summary_part.added", "response.reasoning_summary_part.done":
		part := gjson.Get(trimmed, "part")
		return openAIStreamJSONValueHasSemanticValue(part.Get("text")) ||
			openAIStreamJSONValueHasSemanticValue(part.Get("transcript"))
	case "response.output_item.added", "response.output_item.done":
		return openAIStreamItemHasVisibleOutput(gjson.Get(trimmed, "item"))
	case "response.completed", "response.done":
		for _, item := range gjson.Get(trimmed, "response.output").Array() {
			if openAIStreamItemHasVisibleOutput(item) {
				return true
			}
		}
	}
	return false
}

func openAIStreamDataStartsClientOutput(data, eventType string) bool {
	return openAIStreamDataStartsClientOutputTrimmed(strings.TrimSpace(data), strings.TrimSpace(eventType))
}

func openAIStreamDataStartsClientOutputTrimmed(trimmed, eventType string) bool {
	if trimmed == "" {
		return false
	}
	if trimmed == "[DONE]" {
		return false
	}
	if eventType == "response.failed" || eventType == "error" || openAIStreamEventTypeIsTerminal(eventType) {
		return false
	}
	if openAIStreamEventIsPreambleTrimmed(eventType) {
		return false
	}
	if strings.Contains(eventType, "metadata") {
		return false
	}
	if openAIStreamEventIsReplaySafeStructureTrimmed(eventType) {
		return openAIStreamStructureDataHasSemanticOutput(trimmed, eventType)
	}

	if strings.Contains(eventType, ".delta") {
		// These are the overwhelmingly common Responses events. A non-empty
		// delta is enough to classify semantic output; event-boundary flushing
		// performs the separate malformed-payload check when the value is empty.
		switch eventType {
		case "response.output_text.delta", "response.function_call_arguments.delta",
			"response.reasoning_summary_text.delta", "response.reasoning_content.delta",
			"response.audio.delta":
			return openAIStreamDataHasSemanticField(trimmed, "delta")
		default:
			return openAIStreamDataHasAnySemanticField(trimmed, openAIStreamDeltaSemanticFields)
		}
	}

	if strings.HasSuffix(eventType, ".done") {
		// output_item.done may carry a complete tool call with an empty
		// arguments string. Its item identity is already irreversible output.
		if openAIStreamItemIsReplayUnsafe(trimmed) {
			return true
		}
		if strings.Contains(eventType, "function_call") || strings.Contains(eventType, "tool_call") || strings.Contains(eventType, "computer_call") {
			return openAIStreamDataHasAnySemanticField(trimmed, openAIStreamToolDoneSemanticFields) ||
				strings.TrimSpace(gjson.Get(trimmed, "name").String()) != "" ||
				strings.TrimSpace(gjson.Get(trimmed, "call_id").String()) != ""
		}
		return openAIStreamDataHasAnySemanticField(trimmed, openAIStreamStructureSemanticFields)
	}

	// Unknown event families are conservatively treated as semantic output.
	// This keeps newly introduced upstream event types fail-closed without a
	// validation scan on the hot path.
	return true
}

// openAIStreamDataSignalsOutputProgressTrimmed preserves the historical distinction
// between a pure response.created/in_progress preamble and a stream that has
// entered an output phase. Progress alone is still replay-safe for an explicit
// response.failed capacity signal, but an ambiguous transport EOF must retain
// the existing incomplete-stream behavior.
func openAIStreamDataSignalsOutputProgressTrimmed(trimmed, eventType string) bool {
	if trimmed == "" || trimmed == "[DONE]" {
		return false
	}
	if eventType == "response.failed" || eventType == "error" || openAIStreamEventTypeIsTerminal(eventType) {
		return false
	}
	return !openAIStreamEventIsPreambleTrimmed(eventType)
}

func openAIStreamEventNeedsFlushKnownValidity(trimmed, eventType string, semanticOutput, forceFailureBoundary, payloadValid bool) bool {
	if semanticOutput || forceFailureBoundary {
		return true
	}
	if trimmed == "" {
		return false
	}
	if trimmed == "[DONE]" || openAIStreamEventTypeIsTerminal(eventType) {
		return true
	}
	if eventType == "error" {
		return true
	}
	// A complete but malformed/unknown SSE data event cannot safely remain
	// hidden behind a later event's flush.
	return !payloadValid
}

func openAIStreamFailedEventErrorCode(payload []byte) string {
	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.code").String()))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.code").String()))
	}
	return code
}

func isOpenAIUpstreamCapacityShedEvent(payload []byte) bool {
	switch openAIStreamFailedEventErrorCode(payload) {
	case "server_is_overloaded", "slow_down":
		return true
	}
	for _, path := range []string{"response.error.message", "error.message", "message"} {
		if isOpenAICapacityShedMessage(gjson.GetBytes(payload, path).String()) {
			return true
		}
	}
	return false
}

func logOpenAICapacityFailoverSuppressed(
	ctx context.Context,
	account *Account,
	path string,
	upstreamRequestID string,
	eventType string,
) {
	fields := []zap.Field{
		zap.String("path", path),
		zap.String("event_type", strings.TrimSpace(eventType)),
		zap.String("upstream_request_id", strings.TrimSpace(upstreamRequestID)),
	}
	if account != nil {
		fields = append(fields,
			zap.Int64("account_id", account.ID),
			zap.String("platform", account.Platform),
		)
	}
	logger.FromContext(ctx).Warn("gateway.failover_suppressed_after_semantic_output", fields...)
}

const openAICapacityShedRetryableClientCode = "server_error"

// sanitizeOpenAICapacityShedErrorCodeForClient only changes the copy sent to
// the client. Account health and failover classification continue to use the
// original upstream payload.
func sanitizeOpenAICapacityShedErrorCodeForClient(payload []byte) ([]byte, bool) {
	if len(payload) == 0 || !gjson.ValidBytes(payload) || !isOpenAIUpstreamCapacityShedEvent(payload) {
		return payload, false
	}
	updated := payload
	changed := false
	for _, path := range []string{"response.error.code", "error.code"} {
		parent := strings.TrimSuffix(path, ".code")
		if !gjson.GetBytes(updated, parent).Exists() {
			continue
		}
		code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(updated, path).String()))
		if code != "" && code != "server_is_overloaded" && code != "slow_down" {
			continue
		}
		next, err := sjson.SetBytes(updated, path, openAICapacityShedRetryableClientCode)
		if err != nil {
			return payload, false
		}
		updated = next
		changed = true
	}
	return updated, changed
}

func openAIStreamFailedEventSemanticStatus(payload []byte, message string) int {
	if isOpenAIContextWindowError(message, payload) {
		return http.StatusBadRequest
	}

	code := openAIStreamFailedEventErrorCode(payload)
	errType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.type").String()))
	if errType == "" {
		errType = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.type").String()))
	}
	combined := strings.TrimSpace(errType + " " + code + " " + strings.ToLower(strings.TrimSpace(message)))
	switch {
	case strings.Contains(combined, "rate_limit"):
		return http.StatusTooManyRequests
	case strings.Contains(errType, "invalid_request"):
		return http.StatusBadRequest
	case strings.Contains(combined, "authentication") || strings.Contains(combined, "unauthorized") || strings.Contains(combined, "invalid_api_key"):
		return http.StatusUnauthorized
	case strings.Contains(combined, "permission") || strings.Contains(combined, "forbidden") || strings.Contains(combined, "access denied"):
		return http.StatusForbidden
	case isOpenAIUpstreamCapacityShedEvent(payload):
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadGateway
	}
}

func openAIStreamFailureStatus(payload []byte, message string) int {
	if len(bytes.TrimSpace(payload)) == 0 || !gjson.ValidBytes(payload) {
		return http.StatusBadGateway
	}
	if openAIStreamFailedEventSemanticStatus(payload, message) == http.StatusTooManyRequests {
		return http.StatusTooManyRequests
	}
	return http.StatusBadGateway
}

func openAIStreamFailedEventPassthroughBody(payload []byte, failedMessage string) []byte {
	if len(payload) == 0 || !gjson.ValidBytes(payload) {
		return payload
	}
	if gjson.GetBytes(payload, "error").Exists() {
		return payload
	}
	responseError := gjson.GetBytes(payload, "response.error")
	if !responseError.Exists() {
		if strings.TrimSpace(failedMessage) == "" {
			return payload
		}
		body, err := marshalOpenAIUpstreamJSON(gin.H{
			"error": gin.H{
				"message": failedMessage,
			},
		})
		if err != nil {
			return payload
		}
		return body
	}

	errorPayload := gin.H{}
	if errType := strings.TrimSpace(gjson.Get(responseError.Raw, "type").String()); errType != "" {
		errorPayload["type"] = errType
	}
	if code := strings.TrimSpace(gjson.Get(responseError.Raw, "code").String()); code != "" {
		errorPayload["code"] = code
	}
	if param := strings.TrimSpace(gjson.Get(responseError.Raw, "param").String()); param != "" {
		errorPayload["param"] = param
	}
	message := strings.TrimSpace(gjson.Get(responseError.Raw, "message").String())
	if message == "" {
		message = strings.TrimSpace(failedMessage)
	}
	if message != "" {
		errorPayload["message"] = message
	}
	if len(errorPayload) == 0 {
		return payload
	}
	body, err := marshalOpenAIUpstreamJSON(gin.H{"error": errorPayload})
	if err != nil {
		return payload
	}
	return body
}

// applyOpenAIStreamFailedErrorPassthroughRule 对 response.failed 事件应用错误透传规则：
// 归一化 body 供关键词匹配/消息提取，并推断语义状态码使按错误码配置的规则可以命中。
// platform 必须传 account.Platform——本服务同时承载 openai 与 grok 平台账号，规则按平台匹配。
func applyOpenAIStreamFailedErrorPassthroughRule(
	c *gin.Context,
	platform string,
	payload []byte,
	failedMessage string,
) (status int, errType string, errMsg string, matched bool) {
	ruleBody := openAIStreamFailedEventPassthroughBody(payload, failedMessage)
	upstreamStatus := openAIStreamFailedEventSemanticStatus(payload, failedMessage)
	return applyErrorPassthroughRule(
		c,
		platform,
		upstreamStatus,
		ruleBody,
		http.StatusBadGateway,
		"upstream_error",
		"Upstream request failed",
	)
}

// handleOpenAIResponseFailedBeforeOutput applies the one ordering rule shared
// by native Responses and passthrough Responses paths: a retryable semantic
// failure must be classified before an error-passthrough rule is allowed to
// commit a downstream response.  A passthrough rule can still handle a
// non-retryable failure, preserving the existing mapped status/message.
//
// The bool reports that the caller must stop processing the upstream event.  A
// nil error with handled=true is not expected; keeping the pair makes the
// control flow explicit at each streaming and buffered-response call site.
func (s *OpenAIGatewayService) handleOpenAIResponseFailedBeforeOutput(
	c *gin.Context,
	account *Account,
	passthrough bool,
	upstreamRequestID string,
	payload []byte,
	message string,
	responseHeaders http.Header,
) (error, bool) {
	if account != nil && openAIResponseFailureShouldFailover(payload, message) {
		return s.newOpenAIStreamFailoverError(
			c, account, passthrough, upstreamRequestID, payload, message, responseHeaders,
		), true
	}

	platform := PlatformOpenAI
	if account != nil {
		platform = account.Platform
	}
	if status, errType, errMsg, matched := applyOpenAIStreamFailedErrorPassthroughRule(
		c, platform, payload, message,
	); matched {
		if errMsg == "" {
			errMsg = message
		}
		s.recordOpenAIStreamUpstreamError(c, account, passthrough, upstreamRequestID, "http_error", payload, errMsg)
		if c != nil && c.Writer != nil {
			MarkResponseCommitted(c)
			c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
			c.JSON(status, gin.H{
				"error": gin.H{
					"type":    errType,
					"message": errMsg,
				},
			})
		}
		return fmt.Errorf("upstream response failed: passthrough rule matched message=%s", errMsg), true
	}
	return nil, false
}

func openAIStreamFailedEventShouldFailover(payload []byte, message string) bool {
	if isOpenAIContextWindowError(message, payload) {
		return false
	}
	if openAIStreamFailureStatus(payload, message) == http.StatusTooManyRequests {
		return true
	}
	if isOpenAITransientProcessingError(http.StatusBadRequest, message, payload) {
		return true
	}
	code := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.code").String()))
	if code == "" {
		code = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.code").String()))
	}
	errType := strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "response.error.type").String()))
	if errType == "" {
		errType = strings.ToLower(strings.TrimSpace(gjson.GetBytes(payload, "error.type").String()))
	}
	combined := strings.ToLower(strings.TrimSpace(message + " " + code + " " + errType))
	if combined == "" {
		return true
	}
	nonRetryableMarkers := []string{
		"invalid_request",
		"content_policy",
		"policy",
		"safety",
		"high-risk cyber",
		"not allowed",
		"violat",
	}
	for _, marker := range nonRetryableMarkers {
		if strings.Contains(combined, marker) {
			return false
		}
	}
	return true
}

func openAIStreamFailureIsExplicitlyRetryable(payload []byte, message string) bool {
	if openAIStreamFailureStatus(payload, message) == http.StatusTooManyRequests {
		return true
	}
	if isOpenAIUpstreamCapacityShedEvent(payload) {
		return true
	}
	return isOpenAITransientProcessingError(http.StatusBadRequest, message, payload)
}

func openAIResponseFailureShouldFailover(payload []byte, message string) bool {
	if strings.EqualFold(strings.TrimSpace(gjson.GetBytes(payload, "type").String()), "error") {
		// A generic type:error event is used by several compatible upstreams as
		// ordinary stream data. Only explicit transient/capacity signals in this
		// envelope are safe to reinterpret as an account failover.
		return openAIStreamFailureIsExplicitlyRetryable(payload, message)
	}
	return openAIStreamFailedEventShouldFailover(payload, message)
}

func openAIStreamFailedEventRetryableOnSameAccount(account *Account, payload []byte, message string) bool {
	if account == nil {
		return false
	}
	if isOpenAIUpstreamCapacityShedEvent(payload) {
		return true
	}
	if !account.IsPoolMode() {
		return false
	}
	semanticStatus := openAIStreamFailedEventSemanticStatus(payload, message)
	return account.IsPoolModeRetryableStatus(semanticStatus) ||
		isOpenAITransientProcessingError(http.StatusBadRequest, message, payload)
}

func (s *OpenAIGatewayService) recordOpenAIStreamUpstreamError(
	c *gin.Context,
	account *Account,
	passthrough bool,
	upstreamRequestID string,
	kind string,
	payload []byte,
	message string,
) string {
	message = sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	if message == "" {
		message = "OpenAI upstream response failed"
	}
	statusCode := openAIStreamFailureStatus(payload, message)
	detail := ""
	if len(payload) > 0 && s != nil && s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		detail = truncateString(string(payload), maxBytes)
	}
	if c != nil {
		setOpsUpstreamError(c, statusCode, message, detail)
		event := OpsUpstreamErrorEvent{
			Platform:           PlatformOpenAI,
			UpstreamStatusCode: statusCode,
			UpstreamRequestID:  strings.TrimSpace(upstreamRequestID),
			Passthrough:        passthrough,
			Kind:               kind,
			Message:            message,
			Detail:             detail,
		}
		if account != nil {
			event.Platform = account.Platform
			event.AccountID = account.ID
			event.AccountName = account.Name
		}
		appendOpsUpstreamError(c, event)
	}
	return message
}

func (s *OpenAIGatewayService) newOpenAIStreamFailoverError(
	c *gin.Context,
	account *Account,
	passthrough bool,
	upstreamRequestID string,
	payload []byte,
	message string,
	responseHeaders ...http.Header,
) *UpstreamFailoverError {
	message = sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	if message == "" {
		message = "OpenAI stream disconnected before completion"
	}
	statusCode := openAIStreamFailureStatus(payload, message)
	var headers http.Header
	if len(responseHeaders) > 0 && responseHeaders[0] != nil {
		headers = responseHeaders[0].Clone()
	}
	message = s.recordOpenAIStreamUpstreamError(c, account, passthrough, upstreamRequestID, "failover", payload, message)
	errType := "upstream_error"
	if statusCode == http.StatusTooManyRequests {
		errType = "rate_limit_error"
	}
	body, _ := json.Marshal(gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
	failoverErr := &UpstreamFailoverError{
		StatusCode:             statusCode,
		ResponseBody:           body,
		ResponseHeaders:        headers,
		RetryableOnSameAccount: openAIStreamFailedEventRetryableOnSameAccount(account, payload, message),
	}
	if isOpenAIUpstreamCapacityShedEvent(payload) {
		failoverErr.Scope = GatewayFailureScopeRequest
	}
	return failoverErr
}

func (s *OpenAIGatewayService) newOpenAIStreamClientError(
	c *gin.Context,
	account *Account,
	upstreamRequestID string,
	statusCode int,
	errType string,
	message string,
) *UpstreamFailoverError {
	message = sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	if message == "" {
		message = "OpenAI request failed"
	}
	errType = strings.TrimSpace(errType)
	if errType == "" {
		errType = "invalid_request_error"
	}
	if statusCode < 400 || statusCode >= 500 {
		statusCode = http.StatusBadRequest
	}
	message = s.recordOpenAIStreamUpstreamError(c, account, false, upstreamRequestID, "client_error", nil, message)
	fields := []zap.Field{
		zap.String("upstream_request_id", strings.TrimSpace(upstreamRequestID)),
		zap.Int("status_code", statusCode),
		zap.String("error_type", errType),
		zap.String("message", message),
		zap.Bool("retryable_on_same_account", false),
	}
	if c != nil && c.Request != nil {
		if requestID, _ := c.Request.Context().Value(ctxkey.RequestID).(string); strings.TrimSpace(requestID) != "" {
			fields = append(fields, zap.String("request_id", strings.TrimSpace(requestID)))
		}
		if clientRequestID, _ := c.Request.Context().Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(clientRequestID) != "" {
			fields = append(fields, zap.String("client_request_id", strings.TrimSpace(clientRequestID)))
		}
	}
	if account != nil {
		fields = append(fields,
			zap.Int64("account_id", account.ID),
			zap.String("account_platform", account.Platform),
		)
	}
	logger.L().Warn("openai_messages.stream_client_error", fields...)
	body, _ := json.Marshal(gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
	return &UpstreamFailoverError{
		StatusCode:             statusCode,
		ResponseBody:           body,
		RetryableOnSameAccount: false,
	}
}

func (s *OpenAIGatewayService) newRetryableOpenAIStreamFailoverError(
	c *gin.Context,
	account *Account,
	passthrough bool,
	upstreamRequestID string,
	payload []byte,
	message string,
) *UpstreamFailoverError {
	err := s.newOpenAIStreamFailoverError(c, account, passthrough, upstreamRequestID, payload, message)
	markOpenAIPassthroughTransportRetry(err)
	return err
}

func (s *OpenAIGatewayService) handleStreamingResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	startTime time.Time,
	originalModel string,
	mappedModel string,
	reasoningEfforts ...string,
) (*openaiStreamingResultPassthrough, error) {
	visibleOutputTTFT := s.useOpenAIVisibleOutputTTFT(ctx)
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}
	preservePreOutputForFailover := account != nil && account.Platform == PlatformOpenAI
	var attemptResponseHeaders http.Header
	if preservePreOutputForFailover {
		attemptResponseHeaders = make(http.Header)
		writeOpenAIPassthroughResponseHeaders(attemptResponseHeaders, resp.Header, s.responseHeaderFilter)
	} else {
		writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	}

	// SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	if v := resp.Header.Get("x-request-id"); v != "" {
		if preservePreOutputForFailover {
			attemptResponseHeaders.Set("x-request-id", v)
		} else {
			c.Header("x-request-id", v)
		}
	}

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("streaming not supported")
	}

	usage := &OpenAIUsage{}
	imageCounter := newOpenAIImageOutputCounter()
	var firstTokenMs *int
	visibleOutputObserved := false
	responseID := ""
	clientDisconnected := false
	sawDone := false
	sawTerminalEvent := false
	sawFailedEvent := false
	responsesSemanticOutputSeen := false
	capacityFailoverSuppressedLogged := false
	failedMessage := ""
	clientOutputStarted := false
	sawOutputProgressEvent := false
	upstreamRequestID := strings.TrimSpace(resp.Header.Get("x-request-id"))
	reasoningEffort := ""
	if len(reasoningEfforts) > 0 {
		reasoningEffort = strings.TrimSpace(reasoningEfforts[0])
	}
	firstOutputTimeout := time.Duration(0)
	if account != nil && account.Platform == PlatformOpenAI {
		firstOutputTimeout = s.openAIFirstOutputTimeoutWithContext(ctx, reasoningEffort)
	}
	var firstOutputTimer *time.Timer
	var firstOutputCh <-chan time.Time
	if firstOutputTimeout > 0 {
		remaining := time.Until(startTime.Add(firstOutputTimeout))
		if remaining <= 0 {
			remaining = time.Nanosecond
		}
		firstOutputTimer = time.NewTimer(remaining)
		firstOutputCh = firstOutputTimer.C
	}
	stopFirstOutputWatchdog := func() {
		if firstOutputTimer == nil {
			return
		}
		if !firstOutputTimer.Stop() {
			select {
			case <-firstOutputTimer.C:
			default:
			}
		}
		firstOutputTimer = nil
		firstOutputCh = nil
	}
	defer stopFirstOutputWatchdog()

	keepaliveInterval := time.Duration(0)
	keepaliveInterval = s.openAIStreamKeepaliveIntervalWithContext(ctx)
	var keepaliveTicker *time.Ticker
	var keepaliveCh <-chan time.Time
	if keepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(keepaliveInterval)
		keepaliveCh = keepaliveTicker.C
		defer keepaliveTicker.Stop()
	}
	lastDownstreamWriteAt := time.Now()
	downstreamEventInProgress := false
	// pendingLines 在首个可见输出前保留前导事件，确保无输出失败仍可安全 failover。
	var preOutputStage *openAIFirstOutputStage
	if preservePreOutputForFailover {
		stage := newDefaultOpenAIFirstOutputStage()
		preOutputStage = stage
		defer func() {
			if err := stage.Close(); err != nil {
				logger.LegacyPrintf("service.openai_gateway", "OpenAI passthrough first-output staging cleanup failed: account=%d error=%v", account.ID, err)
			}
		}()
	}
	pendingLines := make([]string, 0, 16)
	pendingBytes := 0
	pendingOutputBytes := func() int64 {
		if preOutputStage != nil {
			return preOutputStage.Buffered()
		}
		return int64(pendingBytes)
	}
	turnStateNoted := false
	applyAttemptResponseHeaders := func() {
		if !preservePreOutputForFailover || len(attemptResponseHeaders) == 0 || c.Writer.Written() {
			return
		}
		for key, values := range attemptResponseHeaders {
			c.Writer.Header().Del(key)
			for _, value := range values {
				c.Writer.Header().Add(key, value)
			}
		}
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
	}
	noteTurnStateCommitted := func() {
		if turnStateNoted || extractOpenAICodexTurnState(resp.Header) == "" {
			return
		}
		s.noteStagedOpenAICodexTurnStateCommitted(c, account, resp.Header)
		turnStateNoted = true
	}
	// flushPending 表示已写入但未到 SSE 空行边界的脏状态；defer 兜底函数退出前的残留，断连后不再 Flush。
	flushPending := false
	flushPendingOutput := func() {
		if clientDisconnected || !flushPending {
			return
		}
		flusher.Flush()
		lastDownstreamWriteAt = time.Now()
		flushPending = false
	}
	defer flushPendingOutput()
	writePendingLines := func() bool {
		applyAttemptResponseHeaders()
		if preOutputStage != nil {
			if preOutputStage.Buffered() == 0 {
				return true
			}
			if err := preOutputStage.CommitTo(w); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] Client disconnected during staged output commit: account=%d error=%v", account.ID, err)
				return false
			}
			preOutputStage = nil
			noteTurnStateCommitted()
			return true
		}
		pendingEndedEvent := len(pendingLines) == 0 || pendingLines[len(pendingLines)-1] == ""
		for _, pending := range pendingLines {
			if _, err := fmt.Fprintln(w, pending); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] Client disconnected during streaming, continue draining upstream for usage: account=%d", account.ID)
				return false
			}
		}
		noteTurnStateCommitted()
		pendingLines = pendingLines[:0]
		pendingBytes = 0
		downstreamEventInProgress = !pendingEndedEvent
		return true
	}
	commitPendingOutputProgress := func() {
		if clientDisconnected || clientOutputStarted || !sawOutputProgressEvent || pendingOutputBytes() == 0 {
			return
		}
		if writePendingLines() {
			clientOutputStarted = true
			flushPending = true
			flushPendingOutput()
		}
	}

	scanner := bufio.NewScanner(resp.Body)
	maxLineSize := defaultMaxLineSize
	if s.cfg != nil && s.cfg.Gateway.MaxLineSize > 0 {
		maxLineSize = s.cfg.Gateway.MaxLineSize
	}
	scanBuf := getSSEScannerBuf64K()
	scanner.Buffer(scanBuf[:0], maxLineSize)
	documentScanner := newOpenAISSEJSONDocumentScanner(scanner)
	type passthroughScanEvent struct {
		line      string
		dataValid bool
		err       error
	}
	scanCtx, cancelScan := context.WithCancel(context.Background())
	defer cancelScan()
	events := make(chan passthroughScanEvent, openAIFirstOutputEventQueueSize(firstOutputTimeout > 0))
	go func(scanBuf *sseScannerBuf64K) {
		defer putSSEScannerBuf64K(scanBuf)
		defer close(events)
		for documentScanner.Scan() {
			event := passthroughScanEvent{
				line:      documentScanner.Text(),
				dataValid: documentScanner.DataValid(),
			}
			select {
			case events <- event:
			case <-scanCtx.Done():
				return
			}
		}
		if err := documentScanner.Err(); err != nil {
			select {
			case events <- passthroughScanEvent{err: err}:
			case <-scanCtx.Done():
			}
		}
	}(scanBuf)

	needModelReplace := strings.TrimSpace(originalModel) != "" && strings.TrimSpace(mappedModel) != "" && strings.TrimSpace(originalModel) != strings.TrimSpace(mappedModel)
	resultWithUsage := func() *openaiStreamingResultPassthrough {
		return &openaiStreamingResultPassthrough{
			usage:            usage,
			firstTokenMs:     firstTokenMs,
			responseID:       responseID,
			imageCount:       imageCounter.Count(),
			imageOutputSizes: imageCounter.Sizes(),
		}
	}
	var scanErr error
	streamEnded := false
	for !streamEnded {
		var line string
		dataValid := false
		select {
		case event, ok := <-events:
			if !ok {
				streamEnded = true
				continue
			}
			if event.err != nil {
				scanErr = event.err
				streamEnded = true
				continue
			}
			line = event.line
			dataValid = event.dataValid
		case <-firstOutputCh:
			_ = resp.Body.Close()
			cancelScan()
			return resultWithUsage(), s.newOpenAIFirstOutputTimeoutError(
				ctx, c, account, startTime, originalModel, reasoningEffort,
				firstOutputTimeout, "passthrough_semantic_output", resp.Header,
			)
		case <-keepaliveCh:
			if clientDisconnected || downstreamEventInProgress || time.Since(lastDownstreamWriteAt) < keepaliveInterval {
				continue
			}
			n, writeErr := w.Write([]byte(":\n\n"))
			recordOpenAIStreamKeepaliveBytes(c, n)
			if writeErr != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] Client disconnected during keepalive, continue draining upstream for usage: account=%d", account.ID)
				continue
			}
			flusher.Flush()
			lastDownstreamWriteAt = time.Now()
			continue
		}
		lineStartsClientOutput := false
		lineNeedsFlush := false
		forceFlushFailedEvent := false
		if data, ok := extractOpenAISSEDataLine(line); ok {
			dataBytes := []byte(data)
			trimmedData := strings.TrimSpace(data)
			rawEventType := strings.TrimSpace(gjson.GetBytes(dataBytes, "type").String())
			observer.ObserveOpenAI(dataBytes, rawEventType)
			if needModelReplace && strings.Contains(data, mappedModel) {
				line = s.replaceModelInSSELine(line, mappedModel, originalModel)
				if replacedData, replaced := extractOpenAISSEDataLine(line); replaced {
					dataBytes = []byte(replacedData)
					trimmedData = strings.TrimSpace(replacedData)
				}
			}
			if normalizedData, normalized := normalizeOpenAIResponsesFunctionCallArguments(dataBytes); normalized {
				dataBytes = normalizedData
				trimmedData = strings.TrimSpace(string(normalizedData))
				line = "data: " + string(normalizedData)
			}
			if normalizedData, normalized := normalizeCompletedImageGenerationStatus(dataBytes); normalized {
				dataBytes = normalizedData
				trimmedData = strings.TrimSpace(string(normalizedData))
				line = "data: " + string(normalizedData)
			}
			if trimmedData != "[DONE]" {
				restoredData, restoreErr := restoreOpenAIResponsesNamespacePayload(c, dataBytes)
				if restoreErr != nil {
					return resultWithUsage(), fmt.Errorf("restore OpenAI passthrough namespace response: %w", restoreErr)
				}
				if !bytes.Equal(restoredData, dataBytes) {
					dataBytes = restoredData
					trimmedData = strings.TrimSpace(string(restoredData))
					line = "data: " + string(restoredData)
				}
			}
			eventType := strings.TrimSpace(gjson.Get(trimmedData, "type").String())
			if openAIStreamDataSignalsOutputProgressTrimmed(trimmedData, eventType) {
				sawOutputProgressEvent = true
			}
			failureEnvelopeMessage := ""
			isFailureEnvelope := eventType == "response.failed"
			if eventType == "error" {
				failureEnvelopeMessage = extractOpenAISSEErrorMessage(dataBytes)
				isFailureEnvelope = openAIStreamFailureIsExplicitlyRetryable(dataBytes, failureEnvelopeMessage)
			}
			if isFailureEnvelope {
				failedMessage = failureEnvelopeMessage
				if failedMessage == "" {
					failedMessage = extractOpenAISSEErrorMessage(dataBytes)
				}
				// response.failed 自带上游已消耗的 usage（input token 通常已扣）；必须先解析
				// 再打 cyber 标记，否则 mark 记到的是解析前的 0，导致流式 cyber 按 0 token 计费
				// 而漏记真实用量。对齐 WS V2 / Chat 流式路径（均先解析 usage 再 Mark）。
				s.parseSSEUsageBytes(dataBytes, usage)
				if hit, code, msg := detectOpenAICyberPolicy(dataBytes); hit {
					MarkOpsCyberPolicy(c, CyberPolicyMark{
						Code:           code,
						Message:        msg,
						Body:           truncateString(string(dataBytes), 4096),
						UpstreamStatus: http.StatusOK,
						UpstreamInTok:  usage.InputTokens,
						UpstreamOutTok: usage.OutputTokens,
					})
				}
				clientHadSemanticOutput := openAIStreamClientOutputStarted(c, clientOutputStarted)
				if !clientHadSemanticOutput {
					if handledErr, handled := s.handleOpenAIResponseFailedBeforeOutput(
						c, account, true, upstreamRequestID, dataBytes, failedMessage, resp.Header,
					); handled {
						if handledErr != nil {
							return resultWithUsage(), handledErr
						}
						return resultWithUsage(), fmt.Errorf("upstream response failed")
					}
				} else if openAIStreamFailureIsExplicitlyRetryable(dataBytes, failedMessage) {
					s.recordOpenAIStreamUpstreamError(c, account, true, upstreamRequestID, "http_error", dataBytes, failedMessage)
					if !capacityFailoverSuppressedLogged && isOpenAIUpstreamCapacityShedEvent(dataBytes) {
						logOpenAICapacityFailoverSuppressed(ctx, account, "passthrough_sse", upstreamRequestID, eventType)
						capacityFailoverSuppressedLogged = true
					}
					accountID := int64(0)
					if account != nil {
						accountID = account.ID
					}
					logger.LegacyPrintf("service.openai_gateway", "OpenAI passthrough response.failed after semantic output; failover skipped: response already committed account=%d request_id=%s", accountID, upstreamRequestID)
				}
				forceFlushFailedEvent = true
				sawFailedEvent = true
				if eventType == "error" {
					sawTerminalEvent = true
				}
			}
			if trimmedData == "[DONE]" {
				sawDone = true
			}
			if openAIStreamEventIsTerminal(trimmedData) {
				sawTerminalEvent = true
				stopFirstOutputWatchdog()
			}
			if responseID == "" {
				responseID = extractOpenAIResponseIDFromJSONBytes(dataBytes)
			}
			imageCounter.AddSSEData(dataBytes)
			if sanitizedData, sanitized := sanitizeOpenAIResponseFailedEventForClient(
				dataBytes,
				eventType,
				openAIStreamClientOutputStarted(c, clientOutputStarted),
			); sanitized {
				dataBytes = sanitizedData
				trimmedData = strings.TrimSpace(string(sanitizedData))
				line = "data: " + string(sanitizedData)
			}
			if !clientOutputStarted || !visibleOutputObserved {
				lineStartsClientOutput = openAIStreamDataStartsClientOutputTrimmed(trimmedData, eventType)
				lineNeedsFlush = openAIStreamEventNeedsFlushKnownValidity(
					trimmedData,
					eventType,
					lineStartsClientOutput,
					forceFlushFailedEvent,
					dataValid,
				)
			} else {
				lineNeedsFlush = forceFlushFailedEvent
			}
			if lineStartsClientOutput && !openAIStreamEventTypeIsTerminal(eventType) {
				responsesSemanticOutputSeen = true
			}
			if account != nil && account.Platform == PlatformOpenAI &&
				(eventType == "response.completed" || eventType == "response.done") &&
				!sawFailedEvent && !responsesSemanticOutputSeen &&
				!openAIStreamClientOutputStarted(c, clientOutputStarted) &&
				openAIResponsesCompletedEventIsEmpty(dataBytes, usage) {
				return resultWithUsage(), newOpenAIResponsesEmptyCompletedFailoverError(c, account, upstreamRequestID)
			}
			if !visibleOutputObserved && openAIStreamDataStartsVisibleOutput(trimmedData, eventType) {
				visibleOutputObserved = true
			}
			if firstTokenMs == nil && openAIStreamDataStartsTTFT(trimmedData, eventType, visibleOutputTTFT) {
				ms := int(time.Since(startTime).Milliseconds())
				firstTokenMs = &ms
			}
			if lineStartsClientOutput {
				stopFirstOutputWatchdog()
			}
			s.parseSSEUsageBytes(dataBytes, usage)
		}

		if !clientDisconnected {
			if !clientOutputStarted && !lineNeedsFlush {
				if preOutputStage != nil {
					incomingBytes := int64(len(line) + 1)
					if incomingBytes > preOutputStage.limit-preOutputStage.Buffered() {
						stopFirstOutputWatchdog()
						failoverErr := s.newOpenAIStreamFailoverError(
							c, account, true, upstreamRequestID, nil,
							"OpenAI passthrough first-output staging limit exceeded",
							resp.Header,
						)
						failoverErr.SafeToFailoverAfterWrite = true
						return resultWithUsage(), failoverErr
					}
					_, writeErr := preOutputStage.WriteString(line)
					if writeErr == nil {
						_, writeErr = preOutputStage.WriteString("\n")
					}
					if writeErr != nil {
						failoverErr := s.newOpenAIStreamFailoverError(
							c, account, true, upstreamRequestID, nil,
							"OpenAI passthrough first-output staging failed",
							resp.Header,
						)
						failoverErr.SafeToFailoverAfterWrite = true
						return resultWithUsage(), failoverErr
					}
					continue
				}
				pendingLines = append(pendingLines, line)
				pendingBytes += len(line) + 1
				if pendingBytes >= openAIPassthroughPreOutputBufferLimit {
					stopFirstOutputWatchdog()
					if writePendingLines() {
						clientOutputStarted = true
						flusher.Flush()
						lastDownstreamWriteAt = time.Now()
					}
				}
				continue
			}
			if !clientOutputStarted && pendingOutputBytes() > 0 {
				if !writePendingLines() {
					continue
				}
			}
			applyAttemptResponseHeaders()
			if _, err := fmt.Fprintln(w, line); err != nil {
				clientDisconnected = true
				logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] Client disconnected during streaming, continue draining upstream for usage: account=%d", account.ID)
			} else {
				if !clientOutputStarted {
					noteTurnStateCommitted()
				}
				clientOutputStarted = true
				downstreamEventInProgress = line != ""
				flushPending = true
				if line == "" {
					flushPendingOutput()
				}
			}
		}
	}
	if scanErr != nil {
		err := scanErr
		if (sawDone || sawTerminalEvent) && !sawFailedEvent {
			s.clearOpenAIProxyStreamDisconnect(account, startTime)
			return resultWithUsage(), nil
		}
		if sawFailedEvent {
			return resultWithUsage(), fmt.Errorf("upstream response failed: %s", failedMessage)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete: %w", ctxErr)
		}
		if errors.Is(err, context.Canceled) {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete: %w", err)
		}
		if errors.Is(err, bufio.ErrTooLong) {
			logger.LegacyPrintf("service.openai_gateway", "[OpenAI passthrough] SSE line too long: account=%d max_size=%d error=%v", account.ID, maxLineSize, err)
			return resultWithUsage(), err
		}
		if !openAIStreamClientOutputStarted(c, clientOutputStarted) && !sawOutputProgressEvent {
			if classifyOpenAITransportError(err).Persistent {
				return resultWithUsage(), s.handleOpenAIUpstreamTransportError(ctx, c, account, err, true)
			}
			msg := "OpenAI stream disconnected before completion"
			if errText := strings.TrimSpace(err.Error()); errText != "" {
				msg += ": " + errText
			}
			return resultWithUsage(),
				s.newRetryableOpenAIStreamFailoverError(c, account, true, upstreamRequestID, nil, msg)
		}
		commitPendingOutputProgress()
		if errors.Is(err, context.DeadlineExceeded) {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete: %w", err)
		}
		if clientDisconnected {
			return resultWithUsage(), fmt.Errorf("stream usage incomplete after disconnect: %w", err)
		}
		s.recordOpenAIProxyStreamDisconnect(account, err, upstreamRequestID, startTime)
		logger.LegacyPrintf("service.openai_gateway",
			"[OpenAI passthrough] 流读取异常中断: account=%d request_id=%s err=%v",
			account.ID,
			upstreamRequestID,
			err,
		)
		return resultWithUsage(), fmt.Errorf("stream read error: %w", err)
	}
	if sawFailedEvent {
		return resultWithUsage(), fmt.Errorf("upstream response failed: %s", failedMessage)
	}
	if !clientDisconnected && !sawDone && !sawTerminalEvent && ctx.Err() == nil {
		logger.FromContext(ctx).With(
			zap.String("component", "service.openai_gateway"),
			zap.Int64("account_id", account.ID),
			zap.String("upstream_request_id", upstreamRequestID),
		).Info("OpenAI passthrough 上游流在未收到 [DONE] 时结束，疑似断流")
		if !openAIStreamClientOutputStarted(c, clientOutputStarted) && !sawOutputProgressEvent {
			return resultWithUsage(),
				s.newRetryableOpenAIStreamFailoverError(c, account, true, upstreamRequestID, nil, "OpenAI stream ended before a terminal event")
		}
		commitPendingOutputProgress()
		s.recordOpenAIProxyStreamDisconnect(account, errors.New("stream ended before terminal event"), upstreamRequestID, startTime)
		return resultWithUsage(), errors.New("stream usage incomplete: missing terminal event")
	}
	if (sawDone || sawTerminalEvent) && !sawFailedEvent {
		s.clearOpenAIProxyStreamDisconnect(account, startTime)
	}

	return resultWithUsage(), nil
}

func (s *OpenAIGatewayService) handleNonStreamingResponsePassthrough(
	ctx context.Context,
	resp *http.Response,
	c *gin.Context,
	account *Account,
	originalModel string,
	mappedModel string,
) (*openaiNonStreamingResultPassthrough, error) {
	body, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		if errors.Is(err, ErrUpstreamResponseBodyTooLarge) {
			return nil, err
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if errors.Is(err, context.Canceled) {
			return nil, err
		}
		failoverErr := s.handleOpenAIUpstreamTransportError(ctx, c, account, err, true)
		if typed, ok := failoverErr.(*UpstreamFailoverError); ok && !classifyOpenAITransportError(err).Persistent {
			markOpenAIPassthroughTransportRetry(typed)
		}
		return nil, failoverErr
	}
	observer := upstreamResponseModelObserverFromContext(c)
	if observer == nil {
		observer = beginUpstreamResponseModelObservation(c)
	}
	if bodyHasSSEFraming(body) {
		observeOpenAISSEBody(observer, string(body))
	} else {
		observer.ObserveOpenAI(body, strings.TrimSpace(gjson.GetBytes(body, "type").String()))
	}

	// Detect SSE responses from upstream and convert to JSON.
	// Some upstreams (e.g. other sub2api instances) may return SSE even when
	// stream=false was requested. Without this conversion the client would
	// receive raw SSE text or a terminal event with empty output.
	if isEventStreamResponse(resp.Header) || bodyHasSSEFraming(body) {
		return s.handlePassthroughSSEToJSONWithAccount(resp, c, body, originalModel, mappedModel, account)
	}
	if failurePayload, failure := extractOpenAIJSONFailureEnvelope(body); failure {
		return nil, s.handleOpenAINonStreamingFailureEnvelope(resp, c, account, failurePayload, body, true)
	}

	usage := &OpenAIUsage{}
	usageParsed := false
	if len(body) > 0 {
		if parsedUsage, ok := extractOpenAIUsageFromJSONBytes(body); ok {
			*usage = parsedUsage
			usageParsed = true
		}
	}
	if !usageParsed {
		// 兜底：尝试从 SSE 文本中解析 usage
		usage = s.parseSSEUsageFromBody(string(body))
	}

	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	s.relayOpenAICodexTurnState(c, account, resp.Header)

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}
	if originalModel != "" && mappedModel != "" && originalModel != mappedModel {
		body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
	}
	body, err = restoreOpenAIResponsesNamespacePayload(c, body)
	if err != nil {
		return nil, fmt.Errorf("restore OpenAI passthrough namespace response: %w", err)
	}
	if !writeOpenAICompactSSEBridge(c, resp.StatusCode, body) {
		c.Data(resp.StatusCode, contentType, body)
	}
	return &openaiNonStreamingResultPassthrough{
		OpenAIUsage:      usage,
		usage:            usage,
		responseID:       extractOpenAIResponseIDFromJSONBytes(body),
		imageCount:       countOpenAIResponseImageOutputsFromJSONBytes(body),
		imageOutputSizes: collectOpenAIResponseImageOutputSizesFromJSONBytes(body),
	}, nil
}

func (s *OpenAIGatewayService) handlePassthroughSSEToJSONWithAccount(
	resp *http.Response,
	c *gin.Context,
	body []byte,
	originalModel string,
	mappedModel string,
	account *Account,
) (*openaiNonStreamingResultPassthrough, error) {
	bodyText := string(body)
	if failurePayload, failure := extractOpenAISSEFailureEvent(bodyText); failure {
		failedMessage := extractOpenAISSEErrorMessage(failurePayload)
		if failedMessage == "" {
			failedMessage = "Upstream response failed"
		}
		if !openAIResponseFailureHasSemanticOutput(bodyText, failurePayload) {
			if handledErr, handled := s.handleOpenAIResponseFailedBeforeOutput(
				c, account, true, strings.TrimSpace(resp.Header.Get("x-request-id")),
				failurePayload, failedMessage, resp.Header,
			); handled {
				return nil, handledErr
			}
		} else {
			s.recordOpenAIStreamUpstreamError(c, account, true, strings.TrimSpace(resp.Header.Get("x-request-id")), "http_error", failurePayload, failedMessage)
			logger.LegacyPrintf("service.openai_gateway", "OpenAI passthrough non-streaming response.failed after semantic output; failover skipped: response already produced output")
			if openAIStreamFailureIsExplicitlyRetryable(failurePayload, failedMessage) {
				failedMessage = openAIReplayUnsafeTransientFailureMessage
			}
		}
		return nil, s.writeOpenAINonStreamingProtocolError(resp, c, failedMessage)
	}
	finalResponse, ok := extractCodexFinalResponse(bodyText)

	usage := &OpenAIUsage{}
	if ok {
		if parsedUsage, parsed := extractOpenAIUsageFromJSONBytes(finalResponse); parsed {
			*usage = parsedUsage
		}
		// When the terminal event has an empty output array, reconstruct
		// output from accumulated delta events so the client gets full content.
		if len(gjson.GetBytes(finalResponse, "output").Array()) == 0 {
			if outputJSON, reconstructed := reconstructResponseOutputFromSSE(bodyText); reconstructed {
				if patched, err := sjson.SetRawBytes(finalResponse, "output", outputJSON); err == nil {
					finalResponse = patched
				}
			}
		}
		finalResponse = supplementCompactionItemFromSSE(c, finalResponse, bodyText)
		body = finalResponse
		if originalModel != "" && mappedModel != "" && originalModel != mappedModel {
			body = s.replaceModelInResponseBody(body, mappedModel, originalModel)
		}
		// Correct tool calls in final response
		body = s.correctToolCallsInResponseBody(body)
		restoredBody, restoreErr := restoreOpenAIResponsesNamespacePayload(c, body)
		if restoreErr != nil {
			return nil, fmt.Errorf("restore OpenAI passthrough namespace response: %w", restoreErr)
		}
		body = restoredBody
	} else {
		usage = s.parseSSEUsageFromBody(bodyText)
		if originalModel != "" && mappedModel != "" && originalModel != mappedModel {
			bodyText = s.replaceModelInSSEBody(bodyText, mappedModel, originalModel)
		}
		body = []byte(bodyText)
	}

	writeOpenAIPassthroughResponseHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	s.relayOpenAICodexTurnState(c, account, resp.Header)

	contentType := "application/json; charset=utf-8"
	if !ok {
		contentType = resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "text/event-stream"
		}
	}
	if !writeOpenAICompactSSEBridge(c, resp.StatusCode, body) {
		c.Data(resp.StatusCode, contentType, body)
	}

	return &openaiNonStreamingResultPassthrough{
		OpenAIUsage:      usage,
		usage:            usage,
		responseID:       extractOpenAIResponseIDFromJSONBytes(body),
		imageCount:       countOpenAIImageOutputsFromSSEBody(bodyText),
		imageOutputSizes: collectOpenAIImageOutputSizesFromSSEBody(bodyText),
	}, nil
}

func writeOpenAIPassthroughResponseHeaders(dst http.Header, src http.Header, filter *responseheaders.CompiledHeaderFilter) {
	if dst == nil || src == nil {
		return
	}
	if filter != nil {
		responseheaders.WriteFilteredHeaders(dst, src, filter)
	} else {
		// 兜底：尽量保留最基础的 content-type
		if v := strings.TrimSpace(src.Get("Content-Type")); v != "" {
			dst.Set("Content-Type", v)
		}
	}
	// 透传模式强制放行 x-codex-* 响应头（若上游返回）。
	// 注意：真实 http.Response.Header 的 key 一般会被 canonicalize；但为了兼容测试/自建响应，
	// 这里用 EqualFold 做一次大小写不敏感的查找。
	getCaseInsensitiveValues := func(h http.Header, want string) []string {
		if h == nil {
			return nil
		}
		for k, vals := range h {
			if strings.EqualFold(k, want) {
				return vals
			}
		}
		return nil
	}

	for _, rawKey := range []string{
		"x-codex-primary-used-percent",
		"x-codex-primary-reset-after-seconds",
		"x-codex-primary-window-minutes",
		"x-codex-secondary-used-percent",
		"x-codex-secondary-reset-after-seconds",
		"x-codex-secondary-window-minutes",
		"x-codex-primary-over-secondary-limit-percent",
	} {
		vals := getCaseInsensitiveValues(src, rawKey)
		if len(vals) == 0 {
			continue
		}
		key := http.CanonicalHeaderKey(rawKey)
		dst.Del(key)
		for _, v := range vals {
			dst.Add(key, v)
		}
	}

	// Codex clients echo this opaque response header on later requests in the
	// same turn. Clear stale values when the selected upstream omits it.
	turnStateKey := http.CanonicalHeaderKey(openAICodexTurnStateHeader)
	dst.Del(turnStateKey)
	for _, value := range getCaseInsensitiveValues(src, openAICodexTurnStateHeader) {
		dst.Add(turnStateKey, value)
	}
}

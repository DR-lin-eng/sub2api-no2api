package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/shared/claude"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/Wei-Shaw/sub2api/internal/shared/openai_compat"
	"github.com/Wei-Shaw/sub2api/internal/shared/responseheaders"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

var openAIAnthropicCompactChunkTargetChars = 300_000
var openAIAnthropicCompactMergeTargetChars = 180_000

const (
	openAIAnthropicCompactFallbackMaxChunks        = 40
	openAIAnthropicCompactChunkMaxOutputTokens     = 6_000
	openAIAnthropicCompactMergeMaxOutputTokens     = 12_000
	openAIAnthropicCompactEmergencyMaxRunes        = 90_000
	openAIAnthropicCompactFallbackMinSplitRunes    = 4_000
	openAIAnthropicCompactMergeMaxDepth            = 6
	openAIAnthropicCompactFallbackChunkReasoning   = "low"
	openAIAnthropicCompactFallbackFallbackResponse = "Claude Code compact fallback failed before producing a summary"
)

// ForwardAsAnthropic accepts an Anthropic Messages request body, converts it
// to OpenAI Responses API format, forwards to the OpenAI upstream, and converts
// the response back to Anthropic Messages format. This enables Claude Code
// clients to access OpenAI models through the standard /v1/messages endpoint.
func (s *OpenAIGatewayService) ForwardAsAnthropic(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	body []byte,
	promptCacheKey string,
	defaultMappedModel string,
) (*OpenAIForwardResult, error) {
	if err := s.openAIAccountRuntimeBlockedFailover(account); err != nil {
		return nil, err
	}
	// 入口分流：APIKey 账号 + 上游不支持 Responses API → 走 CC 直转（与
	// ForwardAsChatCompletions 对称）。缺少此分流时，/v1/messages 入站请求
	// 会被无条件转为 Responses 格式发往上游 /v1/responses，导致只支持
	// /v1/chat/completions 的第三方 OpenAI 兼容上游全部 400。
	if account.Type == AccountTypeAPIKey && !openai_compat.ShouldUseResponsesAPI(account.Extra) {
		return s.forwardAnthropicViaRawChatCompletions(ctx, c, account, body, defaultMappedModel)
	}

	startTime := time.Now()

	// 1. Parse Anthropic request
	var anthropicReq apicompat.AnthropicRequest
	if err := json.Unmarshal(body, &anthropicReq); err != nil {
		return nil, fmt.Errorf("parse anthropic request: %w", err)
	}
	anthropicDigestReq := cloneAnthropicRequestForDigest(&anthropicReq)
	originalModel := anthropicReq.Model
	applyOpenAICompatModelNormalization(&anthropicReq)
	normalizedModel := anthropicReq.Model
	clientStream := anthropicReq.Stream // client's original stream preference

	// 2. Model mapping
	billingModel := resolveOpenAIForwardModel(account, normalizedModel, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	anthropicCompactRequest := isClaudeCodeCompactAnthropicRequest(&anthropicReq)
	anthropicCompactModelMapped := false
	anthropicCompactRequestedModel := billingModel
	var anthropicCompactFallbackUpstreamModels []string
	if anthropicCompactRequest {
		compactBillingModel := resolveOpenAICompactForwardModel(account, billingModel)
		anthropicCompactModelMapped = compactBillingModel != "" && compactBillingModel != billingModel
		if anthropicCompactModelMapped {
			billingModel = compactBillingModel
			upstreamModel = normalizeOpenAIModelForUpstream(account, billingModel)
			anthropicCompactFallbackUpstreamModels = resolveOpenAICompactFallbackForwardModels(account, anthropicCompactRequestedModel, billingModel)
		}
	}
	promptCacheKey = strings.TrimSpace(promptCacheKey)
	apiKeyID := getAPIKeyIDFromContext(c)
	anthropicDigestChain := ""
	anthropicMatchedDigestChain := ""
	compatPromptCacheInjected := false
	// Grok is outside the gpt-5/codex compat injector, but Claude Code still
	// carries a stable session id. Prefer that as the Grok prompt-cache seed so
	// multi-turn /v1/messages traffic can hit xAI's server-side cache.
	if promptCacheKey == "" && account.Platform == PlatformGrok {
		if sessionSeed := extractClaudeCodeSessionID(c, body); sessionSeed != "" {
			promptCacheKey = sessionSeed
			compatPromptCacheInjected = true
		} else if sessionSeed := promptCacheKeyFromAnthropicMetadataSession(&anthropicReq); sessionSeed != "" {
			promptCacheKey = sessionSeed
			compatPromptCacheInjected = true
		}
	}
	if promptCacheKey == "" && shouldAutoInjectPromptCacheKeyForCompat(upstreamModel) {
		promptCacheKey = promptCacheKeyFromAnthropicMetadataSession(&anthropicReq)
		if promptCacheKey == "" {
			promptCacheKey = deriveAnthropicCacheControlPromptCacheKey(&anthropicReq)
		}
		if promptCacheKey == "" {
			anthropicDigestChain = buildOpenAICompatAnthropicDigestChain(anthropicDigestReq)
			if reusedKey, matchedChain := s.findOpenAICompatAnthropicDigestPromptCacheKey(account, apiKeyID, anthropicDigestChain); reusedKey != "" {
				promptCacheKey = reusedKey
				anthropicMatchedDigestChain = matchedChain
			} else {
				promptCacheKey = promptCacheKeyFromAnthropicDigest(anthropicDigestChain)
			}
		}
		compatPromptCacheInjected = promptCacheKey != ""
	}
	compatReplayTrimmed := false
	compatReplayGuardEnabled := shouldAutoInjectPromptCacheKeyForCompat(upstreamModel)
	compatContinuationEnabled := openAICompatContinuationEnabled(account, upstreamModel)
	previousResponseID := ""
	if compatContinuationEnabled {
		previousResponseID = s.getOpenAICompatSessionResponseID(ctx, c, account, promptCacheKey)
	}
	compatContinuationDisabled := compatContinuationEnabled &&
		s.isOpenAICompatSessionContinuationDisabled(ctx, c, account, promptCacheKey)
	compatTurnState := ""
	// OAuth/Plus relies on session_id + x-codex-turn-state; trimming to a
	// sliding 12-message window makes the cached prefix stall at system/tools.
	// Keep full replay there so upstream prompt caching can grow turn by turn.
	if compatReplayGuardEnabled && account.Type != AccountTypeOAuth && previousResponseID == "" && !compatContinuationDisabled {
		compatReplayTrimmed = applyAnthropicCompatFullReplayGuard(&anthropicReq)
	}

	// 3. Convert Anthropic → Responses after compatibility-only replay guard.
	responsesReq, err := apicompat.AnthropicToResponses(&anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("convert anthropic to responses: %w", err)
	}

	// Upstream always uses streaming (upstream may not support sync mode).
	// The client's original preference determines the response format.
	responsesReq.Stream = true
	isStream := true

	// 3b. Handle BetaFastMode → service_tier: "priority"
	if containsBetaToken(c.GetHeader("anthropic-beta"), claude.BetaFastMode) {
		responsesReq.ServiceTier = "priority"
	}

	responsesReq.Model = upstreamModel
	if previousResponseID != "" {
		responsesReq.PreviousResponseID = previousResponseID
		trimAnthropicCompatResponsesInputToLatestTurn(responsesReq)
	}
	if compatReplayGuardEnabled && account.Type != AccountTypeOAuth {
		appendOpenAICompatClaudeCodeTodoGuard(responsesReq)
	}

	logFields := []zap.Field{
		zap.Int64("account_id", account.ID),
		zap.String("original_model", originalModel),
		zap.String("normalized_model", normalizedModel),
		zap.String("billing_model", billingModel),
		zap.String("upstream_model", upstreamModel),
		zap.Bool("stream", isStream),
	}
	if anthropicCompactRequest {
		logFields = append(logFields,
			zap.Bool("anthropic_compact_request", true),
			zap.Bool("anthropic_compact_model_mapped", anthropicCompactModelMapped),
		)
		if len(anthropicCompactFallbackUpstreamModels) > 0 {
			logFields = append(logFields, zap.Strings("anthropic_compact_fallback_upstream_models", anthropicCompactFallbackUpstreamModels))
		}
	}
	if compatPromptCacheInjected {
		logFields = append(logFields,
			zap.Bool("compat_prompt_cache_key_injected", true),
			zap.String("compat_prompt_cache_key_sha256", hashSensitiveValueForLog(promptCacheKey)),
		)
	}
	if compatReplayTrimmed {
		logFields = append(logFields,
			zap.Bool("compat_full_replay_trimmed", true),
			zap.Int("compat_messages_after_trim", len(anthropicReq.Messages)),
		)
	}
	if previousResponseID != "" {
		logFields = append(logFields,
			zap.Bool("compat_previous_response_id_attached", true),
			zap.String("compat_previous_response_id", truncateOpenAIWSLogValue(previousResponseID, openAIWSIDValueMaxLen)),
		)
	}
	if compatTurnState != "" {
		logFields = append(logFields, zap.Bool("compat_turn_state_attached", true))
	}
	logger.L().Debug("openai messages: model mapping applied", logFields...)

	// 4. Marshal Responses request body, then apply OAuth codex transform
	responsesBody, err := json.Marshal(responsesReq)
	if err != nil {
		return nil, fmt.Errorf("marshal responses request: %w", err)
	}

	if account.Type == AccountTypeOAuth && account.Platform != PlatformGrok {
		var reqBody map[string]any
		if err := json.Unmarshal(responsesBody, &reqBody); err != nil {
			return nil, fmt.Errorf("unmarshal for codex transform: %w", err)
		}
		codexResult := applyCodexOAuthTransformWithOptions(reqBody, codexOAuthTransformOptions{
			SkipDefaultInstructions: true,
			PreserveToolCallIDs:     true,
		})
		forcedTemplateText := ""
		if s.cfg != nil {
			forcedTemplateText = s.cfg.Gateway.ForcedCodexInstructionsTemplate
		}
		templateUpstreamModel := upstreamModel
		if codexResult.NormalizedModel != "" {
			templateUpstreamModel = codexResult.NormalizedModel
		}
		existingInstructions, _ := reqBody["instructions"].(string)
		if strings.TrimSpace(existingInstructions) == "" {
			existingInstructions = extractPromptLikeInstructionsFromInput(reqBody)
		}
		if _, err := applyForcedCodexInstructionsTemplate(reqBody, forcedTemplateText, forcedCodexInstructionsTemplateData{
			ExistingInstructions: strings.TrimSpace(existingInstructions),
			OriginalModel:        originalModel,
			NormalizedModel:      normalizedModel,
			BillingModel:         billingModel,
			UpstreamModel:        templateUpstreamModel,
		}); err != nil {
			return nil, err
		}
		ensureCodexOAuthInstructionsField(reqBody)
		if shouldAutoInjectPromptCacheKeyForCompat(upstreamModel) {
			appendOpenAICompatClaudeCodeTodoGuardToRequestBody(reqBody)
		}
		if codexResult.NormalizedModel != "" {
			upstreamModel = codexResult.NormalizedModel
		}
		if codexResult.PromptCacheKey != "" {
			promptCacheKey = codexResult.PromptCacheKey
		}
		delete(reqBody, "prompt_cache_key")
		if shouldAutoInjectPromptCacheKeyForCompat(upstreamModel) {
			compatTurnState = s.getOpenAICompatSessionTurnState(ctx, c, account, promptCacheKey)
		}
		// OAuth codex transform forces stream=true upstream, so always use
		// the streaming response handler regardless of what the client asked.
		isStream = true
		responsesBody, err = json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("remarshal after codex transform: %w", err)
		}
	}

	// For API key accounts (including OpenAI-compatible upstream gateways),
	// ensure promptCacheKey is also propagated via the request body so that
	// upstreams using the Responses API can derive a stable session identifier
	// from prompt_cache_key. This makes our Anthropic /v1/messages compatibility
	// path behave more like a native Responses client.
	if account.Type == AccountTypeAPIKey {
		if trimmedKey := strings.TrimSpace(promptCacheKey); trimmedKey != "" {
			var reqBody map[string]any
			if err := json.Unmarshal(responsesBody, &reqBody); err != nil {
				return nil, fmt.Errorf("unmarshal for prompt cache key injection: %w", err)
			}
			if existing, ok := reqBody["prompt_cache_key"].(string); !ok || strings.TrimSpace(existing) == "" {
				reqBody["prompt_cache_key"] = trimmedKey
				updated, err := json.Marshal(reqBody)
				if err != nil {
					return nil, fmt.Errorf("remarshal after prompt cache key injection: %w", err)
				}
				responsesBody = updated
			}
		}
	}
	if account.IsOpenAI() {
		responsesBody, _, err = sanitizeOpenAIResponsesInputIDs(responsesBody, account.IsOpenAIOAuth())
		if err != nil {
			return nil, fmt.Errorf("sanitize Responses input ids: %w", err)
		}
	}

	// 4c. Apply OpenAI fast policy (may filter service_tier or block the request).
	// Mirrors the Claude anthropic-beta "fast-mode-2026-02-01" filter, but keyed
	// on the body-level service_tier field (priority/flex).
	updatedBody, policyErr := s.applyOpenAIFastPolicyToBody(ctx, account, upstreamModel, responsesBody)
	if policyErr != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(policyErr, &blocked) {
			MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
			writeAnthropicError(c, http.StatusForbidden, "forbidden_error", blocked.Message)
		}
		return nil, policyErr
	}
	responsesBody = updatedBody
	grokCacheIdentity := ""
	if account.Platform == PlatformGrok {
		grokIntentBody := responsesBody
		grokCacheIdentity = resolveGrokCacheIdentity(c, grokIntentBody, promptCacheKey, upstreamModel)
		patchedBody, patchErr := patchGrokResponsesBody(grokIntentBody, upstreamModel)
		if patchErr != nil {
			return nil, patchErr
		}
		responsesBody, patchErr = applyGrokResponsesCacheIdentity(patchedBody, grokIntentBody, grokCacheIdentity, account.IsGrokOAuth())
		if patchErr != nil {
			return nil, fmt.Errorf("apply grok prompt cache identity: %w", patchErr)
		}
		responsesBody, patchErr = applyGrokFreeMessagesFunctionToolCacheRoute(responsesBody, grokIntentBody, account, grokCacheIdentity)
		if patchErr != nil {
			return nil, fmt.Errorf("apply grok Free function-tool cache route: %w", patchErr)
		}
	}
	estimatedInputTokens := estimateOpenAIAnthropicStreamInputTokens(responsesBody, upstreamModel)

	// 5. Get access token
	token, _, err := s.getRequestCredential(ctx, c, account)
	if err != nil {
		return nil, fmt.Errorf("get access token: %w", err)
	}

	// 6. Build upstream request
	if account.Type == AccountTypeOAuth && account.Platform != PlatformGrok {
		// Messages 兼容桥即使 body 未带 todo-guard/prompt_cache_key 标记（如映射到非
		// gpt-5/codex 模型），也必须让 buildUpstreamRequest 走 bridge 分支，以保留
		// 既有 body/session/conversation 行为。身份头在 post-build 阶段统一恢复。
		setOpenAICompatMessagesBridgeContext(c, true)
	}
	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	var upstreamReq *http.Request
	if account.Platform == PlatformGrok {
		upstreamReq, err = buildGrokResponsesRequest(upstreamCtx, c, account, responsesBody, token, grokCacheIdentity, s.cfg)
	} else {
		upstreamReq, err = s.buildUpstreamRequest(upstreamCtx, c, account, responsesBody, token, isStream, promptCacheKey, false)
	}
	releaseUpstreamCtx()
	if err != nil {
		return nil, fmt.Errorf("build upstream request: %w", err)
	}

	// Override session_id with a deterministic UUID derived from the isolated
	// session key, ensuring different API keys produce different upstream sessions.
	if account.Platform != PlatformGrok && promptCacheKey != "" {
		isolatedSessionID := generateSessionUUID(isolateOpenAISessionID(apiKeyID, promptCacheKey))
		upstreamReq.Header.Set("session_id", isolatedSessionID)
		if upstreamReq.Header.Get("conversation_id") != "" {
			upstreamReq.Header.Set("conversation_id", isolatedSessionID)
		}
	}
	if account.Type == AccountTypeOAuth && account.Platform != PlatformGrok {
		// buildUpstreamRequest 保留 Messages bridge 的 body/session 兼容行为，并会先
		// 清除身份头。真正发送前恢复完整 Codex 身份，避免 ChatGPT Codex 上游因缺失
		// originator/OpenAI-Beta 返回 404（issue #3901）。
		ensureCodexIdentityHeaders(upstreamReq.Header)
		enforceCodexIdentityHeaders(upstreamReq.Header)
		logger.L().Debug("openai messages: upstream identity restored",
			zap.Int64("account_id", account.ID),
			zap.String("upstream_model", upstreamModel),
			zap.Bool("compat_identity_restored", true),
		)
	}
	if account.Type == AccountTypeOAuth && promptCacheKey != "" && strings.TrimSpace(c.GetHeader("conversation_id")) == "" {
		upstreamReq.Header.Del("conversation_id")
	}
	if compatTurnState != "" && upstreamReq.Header.Get("x-codex-turn-state") == "" {
		upstreamReq.Header.Set("x-codex-turn-state", compatTurnState)
	}

	// 7. Send request
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	// Grok may reject encrypted reasoning replayed under a different OAuth
	// account/cache identity. Match forwardGrokResponses: one strip+retry before
	// treating the 400 as a hard failure / failover trigger.
	var resp *http.Response
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			if account.Platform != PlatformGrok {
				break
			}
			upstreamCtxRetry, releaseRetry := detachUpstreamContext(ctx)
			upstreamReq, err = buildGrokResponsesRequest(upstreamCtxRetry, c, account, responsesBody, token, grokCacheIdentity, s.cfg)
			releaseRetry()
			if err != nil {
				return nil, fmt.Errorf("build grok retry request: %w", err)
			}
		}
		resp, err = s.httpUpstream.Do(upstreamReq, proxyURL, account.ID, account.Concurrency)
		if err != nil {
			return nil, s.handleOpenAIUpstreamTransportError(ctx, c, account, err, false)
		}
		if account.Platform != PlatformGrok || attempt > 0 || resp.StatusCode != http.StatusBadRequest {
			break
		}
		respBody := s.readUpstreamErrorBody(resp)
		if resp.Body != nil {
			_ = resp.Body.Close()
		}
		// Prefer explicit decrypt errors; also strip once on any 400 when the
		// outbound body still carries reasoning.encrypted_content (account
		// switch often returns opaque "Upstream error: 400").
		shouldStrip := isGrokInvalidEncryptedContentResponse(resp.StatusCode, respBody) ||
			requestHasGrokEncryptedReasoning(responsesBody)
		if !shouldStrip {
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			break
		}
		retryBody, changed, trimErr := trimGrokInvalidEncryptedContentRetryBody(responsesBody)
		if trimErr != nil {
			return nil, fmt.Errorf("prepare Grok invalid encrypted_content retry: %w", trimErr)
		}
		if !changed {
			resp.Body = io.NopCloser(bytes.NewReader(respBody))
			break
		}
		responsesBody = retryBody
		logger.L().Info("openai messages: retrying after stripping invalid Grok encrypted_content",
			zap.Int64("account_id", account.ID),
			zap.Bool("cache_identity_present", strings.TrimSpace(grokCacheIdentity) != ""),
			zap.String("upstream_error_preview", truncateOpenAIWSLogValue(string(respBody), 240)),
		)
	}
	defer func() { _ = resp.Body.Close() }()

	// 8. Handle error response with failover
	if resp.StatusCode >= 400 {
		respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
		if !agentIdentityTaskRecoveryWasTried(ctx) && s.isAgentIdentityAccount(ctx, account) && isAgentIdentityTaskInvalidHTTPResponse(resp.StatusCode, respBody) {
			expectedTaskID := account.GetCredential("task_id")
			if err := s.recoverAgentIdentityTask(ctx, account, expectedTaskID); err != nil {
				return nil, fmt.Errorf("agent identity task recovery failed: %w", err)
			}
			return s.ForwardAsAnthropic(markAgentIdentityTaskRecoveryTried(ctx), c, account, body, promptCacheKey, defaultMappedModel)
		}

		if anthropicCompactRequest && anthropicCompactModelMapped &&
			isOpenAICompactModelUnavailableHTTP(resp.StatusCode, upstreamMsg, respBody) &&
			len(anthropicCompactFallbackUpstreamModels) > 0 {
			logger.L().Warn("openai_messages.compact_model_unavailable_fallback",
				zap.Int64("account_id", account.ID),
				zap.String("model", originalModel),
				zap.String("upstream_model", upstreamModel),
				zap.Strings("fallback_upstream_models", anthropicCompactFallbackUpstreamModels),
				zap.Int("upstream_status", resp.StatusCode),
				zap.String("upstream_request_id", resp.Header.Get("x-request-id")),
				zap.String("upstream_message", upstreamMsg),
			)
			result, fallbackErr := s.runAnthropicCompactChunkFallbackWithModelFallbacks(ctx, c, account, &anthropicReq, token, originalModel, anthropicCompactFallbackUpstreamModels, startTime, OpenAIUsage{}, clientStream, resp.Header.Get("x-request-id"))
			if fallbackErr == nil || c.Writer.Written() {
				return result, fallbackErr
			}
			logger.L().Warn("openai_messages.compact_model_unavailable_fallback_failed",
				zap.Int64("account_id", account.ID),
				zap.String("model", originalModel),
				zap.String("upstream_model", upstreamModel),
				zap.Strings("fallback_upstream_models", anthropicCompactFallbackUpstreamModels),
				zap.Error(fallbackErr),
			)
		}
		if previousResponseID != "" && (isOpenAICompatPreviousResponseNotFound(resp.StatusCode, upstreamMsg, respBody) || isOpenAICompatPreviousResponseUnsupported(resp.StatusCode, upstreamMsg, respBody)) {
			if isOpenAICompatPreviousResponseUnsupported(resp.StatusCode, upstreamMsg, respBody) {
				s.disableOpenAICompatSessionContinuation(ctx, c, account, promptCacheKey)
			} else {
				s.deleteOpenAICompatSessionResponseID(ctx, c, account, promptCacheKey)
			}
			logger.L().Info("openai messages: previous_response_id unavailable, retrying without continuation",
				zap.Int64("account_id", account.ID),
				zap.String("previous_response_id", truncateOpenAIWSLogValue(previousResponseID, openAIWSIDValueMaxLen)),
				zap.String("upstream_model", upstreamModel),
			)
			return s.ForwardAsAnthropic(ctx, c, account, body, promptCacheKey, defaultMappedModel)
		}
		// Grok account-switched history often fails decrypt; strip encrypted
		// reasoning once at the client-body level so failover accounts can accept
		// the multi-turn tool continuation instead of cascading 400s.
		if account.Platform == PlatformGrok &&
			isGrokInvalidEncryptedContentResponse(resp.StatusCode, respBody) &&
			!grokEncryptedContentStripRetried(ctx) {
			if strippedBody, ok := stripAnthropicThinkingSignatures(body); ok {
				logger.L().Info("openai messages: stripping thinking signatures for Grok failover retry",
					zap.Int64("account_id", account.ID),
				)
				return s.ForwardAsAnthropic(markGrokEncryptedContentStripRetried(ctx), c, account, strippedBody, promptCacheKey, defaultMappedModel)
			}
		}
		if foErr := s.failoverOpenAIUpstreamHTTPError(ctx, c, account, resp, respBody, upstreamMsg, upstreamModel); foErr != nil {
			return nil, foErr
		}
		// Non-failover error: return Anthropic-formatted error to client
		return s.handleAnthropicErrorResponse(resp, c, account, billingModel)
	}
	if account.Platform == PlatformGrok && account.Type == AccountTypeOAuth && !account.IsShadow() {
		s.updateGrokUsageFromResponse(ctx, account, resp.Header, resp.StatusCode)
	}

	if account.Type == AccountTypeOAuth && promptCacheKey != "" {
		if turnState := strings.TrimSpace(resp.Header.Get("x-codex-turn-state")); turnState != "" {
			s.bindOpenAICompatSessionTurnState(ctx, c, account, promptCacheKey, turnState)
		}
	}

	// 9. Handle normal response
	// Upstream is always streaming; choose response format based on client preference.
	var result *OpenAIForwardResult
	var handleErr error
	if anthropicCompactRequest && anthropicCompactModelMapped {
		result, handleErr = s.handleAnthropicCompactMappedStreamingResponse(ctx, c, account, resp, originalModel, billingModel, upstreamModel, anthropicCompactFallbackUpstreamModels, startTime, &anthropicReq, token, clientStream)
	} else if clientStream {
		result, handleErr = s.handleAnthropicStreamingResponse(resp, c, account, originalModel, billingModel, upstreamModel, startTime, estimatedInputTokens)
	} else {
		// Client wants JSON: buffer the streaming response and assemble a JSON reply.
		result, handleErr = s.handleAnthropicBufferedStreamingResponse(resp, c, account, originalModel, billingModel, upstreamModel, startTime)
	}

	// cyber_policy：标记已设、error 已按 Anthropic 格式发给客户端。丢弃 result、返回哨兵，
	// 使 handler 落入 tokens=0 免费用量行（对齐 /v1/responses），不计费、不 failover。
	if GetOpsCyberPolicy(c) != nil {
		if handleErr == nil {
			handleErr = errOpenAICyberPolicyForwarded
		}
		return nil, handleErr
	}

	// Propagate ServiceTier and ReasoningEffort to result for billing
	if handleErr == nil && result != nil {
		if compatContinuationEnabled && promptCacheKey != "" && result.ResponseID != "" {
			s.bindOpenAICompatSessionResponseID(ctx, c, account, promptCacheKey, result.ResponseID)
		}
		if promptCacheKey != "" && anthropicDigestChain != "" {
			s.bindOpenAICompatAnthropicDigestPromptCacheKey(account, apiKeyID, anthropicDigestChain, promptCacheKey, anthropicMatchedDigestChain)
		}
		if responsesReq.ServiceTier != "" {
			st := responsesReq.ServiceTier
			result.ServiceTier = &st
		}
		if responsesReq.Reasoning != nil && responsesReq.Reasoning.Effort != "" {
			re := responsesReq.Reasoning.Effort
			result.ReasoningEffort = &re
		}
	}

	// Extract and save Codex usage snapshot from response headers (for OAuth accounts).
	// 排除 spark 影子:其 codex_* 仅由 QueryUsage(/wham/usage bengalfox)更新(外审第7轮 P1)。
	if handleErr == nil && account.Type == AccountTypeOAuth && !account.IsShadow() && account.Platform != PlatformGrok {
		if snapshot := ParseCodexRateLimitHeaders(resp.Header); snapshot != nil {
			s.updateCodexUsageSnapshot(ctx, account.ID, snapshot)
		}
	}

	return result, handleErr
}

func ensureCodexOAuthInstructionsField(reqBody map[string]any) {
	if reqBody == nil {
		return
	}
	if value, ok := reqBody["instructions"]; !ok || value == nil {
		reqBody["instructions"] = ""
		return
	}
	if _, ok := reqBody["instructions"].(string); !ok {
		reqBody["instructions"] = ""
	}
}

// handleAnthropicErrorResponse reads an upstream error and returns it in
// Anthropic error format.
func (s *OpenAIGatewayService) handleAnthropicErrorResponse(
	resp *http.Response,
	c *gin.Context,
	account *Account,
	requestedModel ...string,
) (*OpenAIForwardResult, error) {
	return s.handleCompatErrorResponse(resp, c, account, writeAnthropicError, requestedModel...)
}

// handleAnthropicCompactMappedStreamingResponse keeps compact reroutes
// buffered until we know whether the fast compact model can handle the full
// transcript. If it cannot, it summarizes transcript chunks and merges those
// summaries into the compact response Claude Code expects.
func (s *OpenAIGatewayService) handleAnthropicCompactMappedStreamingResponse(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	resp *http.Response,
	originalModel string,
	billingModel string,
	upstreamModel string,
	compactFallbackUpstreamModels []string,
	startTime time.Time,
	anthropicReq *apicompat.AnthropicRequest,
	token string,
	clientStream bool,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	finalResponse, usage, acc, err := s.readOpenAICompatBufferedTerminal(resp, "openai messages compact buffered", requestID)
	if err != nil {
		return nil, err
	}
	if finalResponse == nil {
		writeAnthropicError(c, http.StatusBadGateway, "api_error", "Upstream stream ended without a terminal response event")
		return nil, fmt.Errorf("upstream stream ended without terminal event")
	}

	if isOpenAIResponsesCompactModelUnavailable(finalResponse) && len(compactFallbackUpstreamModels) > 0 {
		logger.L().Warn("openai_messages.compact_model_unavailable_fallback",
			zap.String("request_id", requestID),
			zap.Int64("account_id", account.ID),
			zap.String("model", originalModel),
			zap.String("upstream_model", upstreamModel),
			zap.Strings("fallback_upstream_models", compactFallbackUpstreamModels),
			zap.Int("initial_input_tokens", usage.InputTokens),
			zap.String("upstream_message", openAIResponsesErrorMessage(finalResponse)),
		)
		result, fallbackErr := s.runAnthropicCompactChunkFallbackWithModelFallbacks(ctx, c, account, anthropicReq, token, originalModel, compactFallbackUpstreamModels, startTime, usage, clientStream, requestID)
		if fallbackErr != nil && !c.Writer.Written() {
			writeAnthropicError(c, http.StatusBadGateway, "api_error", openAIAnthropicCompactFallbackFallbackResponse)
		}
		return result, fallbackErr
	}

	if isOpenAIResponsesContextLengthExceeded(finalResponse) {
		logger.L().Warn("openai_messages.compact_context_length_fallback",
			zap.String("request_id", requestID),
			zap.Int64("account_id", account.ID),
			zap.String("model", originalModel),
			zap.String("upstream_model", upstreamModel),
			zap.Int("initial_input_tokens", usage.InputTokens),
		)
		candidates := append([]string{upstreamModel}, compactFallbackUpstreamModels...)
		result, fallbackErr := s.runAnthropicCompactChunkFallbackWithModelFallbacks(ctx, c, account, anthropicReq, token, originalModel, candidates, startTime, usage, clientStream, requestID)
		if fallbackErr != nil && !c.Writer.Written() {
			writeAnthropicError(c, http.StatusBadGateway, "api_error", openAIAnthropicCompactFallbackFallbackResponse)
		}
		return result, fallbackErr
	}

	if strings.TrimSpace(finalResponse.Status) == "failed" {
		payload, _ := json.Marshal(gin.H{"type": "response.failed", "response": finalResponse})
		if hit, code, msg := detectOpenAICyberPolicy(payload); hit {
			MarkOpsCyberPolicy(c, CyberPolicyMark{
				Code:           code,
				Message:        msg,
				Body:           truncateString(string(payload), 4096),
				UpstreamStatus: http.StatusOK,
				UpstreamInTok:  usage.InputTokens,
				UpstreamOutTok: usage.OutputTokens,
			})
			clientMsg := msg
			if clientMsg == "" {
				clientMsg = "Request blocked by upstream cyber-security policy"
			}
			writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", clientMsg)
			return nil, fmt.Errorf("openai cyber_policy: %s", msg)
		}
	}

	acc.SupplementResponseOutput(finalResponse)
	return s.writeAnthropicBufferedFinalResponse(c, account, resp.Header, finalResponse, usage, originalModel, billingModel, upstreamModel, startTime, clientStream, requestID)
}

// handleAnthropicBufferedStreamingResponse reads all Responses SSE events from
// the upstream streaming response, finds the terminal event (response.completed
// / response.incomplete / response.failed), converts the complete response to
// Anthropic Messages JSON format, and writes it to the client.
// This is used when the client requested stream=false but the upstream is always
// streaming.
func (s *OpenAIGatewayService) handleAnthropicBufferedStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	account *Account,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")

	finalResponse, usage, acc, err := s.readOpenAICompatBufferedTerminal(resp, "openai messages buffered", requestID)
	if err != nil {
		return nil, err
	}

	if finalResponse == nil {
		writeAnthropicError(c, http.StatusBadGateway, "api_error", "Upstream stream ended without a terminal response event")
		return nil, fmt.Errorf("upstream stream ended without terminal event")
	}

	if strings.TrimSpace(finalResponse.Status) == "failed" {
		payload, _ := json.Marshal(gin.H{"type": "response.failed", "response": finalResponse})
		if hit, code, msg := detectOpenAICyberPolicy(payload); hit {
			MarkOpsCyberPolicy(c, CyberPolicyMark{
				Code:           code,
				Message:        msg,
				Body:           truncateString(string(payload), 4096),
				UpstreamStatus: http.StatusOK,
				UpstreamInTok:  usage.InputTokens,
				UpstreamOutTok: usage.OutputTokens,
			})
			clientMsg := msg
			if clientMsg == "" {
				clientMsg = "Request blocked by upstream cyber-security policy"
			}
			writeAnthropicError(c, http.StatusBadRequest, "invalid_request_error", clientMsg)
			return nil, fmt.Errorf("openai cyber_policy: %s", msg)
		}
		message := openAICompatFailedResponseMessage(finalResponse)
		if openAIStreamFailedEventShouldFailover(payload, message) {
			return nil, s.newOpenAIStreamFailoverError(c, account, false, requestID, payload, message)
		}
		message = s.recordOpenAIStreamUpstreamError(c, account, false, requestID, "http_error", payload, message)
		// 统一走语义状态推断 + body 归一化（与 /v1/responses 路径一致），
		// 使按错误码配置的透传规则可命中。
		if status, errType, errMsg, matched := applyOpenAIStreamFailedErrorPassthroughRule(
			c, account.Platform, payload, message,
		); matched {
			if errMsg == "" {
				errMsg = message
			}
			MarkResponseCommitted(c)
			writeAnthropicError(c, status, errType, errMsg)
			return nil, fmt.Errorf("upstream response failed (passthrough): %s", errMsg)
		}
		writeAnthropicError(c, http.StatusBadGateway, "api_error", message)
		return nil, fmt.Errorf("upstream response failed: %s", message)
	}

	// When the terminal event has an empty output array, reconstruct from
	// accumulated delta events so the client receives the full content.
	acc.SupplementResponseOutput(finalResponse)

	return s.writeAnthropicBufferedFinalResponse(c, account, resp.Header, finalResponse, usage, originalModel, billingModel, upstreamModel, startTime, false, requestID)
}

func (s *OpenAIGatewayService) writeAnthropicBufferedFinalResponse(
	c *gin.Context,
	account *Account,
	upstreamHeaders http.Header,
	finalResponse *apicompat.ResponsesResponse,
	usage OpenAIUsage,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
	clientStream bool,
	requestID string,
) (*OpenAIForwardResult, error) {
	anthropicResp := apicompat.ResponsesToAnthropic(finalResponse, originalModel)
	if !anthropicResponseHasVisibleOutput(anthropicResp) {
		result := &OpenAIForwardResult{
			RequestID:     requestID,
			ResponseID:    finalResponse.ID,
			Usage:         usage,
			Model:         originalModel,
			BillingModel:  billingModel,
			UpstreamModel: upstreamModel,
			Stream:        clientStream,
			Duration:      time.Since(startTime),
		}
		message := "OpenAI messages buffered response completed without assistant content or tool output"
		return result, s.newOpenAIStreamFailoverError(c, account, false, requestID, nil, message)
	}

	if s.responseHeaderFilter != nil && upstreamHeaders != nil {
		responseheaders.WriteFilteredHeaders(c.Writer.Header(), upstreamHeaders, s.responseHeaderFilter)
	}
	if clientStream {
		if err := writeAnthropicResponseAsSSE(c, anthropicResp); err != nil {
			return nil, err
		}
	} else {
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.JSON(http.StatusOK, anthropicResp)
	}

	return &OpenAIForwardResult{
		RequestID:     requestID,
		ResponseID:    finalResponse.ID,
		Usage:         usage,
		Model:         originalModel,
		BillingModel:  billingModel,
		UpstreamModel: upstreamModel,
		Stream:        clientStream,
		Duration:      time.Since(startTime),
	}, nil
}

func isOpenAIResponsesContextLengthExceeded(resp *apicompat.ResponsesResponse) bool {
	if resp == nil || resp.Error == nil {
		return false
	}
	code := strings.TrimSpace(resp.Error.Code)
	if code == "context_length_exceeded" {
		return true
	}
	message := strings.ToLower(strings.TrimSpace(resp.Error.Message))
	return strings.Contains(message, "context window") && strings.Contains(message, "exceed")
}

func isOpenAIResponsesCompactModelUnavailable(resp *apicompat.ResponsesResponse) bool {
	if resp == nil || resp.Error == nil || isOpenAIResponsesContextLengthExceeded(resp) {
		return false
	}
	return isOpenAICompactUnavailableText(resp.Error.Code + " " + resp.Error.Message)
}

func isOpenAICompactModelUnavailableHTTP(statusCode int, upstreamMsg string, upstreamBody []byte) bool {
	if isOpenAIContextWindowError(upstreamMsg, upstreamBody) {
		return false
	}
	switch statusCode {
	case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout, 529:
		return true
	}
	if statusCode >= 500 {
		return true
	}
	return isOpenAICompactUnavailableText(upstreamMsg + " " + string(upstreamBody))
}

func isOpenAICompactModelUnavailableError(err error) bool {
	if err == nil {
		return false
	}
	return isOpenAICompactUnavailableText(err.Error())
}

func isOpenAICompactUnavailableText(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	for _, pattern := range []string{
		"429",
		"529",
		"rate_limit",
		"rate limit",
		"too many requests",
		"usage limit",
		"quota",
		"resource exhausted",
		"temporarily unavailable",
		"service unavailable",
		"no available account",
		"no available accounts",
		"selected model is at capacity",
		"server is overloaded",
		"slow_down",
		"unsupported model",
		"unknown model",
	} {
		if strings.Contains(lower, pattern) {
			return true
		}
	}
	return strings.Contains(lower, "unavailable") && strings.Contains(lower, "model")
}

func writeAnthropicResponseAsSSE(c *gin.Context, resp *apicompat.AnthropicResponse) error {
	if resp == nil {
		return errors.New("anthropic response is nil")
	}
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Status(http.StatusOK)

	start := *resp
	start.Content = []apicompat.AnthropicContentBlock{}
	start.StopReason = nil
	start.StopSequence = nil
	start.Usage.OutputTokens = 0
	events := []apicompat.AnthropicStreamEvent{{
		Type:    "message_start",
		Message: &start,
	}}

	for i, block := range resp.Content {
		idx := i
		startBlock := block
		switch strings.TrimSpace(startBlock.Type) {
		case "text":
			startBlock.Text = ""
		case "thinking":
			startBlock.Thinking = ""
		}
		events = append(events, apicompat.AnthropicStreamEvent{
			Type:         "content_block_start",
			Index:        &idx,
			ContentBlock: &startBlock,
		})
		switch strings.TrimSpace(block.Type) {
		case "text":
			if block.Text != "" {
				events = append(events, apicompat.AnthropicStreamEvent{
					Type:  "content_block_delta",
					Index: &idx,
					Delta: &apicompat.AnthropicDelta{
						Type: "text_delta",
						Text: block.Text,
					},
				})
			}
		case "thinking":
			if block.Thinking != "" {
				events = append(events, apicompat.AnthropicStreamEvent{
					Type:  "content_block_delta",
					Index: &idx,
					Delta: &apicompat.AnthropicDelta{
						Type:     "thinking_delta",
						Thinking: block.Thinking,
					},
				})
			}
		}
		events = append(events, apicompat.AnthropicStreamEvent{
			Type:  "content_block_stop",
			Index: &idx,
		})
	}

	stopReason := strings.TrimSpace(apicompat.AnthropicStopReasonString(resp.StopReason))
	if stopReason == "" {
		stopReason = "end_turn"
	}
	events = append(events,
		apicompat.AnthropicStreamEvent{
			Type: "message_delta",
			Delta: &apicompat.AnthropicDelta{
				StopReason:   stopReason,
				StopSequence: resp.StopSequence,
			},
			Usage: &resp.Usage,
		},
		apicompat.AnthropicStreamEvent{Type: "message_stop"},
	)

	for _, evt := range events {
		sse, err := apicompat.ResponsesAnthropicEventToSSE(evt)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprint(c.Writer, sse); err != nil {
			return err
		}
	}
	c.Writer.Flush()
	return nil
}

func (s *OpenAIGatewayService) runAnthropicCompactChunkFallbackWithModelFallbacks(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	anthropicReq *apicompat.AnthropicRequest,
	token string,
	originalModel string,
	candidateUpstreamModels []string,
	startTime time.Time,
	initialUsage OpenAIUsage,
	clientStream bool,
	initialRequestID string,
) (*OpenAIForwardResult, error) {
	candidates := compactModelFallbackCandidates(candidateUpstreamModels, "")
	if len(candidates) == 0 {
		return nil, errors.New("compact fallback has no candidate upstream models")
	}

	runningInitialUsage := initialUsage
	var lastResult *OpenAIForwardResult
	var lastErr error
	for i, candidate := range candidates {
		candidateUpstreamModel := normalizeOpenAIModelForUpstream(account, candidate)
		if candidateUpstreamModel == "" {
			continue
		}
		result, err := s.runAnthropicCompactChunkFallback(ctx, c, account, anthropicReq, token, originalModel, candidateUpstreamModel, candidateUpstreamModel, startTime, runningInitialUsage, clientStream, initialRequestID)
		if err == nil {
			return result, nil
		}
		lastResult = result
		lastErr = err
		if result != nil {
			runningInitialUsage = result.Usage
		}
		if !isOpenAICompactModelUnavailableError(err) {
			return result, err
		}
		if i+1 < len(candidates) {
			logger.L().Warn("openai_messages.compact_chunk_model_unavailable_switching",
				zap.Int64("account_id", account.ID),
				zap.String("model", originalModel),
				zap.String("failed_upstream_model", candidateUpstreamModel),
				zap.String("next_upstream_model", normalizeOpenAIModelForUpstream(account, candidates[i+1])),
				zap.Error(err),
			)
		}
	}
	if lastErr == nil {
		lastErr = errors.New("compact fallback exhausted candidate upstream models")
	}
	return lastResult, lastErr
}

func (s *OpenAIGatewayService) runAnthropicCompactChunkFallback(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	anthropicReq *apicompat.AnthropicRequest,
	token string,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
	initialUsage OpenAIUsage,
	clientStream bool,
	initialRequestID string,
) (*OpenAIForwardResult, error) {
	compactPrompt, transcript := buildAnthropicCompactFallbackTranscript(anthropicReq)
	chunks := splitAnthropicCompactTranscriptChunks(transcript, openAIAnthropicCompactChunkTargetChars, openAIAnthropicCompactFallbackMaxChunks)
	if len(chunks) == 0 {
		return nil, errors.New("compact fallback transcript is empty")
	}

	totalUsage := initialUsage
	summaries := make([]string, 0, len(chunks))
	for i, chunk := range chunks {
		prompt := fmt.Sprintf("Chunk %d of %d from a Claude Code conversation transcript:\n\n%s", i+1, len(chunks), chunk)
		finalResponse, usage, requestID, err := s.runOpenAIAnthropicCompactFallbackResponsesRequest(ctx, c, account, token, upstreamModel, openAIAnthropicCompactChunkInstructions(), prompt, openAIAnthropicCompactChunkMaxOutputTokens)
		totalUsage = sumOpenAIUsage(totalUsage, usage)
		if err != nil {
			return &OpenAIForwardResult{
				RequestID:     firstNonEmpty(requestID, initialRequestID),
				Usage:         totalUsage,
				Model:         originalModel,
				BillingModel:  billingModel,
				UpstreamModel: upstreamModel,
				Stream:        clientStream,
				Duration:      time.Since(startTime),
			}, err
		}
		if isOpenAIResponsesContextLengthExceeded(finalResponse) {
			return &OpenAIForwardResult{
				RequestID:     firstNonEmpty(requestID, initialRequestID),
				ResponseID:    finalResponse.ID,
				Usage:         totalUsage,
				Model:         originalModel,
				BillingModel:  billingModel,
				UpstreamModel: upstreamModel,
				Stream:        clientStream,
				Duration:      time.Since(startTime),
			}, fmt.Errorf("compact fallback chunk %d exceeded context window", i+1)
		}
		if strings.TrimSpace(finalResponse.Status) == "failed" {
			return &OpenAIForwardResult{
				RequestID:     firstNonEmpty(requestID, initialRequestID),
				ResponseID:    finalResponse.ID,
				Usage:         totalUsage,
				Model:         originalModel,
				BillingModel:  billingModel,
				UpstreamModel: upstreamModel,
				Stream:        clientStream,
				Duration:      time.Since(startTime),
			}, fmt.Errorf("compact fallback chunk %d failed: %s", i+1, openAIResponsesErrorMessage(finalResponse))
		}
		summary := strings.TrimSpace(openAIResponsesOutputText(finalResponse))
		if summary == "" {
			return &OpenAIForwardResult{
				RequestID:     firstNonEmpty(requestID, initialRequestID),
				ResponseID:    finalResponse.ID,
				Usage:         totalUsage,
				Model:         originalModel,
				BillingModel:  billingModel,
				UpstreamModel: upstreamModel,
				Stream:        clientStream,
				Duration:      time.Since(startTime),
			}, fmt.Errorf("compact fallback chunk %d produced empty summary", i+1)
		}
		summaries = append(summaries, fmt.Sprintf("## Chunk %d/%d\n%s", i+1, len(chunks), summary))
	}

	finalResponse, mergeUsage, mergeRequestID, err := s.mergeAnthropicCompactFallbackSummaries(ctx, c, account, token, upstreamModel, compactPrompt, summaries, openAIAnthropicCompactMergeTargetChars, 0)
	totalUsage = sumOpenAIUsage(totalUsage, mergeUsage)
	if err != nil {
		return &OpenAIForwardResult{
			RequestID:     firstNonEmpty(mergeRequestID, initialRequestID),
			Usage:         totalUsage,
			Model:         originalModel,
			BillingModel:  billingModel,
			UpstreamModel: upstreamModel,
			Stream:        clientStream,
			Duration:      time.Since(startTime),
		}, err
	}
	if finalResponse == nil {
		return nil, errors.New("compact fallback merge response is nil")
	}
	if strings.TrimSpace(finalResponse.Status) == "failed" {
		return &OpenAIForwardResult{
			RequestID:     firstNonEmpty(mergeRequestID, initialRequestID),
			ResponseID:    finalResponse.ID,
			Usage:         totalUsage,
			Model:         originalModel,
			BillingModel:  billingModel,
			UpstreamModel: upstreamModel,
			Stream:        clientStream,
			Duration:      time.Since(startTime),
		}, fmt.Errorf("compact fallback merge failed: %s", openAIResponsesErrorMessage(finalResponse))
	}

	finalResponse.Usage = responsesUsageFromOpenAIUsage(totalUsage)
	return s.writeAnthropicBufferedFinalResponse(c, account, nil, finalResponse, totalUsage, originalModel, billingModel, upstreamModel, startTime, clientStream, firstNonEmpty(mergeRequestID, initialRequestID))
}

func (s *OpenAIGatewayService) mergeAnthropicCompactFallbackSummaries(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token string,
	upstreamModel string,
	compactPrompt string,
	summaries []string,
	targetChars int,
	depth int,
) (*apicompat.ResponsesResponse, OpenAIUsage, string, error) {
	if len(summaries) == 0 {
		return nil, OpenAIUsage{}, "", errors.New("compact fallback merge summaries are empty")
	}
	if targetChars <= 0 {
		targetChars = openAIAnthropicCompactMergeTargetChars
	}
	if depth > openAIAnthropicCompactMergeMaxDepth {
		return nil, OpenAIUsage{}, "", errors.New("compact fallback merge exceeded recursive depth")
	}

	groups := groupAnthropicCompactSummariesForMerge(compactPrompt, summaries, targetChars)
	if len(groups) > 1 {
		totalUsage := OpenAIUsage{}
		reduced := make([]string, 0, len(groups))
		lastRequestID := ""
		for i, group := range groups {
			groupResp, groupUsage, groupRequestID, err := s.mergeAnthropicCompactFallbackSummaries(ctx, c, account, token, upstreamModel, compactPrompt, group, targetChars, depth+1)
			totalUsage = sumOpenAIUsage(totalUsage, groupUsage)
			lastRequestID = firstNonEmpty(groupRequestID, lastRequestID)
			if err != nil {
				return groupResp, totalUsage, lastRequestID, err
			}
			if groupResp == nil {
				return nil, totalUsage, lastRequestID, errors.New("compact fallback grouped merge response is nil")
			}
			summary := strings.TrimSpace(openAIResponsesOutputText(groupResp))
			if summary == "" {
				return groupResp, totalUsage, lastRequestID, fmt.Errorf("compact fallback grouped merge %d produced empty summary", i+1)
			}
			reduced = append(reduced, fmt.Sprintf("## Summary group %d/%d\n%s", i+1, len(groups), summary))
		}
		finalResp, finalUsage, finalRequestID, err := s.mergeAnthropicCompactFallbackSummaries(ctx, c, account, token, upstreamModel, compactPrompt, reduced, targetChars, depth+1)
		totalUsage = sumOpenAIUsage(totalUsage, finalUsage)
		return finalResp, totalUsage, firstNonEmpty(finalRequestID, lastRequestID), err
	}

	mergePrompt := buildAnthropicCompactMergePrompt(compactPrompt, summaries)
	finalResponse, usage, requestID, err := s.runOpenAIAnthropicCompactFallbackResponsesRequest(ctx, c, account, token, upstreamModel, openAIAnthropicCompactMergeInstructions(), mergePrompt, openAIAnthropicCompactMergeMaxOutputTokens)
	if err != nil {
		return finalResponse, usage, requestID, err
	}
	if finalResponse == nil {
		return nil, usage, requestID, errors.New("compact fallback merge response is nil")
	}
	if isOpenAIResponsesContextLengthExceeded(finalResponse) {
		nextTarget := targetChars / 2
		if nextTarget < openAIAnthropicCompactFallbackMinSplitRunes {
			nextTarget = openAIAnthropicCompactFallbackMinSplitRunes
		}
		retrySummaries := retryAnthropicCompactFallbackSummaries(compactPrompt, summaries, nextTarget)
		if len(retrySummaries) > 0 && nextTarget < targetChars {
			retryResp, retryUsage, retryRequestID, retryErr := s.mergeAnthropicCompactFallbackSummaries(ctx, c, account, token, upstreamModel, compactPrompt, retrySummaries, nextTarget, depth+1)
			usage = sumOpenAIUsage(usage, retryUsage)
			return retryResp, usage, firstNonEmpty(retryRequestID, requestID), retryErr
		}
		emergency := buildAnthropicCompactEmergencySummary(compactPrompt, summaries)
		return buildAnthropicCompactEmergencyResponse(upstreamModel, emergency, usage), usage, requestID, nil
	}
	return finalResponse, usage, requestID, nil
}

func (s *OpenAIGatewayService) runOpenAIAnthropicCompactFallbackResponsesRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token string,
	upstreamModel string,
	instructions string,
	userText string,
	maxOutputTokens int,
) (*apicompat.ResponsesResponse, OpenAIUsage, string, error) {
	body, requestModel, err := s.buildOpenAIAnthropicCompactFallbackResponsesBody(account, upstreamModel, instructions, userText, maxOutputTokens)
	if err != nil {
		return nil, OpenAIUsage{}, "", err
	}

	updatedBody, policyErr := s.applyOpenAIFastPolicyToBody(ctx, account, requestModel, body)
	if policyErr != nil {
		var blocked *OpenAIFastBlockedError
		if errors.As(policyErr, &blocked) {
			MarkOpsClientBusinessLimited(c, OpsClientBusinessLimitedReasonLocalPolicyDenied)
			writeAnthropicError(c, http.StatusForbidden, "forbidden_error", blocked.Message)
		}
		return nil, OpenAIUsage{}, "", policyErr
	}
	body = updatedBody
	if account.Platform == PlatformGrok {
		patchedBody, patchErr := patchGrokResponsesBody(body, requestModel)
		if patchErr != nil {
			return nil, OpenAIUsage{}, "", patchErr
		}
		body = patchedBody
	}

	upstreamCtx, releaseUpstreamCtx := detachUpstreamContext(ctx)
	var req *http.Request
	if account.Platform == PlatformGrok {
		req, err = buildGrokResponsesRequest(upstreamCtx, c, account, body, token, "", s.cfg)
	} else {
		req, err = s.buildUpstreamRequest(upstreamCtx, c, account, body, token, true, "", false)
	}
	releaseUpstreamCtx()
	if err != nil {
		return nil, OpenAIUsage{}, "", err
	}
	if account.Type == AccountTypeOAuth && account.Platform != PlatformGrok {
		req.Header.Del("OpenAI-Beta")
		req.Header.Del("originator")
		req.Header.Del("conversation_id")
		req.Header.Del("session_id")
	}

	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.httpUpstream.Do(req, proxyURL, account.ID, account.Concurrency)
	if err != nil {
		safeErr := sanitizeUpstreamErrorMessage(err.Error())
		setOpsUpstreamError(c, 0, safeErr, "")
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: 0,
			Kind:               "compact_fallback_request_error",
			Message:            safeErr,
		})
		return nil, OpenAIUsage{}, "", fmt.Errorf("compact fallback upstream request failed: %s", safeErr)
	}
	defer func() { _ = resp.Body.Close() }()

	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode >= 400 {
		respBody := s.readUpstreamErrorBody(resp)
		s.handleOpenAIAccountUpstreamError(ctx, account, resp.StatusCode, resp.Header, respBody, requestModel)
		upstreamMsg := strings.TrimSpace(extractUpstreamErrorMessage(respBody))
		if upstreamMsg == "" {
			upstreamMsg = http.StatusText(resp.StatusCode)
		}
		return nil, OpenAIUsage{}, requestID, fmt.Errorf("compact fallback upstream status %d: %s", resp.StatusCode, sanitizeUpstreamErrorMessage(upstreamMsg))
	}

	finalResponse, usage, acc, err := s.readOpenAICompatBufferedTerminal(resp, "openai messages compact fallback", requestID)
	if err != nil {
		return nil, usage, requestID, err
	}
	if finalResponse == nil {
		return nil, usage, requestID, errors.New("compact fallback stream ended without terminal response")
	}
	acc.SupplementResponseOutput(finalResponse)
	return finalResponse, usage, requestID, nil
}

func (s *OpenAIGatewayService) buildOpenAIAnthropicCompactFallbackResponsesBody(
	account *Account,
	upstreamModel string,
	instructions string,
	userText string,
	maxOutputTokens int,
) ([]byte, string, error) {
	content, err := json.Marshal([]apicompat.ResponsesContentPart{{
		Type: "input_text",
		Text: userText,
	}})
	if err != nil {
		return nil, upstreamModel, err
	}
	input, err := json.Marshal([]apicompat.ResponsesInputItem{{
		Role:    "user",
		Content: content,
	}})
	if err != nil {
		return nil, upstreamModel, err
	}
	store := false
	req := apicompat.ResponsesRequest{
		Model:           upstreamModel,
		Instructions:    instructions,
		Input:           input,
		MaxOutputTokens: &maxOutputTokens,
		Stream:          true,
		Store:           &store,
		Reasoning: &apicompat.ResponsesReasoning{
			Effort: openAIAnthropicCompactFallbackChunkReasoning,
		},
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, upstreamModel, err
	}

	requestModel := upstreamModel
	if account.Type == AccountTypeOAuth && account.Platform != PlatformGrok {
		var reqBody map[string]any
		if err := json.Unmarshal(body, &reqBody); err != nil {
			return nil, requestModel, err
		}
		codexResult := applyCodexOAuthTransformWithOptions(reqBody, codexOAuthTransformOptions{
			SkipDefaultInstructions: true,
			PreserveToolCallIDs:     true,
		})
		if codexResult.NormalizedModel != "" {
			requestModel = codexResult.NormalizedModel
		}
		ensureCodexOAuthInstructionsField(reqBody)
		delete(reqBody, "prompt_cache_key")
		body, err = json.Marshal(reqBody)
		if err != nil {
			return nil, requestModel, err
		}
	}
	return body, requestModel, nil
}

func buildAnthropicCompactFallbackTranscript(req *apicompat.AnthropicRequest) (string, string) {
	if req == nil {
		return "", ""
	}
	compactIdx := -1
	compactPrompt := ""
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		if strings.TrimSpace(msg.Role) != "user" {
			continue
		}
		text := anthropicContentTextForCompactFallback(msg.Content)
		if looksLikeClaudeCodeCompactPrompt(text) {
			compactIdx = i
			compactPrompt = text
			break
		}
	}

	var parts []string
	if systemText := anthropicContentTextForCompactFallback(req.System); strings.TrimSpace(systemText) != "" {
		parts = append(parts, "### System\n"+strings.TrimSpace(systemText))
	}
	for i, msg := range req.Messages {
		if i == compactIdx {
			continue
		}
		text := strings.TrimSpace(anthropicContentTextForCompactFallback(msg.Content))
		if text == "" {
			continue
		}
		role := strings.TrimSpace(msg.Role)
		if role == "" {
			role = "message"
		}
		parts = append(parts, fmt.Sprintf("### Message %d (%s)\n%s", i+1, role, text))
	}
	return compactPrompt, strings.Join(parts, "\n\n")
}

func anthropicContentTextForCompactFallback(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []apicompat.AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return strings.TrimSpace(string(raw))
	}
	var parts []string
	for _, block := range blocks {
		switch strings.TrimSpace(block.Type) {
		case "text":
			if block.Text != "" {
				parts = append(parts, block.Text)
			}
		case "thinking":
			if block.Thinking != "" {
				parts = append(parts, "[thinking]\n"+block.Thinking)
			}
		case "tool_use", "server_tool_use":
			input := strings.TrimSpace(string(block.Input))
			if input == "" {
				input = "{}"
			}
			parts = append(parts, fmt.Sprintf("[tool_use id=%s name=%s]\n%s", block.ID, block.Name, input))
		case "tool_result", "web_search_tool_result":
			content := anthropicContentTextForCompactFallback(block.Content)
			if content == "" {
				content = strings.TrimSpace(string(block.Content))
			}
			prefix := fmt.Sprintf("[tool_result tool_use_id=%s]", block.ToolUseID)
			if block.IsError {
				prefix += " [error]"
			}
			parts = append(parts, prefix+"\n"+content)
		case "image":
			parts = append(parts, "[image omitted]")
		default:
			if encoded, err := json.Marshal(block); err == nil {
				parts = append(parts, string(encoded))
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func splitAnthropicCompactTranscriptChunks(text string, targetChars int, maxChunks int) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if targetChars <= 0 {
		targetChars = openAIAnthropicCompactChunkTargetChars
	}
	if maxChunks <= 0 {
		maxChunks = openAIAnthropicCompactFallbackMaxChunks
	}

	sections := strings.Split(text, "\n\n### ")
	var chunks []string
	current := make([]string, 0, 8)
	currentRunes := 0
	flushCurrent := func() {
		if len(current) == 0 {
			return
		}
		chunks = append(chunks, strings.Join(current, "\n\n"))
		current = current[:0]
		currentRunes = 0
	}
	for i, section := range sections {
		if i > 0 {
			section = "### " + section
		}
		section = strings.TrimSpace(section)
		if section == "" {
			continue
		}
		sectionRunes := runeLen(section)
		if sectionRunes > targetChars {
			flushCurrent()
			chunks = append(chunks, splitTextByRuneLimit(section, targetChars)...)
			continue
		}
		separatorRunes := 0
		if len(current) > 0 {
			separatorRunes = 2
		}
		if len(current) > 0 && currentRunes+separatorRunes+sectionRunes > targetChars {
			flushCurrent()
			separatorRunes = 0
		}
		current = append(current, section)
		currentRunes += separatorRunes + sectionRunes
	}
	flushCurrent()
	if len(chunks) <= maxChunks {
		return chunks
	}
	return splitTextByRuneLimit(text, ceilDiv(runeLen(text), maxChunks))
}

func splitTextByRuneLimit(text string, targetChars int) []string {
	if targetChars < openAIAnthropicCompactFallbackMinSplitRunes {
		targetChars = openAIAnthropicCompactFallbackMinSplitRunes
	}
	totalRunes := runeLen(text)
	if totalRunes <= targetChars {
		return []string{strings.TrimSpace(text)}
	}
	chunks := make([]string, 0, ceilDiv(totalRunes, targetChars))
	startByte := 0
	runeCount := 0
	for byteIndex := range text {
		if runeCount < targetChars {
			runeCount++
			continue
		}
		chunk := strings.TrimSpace(text[startByte:byteIndex])
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		startByte = byteIndex
		runeCount = 1
	}
	if chunk := strings.TrimSpace(text[startByte:]); chunk != "" {
		chunks = append(chunks, chunk)
	}
	return chunks
}

func buildAnthropicCompactMergePrompt(compactPrompt string, summaries []string) string {
	if strings.TrimSpace(compactPrompt) == "" {
		compactPrompt = "Create a detailed Claude Code compact summary for the conversation. Preserve current work, user intent, files, commands, blockers, and next steps."
	}
	cleaned := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		summary = sanitizeAnthropicCompactSummaryForMerge(summary)
		if strings.TrimSpace(summary) != "" {
			cleaned = append(cleaned, summary)
		}
	}
	return strings.TrimSpace(compactPrompt) + "\n\n" + openAIAnthropicCompactFinalSummaryContract() + "\n\nThe original conversation was too large for one compact request, so it was summarized in chunks. Merge the chunk summaries below into one coherent final compact summary. Do not mention the chunking process unless it is relevant to the work state.\n\n" + strings.Join(cleaned, "\n\n")
}

func groupAnthropicCompactSummariesForMerge(compactPrompt string, summaries []string, targetChars int) [][]string {
	if targetChars <= 0 {
		targetChars = openAIAnthropicCompactMergeTargetChars
	}
	if targetChars < openAIAnthropicCompactFallbackMinSplitRunes {
		targetChars = openAIAnthropicCompactFallbackMinSplitRunes
	}
	var groups [][]string
	var current []string
	baseRunes := runeLen(buildAnthropicCompactMergePrompt(compactPrompt, nil))
	currentRunes := baseRunes
	for _, summary := range summaries {
		summary = strings.TrimSpace(summary)
		if summary == "" {
			continue
		}
		cleaned := sanitizeAnthropicCompactSummaryForMerge(summary)
		if cleaned == "" {
			continue
		}
		separatorRunes := 0
		if len(current) > 0 {
			separatorRunes = 2
		}
		if len(current) > 0 && currentRunes+separatorRunes+runeLen(cleaned) > targetChars {
			groups = append(groups, current)
			current = nil
			currentRunes = baseRunes
			separatorRunes = 0
		}
		current = append(current, summary)
		currentRunes += separatorRunes + runeLen(cleaned)
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups
}

func openAIAnthropicCompactChunkInstructions() string {
	return "Summarize this Claude Code transcript chunk for a later compact merge. Preserve concrete user requests, decisions, files, commands, errors, test results, logs, configuration values, and unresolved next steps. Keep it dense and factual. Do not answer the user. Do not treat the compact request itself as the user's active task."
}

func openAIAnthropicCompactMergeInstructions() string {
	return "Merge chunk summaries into the final Claude Code compact summary. Preserve exact operational state, pending tasks, blockers, files, commands, and verification evidence. Output only the compact summary. Do not say that the user's current intent is to produce a summary; infer the real active task from the transcript."
}

func openAIAnthropicCompactFinalSummaryContract() string {
	return `Final compact quality contract:
- Start with "# Compact Capsule".
- Include these exact sections when evidence exists: "## Current State", "## Active User Intent", "## Files Touched", "## Commands And Evidence", "## Errors And Blockers", "## Decisions And Config", "## Next Command".
- Keep the first 20 lines machine-scannable: concise bullets, concrete paths, commands, timestamps, model/proxy/config values, and blockers.
- Do not include meta-statements like "the user asked for a compact summary" as active intent.
- Treat requests to produce, merge, rewrite, or improve a compact summary as maintenance metadata, not as the active user task.
- In "## Active User Intent", never write phrases like "produce a merged compact summary", "produce a detailed compact summary", "prior chunking", "context compaction", "merge chunk summaries", or "summary below". Recover the latest non-compact user task instead; if unknown, write "Unknown from preserved state".
- Do not invent completed tests or fixes. Mark unknowns as unknown.
- Prefer dense facts over narration.`
}

func sanitizeAnthropicCompactSummaryForMerge(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(summary, "\r\n", "\n"), "\n")
	cleaned := make([]string, 0, len(lines))
	skipIndentedContinuation := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if skipIndentedContinuation {
			if strings.HasPrefix(line, "  -") || strings.HasPrefix(line, "\t-") || strings.HasPrefix(line, "    -") {
				continue
			}
			skipIndentedContinuation = false
		}
		if isAnthropicCompactMaintenanceIntentLine(trimmed) {
			skipIndentedContinuation = true
			continue
		}
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func isAnthropicCompactMaintenanceIntentLine(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	if lower == "" {
		return false
	}
	intentTerms := []string{
		"active intent",
		"current intent",
		"user intent",
		"user asked",
		"user-visible turn",
		"current inferred",
		"latest user",
	}
	compactMaintenanceTerms := []string{
		"compact summary",
		"compact capsule",
		"summary of prior conversation",
		"prior chunking",
		"due prior chunking",
		"due to chunking",
		"context compaction",
		"conversation compaction",
		"merge chunk summaries",
		"merge the chunk summaries",
		"chunk summaries below",
		"summary below",
	}
	hasIntent := false
	for _, term := range intentTerms {
		if strings.Contains(lower, term) {
			hasIntent = true
			break
		}
	}
	if !hasIntent {
		return false
	}
	for _, term := range compactMaintenanceTerms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func retryAnthropicCompactFallbackSummaries(compactPrompt string, summaries []string, targetChars int) []string {
	if targetChars <= 0 {
		targetChars = openAIAnthropicCompactMergeTargetChars / 2
	}
	if targetChars < openAIAnthropicCompactFallbackMinSplitRunes {
		targetChars = openAIAnthropicCompactFallbackMinSplitRunes
	}
	if len(summaries) == 0 {
		return nil
	}
	if len(summaries) > 1 {
		return summaries
	}
	summary := strings.TrimSpace(summaries[0])
	if summary == "" {
		return nil
	}
	// A single intermediate summary can still be too large once the final
	// compact instructions are prepended. Split it so the recursive reducer can
	// shrink it in smaller model calls instead of returning a hard 502.
	parts := splitTextByRuneLimit(summary, targetChars)
	if len(parts) <= 1 && runeLen(buildAnthropicCompactMergePrompt(compactPrompt, []string{summary})) <= targetChars {
		return nil
	}
	retry := make([]string, 0, len(parts))
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		retry = append(retry, fmt.Sprintf("## Oversized summary split %d/%d\n%s", i+1, len(parts), part))
	}
	return retry
}

func buildAnthropicCompactEmergencySummary(compactPrompt string, summaries []string) string {
	joined := strings.TrimSpace(strings.Join(summaries, "\n\n"))
	if joined == "" {
		joined = strings.TrimSpace(compactPrompt)
	}
	if joined == "" {
		joined = openAIAnthropicCompactFallbackFallbackResponse
	}
	joined = trimRunesMiddle(joined, openAIAnthropicCompactEmergencyMaxRunes)
	return "# Compact Capsule\n\n" +
		"## Current State\n" +
		"- The proxy had to use its emergency compact fallback because the upstream compact merge still exceeded the context window.\n" +
		"- The content below is the best available compressed state from chunk summaries; verify exact details against the transcript if precision matters.\n\n" +
		"## Active User Intent\n" +
		"- Continue the original task from the preserved state below. Do not treat the compact operation itself as the user task.\n\n" +
		"## Preserved State\n" +
		joined + "\n\n" +
		"## Next Command\n" +
		"- Inspect the latest user prompt and resume from the preserved state without asking for a recap."
}

func buildAnthropicCompactEmergencyResponse(model string, summary string, usage OpenAIUsage) *apicompat.ResponsesResponse {
	if strings.TrimSpace(model) == "" {
		model = "compact-fallback"
	}
	return &apicompat.ResponsesResponse{
		ID:     fmt.Sprintf("compact_fallback_%d", time.Now().UnixNano()),
		Object: "response",
		Model:  model,
		Status: "completed",
		Output: []apicompat.ResponsesOutput{{
			Type:   "message",
			Role:   "assistant",
			Status: "completed",
			Content: []apicompat.ResponsesContentPart{{
				Type: "output_text",
				Text: summary,
			}},
		}},
		Usage: responsesUsageFromOpenAIUsage(usage),
	}
}

func trimRunesMiddle(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	totalRunes := runeLen(text)
	if totalRunes <= maxRunes {
		return text
	}
	keepHead := maxRunes * 2 / 3
	keepTail := maxRunes - keepHead
	if keepHead < 0 {
		keepHead = 0
	}
	if keepTail < 0 {
		keepTail = 0
	}
	headEnd := byteIndexAtRune(text, keepHead)
	tailStart := byteIndexAtRune(text, totalRunes-keepTail)
	return text[:headEnd] + "\n\n[... middle omitted by compact fallback emergency guard ...]\n\n" + text[tailStart:]
}

func openAIResponsesOutputText(resp *apicompat.ResponsesResponse) string {
	if resp == nil {
		return ""
	}
	var parts []string
	for _, item := range resp.Output {
		if strings.TrimSpace(item.Type) != "message" {
			continue
		}
		for _, part := range item.Content {
			if strings.TrimSpace(part.Type) == "output_text" && part.Text != "" {
				parts = append(parts, part.Text)
			}
		}
	}
	return strings.Join(parts, "\n")
}

func openAIResponsesErrorMessage(resp *apicompat.ResponsesResponse) string {
	if resp == nil || resp.Error == nil {
		return ""
	}
	if strings.TrimSpace(resp.Error.Message) != "" {
		return strings.TrimSpace(resp.Error.Message)
	}
	return strings.TrimSpace(resp.Error.Code)
}

func responsesUsageFromOpenAIUsage(usage OpenAIUsage) *apicompat.ResponsesUsage {
	result := &apicompat.ResponsesUsage{
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		TotalTokens:  usage.InputTokens + usage.OutputTokens,
	}
	if usage.CacheReadInputTokens > 0 {
		result.InputTokensDetails = &apicompat.ResponsesInputTokensDetails{CachedTokens: usage.CacheReadInputTokens}
	}
	return result
}

func sumOpenAIUsage(a OpenAIUsage, b OpenAIUsage) OpenAIUsage {
	return OpenAIUsage{
		InputTokens:              a.InputTokens + b.InputTokens,
		ImageInputTokens:         a.ImageInputTokens + b.ImageInputTokens,
		OutputTokens:             a.OutputTokens + b.OutputTokens,
		CacheCreationInputTokens: a.CacheCreationInputTokens + b.CacheCreationInputTokens,
		CacheReadInputTokens:     a.CacheReadInputTokens + b.CacheReadInputTokens,
		ImageOutputTokens:        a.ImageOutputTokens + b.ImageOutputTokens,
	}
}

func runeLen(s string) int {
	return utf8.RuneCountInString(s)
}

func byteIndexAtRune(text string, target int) int {
	if target <= 0 {
		return 0
	}
	count := 0
	for byteIndex := range text {
		if count == target {
			return byteIndex
		}
		count++
	}
	return len(text)
}

func ceilDiv(a, b int) int {
	if b <= 0 {
		return 0
	}
	return (a + b - 1) / b
}

func isOpenAICompatResponsesTerminalEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	case "response.completed", "response.done", "response.incomplete", "response.failed":
		return true
	default:
		return false
	}
}

func (s *OpenAIGatewayService) recordOpenAIMessagesStreamUpstreamError(c *gin.Context, account *Account, upstreamRequestID, kind, message string) {
	if c == nil {
		return
	}
	message = sanitizeUpstreamErrorMessage(message)
	setOpsUpstreamError(c, http.StatusBadGateway, message, "")
	event := OpsUpstreamErrorEvent{
		Platform:           PlatformOpenAI,
		UpstreamStatusCode: http.StatusBadGateway,
		UpstreamRequestID:  strings.TrimSpace(upstreamRequestID),
		Kind:               kind,
		Message:            message,
	}
	if account != nil {
		event.Platform = account.Platform
		event.AccountID = account.ID
		event.AccountName = account.Name
	}
	appendOpsUpstreamError(c, event)
}

func isOpenAICompatDoneSentinelLine(line string) bool {
	payload, ok := extractOpenAISSEDataLine(line)
	return ok && strings.TrimSpace(payload) == "[DONE]"
}

func (s *OpenAIGatewayService) readOpenAICompatBufferedTerminal(
	resp *http.Response,
	logPrefix string,
	requestID string,
) (*apicompat.ResponsesResponse, OpenAIUsage, *apicompat.BufferedResponseAccumulator, error) {
	acc := apicompat.NewBufferedResponseAccumulator()
	var usage OpenAIUsage
	if resp == nil || resp.Body == nil {
		return nil, usage, acc, errors.New("upstream response body is nil")
	}

	scanner := s.newUpstreamSSEScanner(resp.Body)

	streamInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	var timeoutCh <-chan time.Time
	var timeoutTimer *time.Timer
	resetTimeout := func() {
		if streamInterval <= 0 {
			return
		}
		if timeoutTimer == nil {
			timeoutTimer = time.NewTimer(streamInterval)
			timeoutCh = timeoutTimer.C
			return
		}
		if !timeoutTimer.Stop() {
			select {
			case <-timeoutTimer.C:
			default:
			}
		}
		timeoutTimer.Reset(streamInterval)
	}
	stopTimeout := func() {
		if timeoutTimer == nil {
			return
		}
		if !timeoutTimer.Stop() {
			select {
			case <-timeoutTimer.C:
			default:
			}
		}
	}
	resetTimeout()
	defer stopTimeout()

	type scanEvent struct {
		line string
		err  error
	}
	events := make(chan scanEvent, 16)
	done := make(chan struct{})
	go func() {
		defer close(events)
		for scanner.Scan() {
			select {
			case events <- scanEvent{line: scanner.Text()}:
			case <-done:
				return
			}
		}
		if err := scanner.Err(); err != nil {
			select {
			case events <- scanEvent{err: err}:
			case <-done:
			}
		}
	}()
	defer close(done)

	var parser openAICompatSSEFrameParser
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				if frame, ok := parser.Finish(); ok {
					payload := openAICompatPayloadWithEventType(frame.Data, frame.EventType)
					var event apicompat.ResponsesStreamEvent
					if err := json.Unmarshal([]byte(payload), &event); err == nil {
						acc.ProcessEvent(&event)
						if isOpenAICompatResponsesTerminalEvent(event.Type) && event.Response != nil {
							if event.Usage != nil {
								usage = copyOpenAIUsageFromResponsesUsage(event.Usage)
								if event.Response.Usage == nil {
									event.Response.Usage = event.Usage
								}
							}
							if event.Response.Usage != nil {
								usage = copyOpenAIUsageFromResponsesUsage(event.Response.Usage)
							}
							return event.Response, usage, acc, nil
						}
					}
				}
				return nil, usage, acc, nil
			}
			resetTimeout()
			if ev.err != nil {
				if !errors.Is(ev.err, context.Canceled) && !errors.Is(ev.err, context.DeadlineExceeded) {
					logger.L().Warn(logPrefix+": read error",
						zap.Error(ev.err),
						zap.String("request_id", requestID),
					)
				}
				return nil, usage, acc, ev.err
			}

			if isOpenAICompatDoneSentinelLine(ev.line) {
				return nil, usage, acc, nil
			}
			frame, ok := parser.AddLine(ev.line)
			if !ok {
				continue
			}
			payload := openAICompatPayloadWithEventType(frame.Data, frame.EventType)

			var event apicompat.ResponsesStreamEvent
			if err := json.Unmarshal([]byte(payload), &event); err != nil {
				logger.L().Warn(logPrefix+": failed to parse event",
					zap.Error(err),
					zap.String("request_id", requestID),
				)
				continue
			}

			acc.ProcessEvent(&event)

			if isOpenAICompatResponsesTerminalEvent(event.Type) && event.Response != nil {
				if event.Usage != nil {
					usage = copyOpenAIUsageFromResponsesUsage(event.Usage)
					if event.Response.Usage == nil {
						event.Response.Usage = event.Usage
					}
				}
				if event.Response.Usage != nil {
					usage = copyOpenAIUsageFromResponsesUsage(event.Response.Usage)
				}
				return event.Response, usage, acc, nil
			}

		case <-timeoutCh:
			_ = resp.Body.Close()
			logger.L().Warn(logPrefix+": data interval timeout",
				zap.String("request_id", requestID),
				zap.Duration("interval", streamInterval),
			)
			return nil, usage, acc, fmt.Errorf("stream data interval timeout")
		}
	}
}

func estimateOpenAIAnthropicStreamInputTokens(responsesBody []byte, _ string) int {
	effectiveBytes := len(responsesBody)
	for remaining := responsesBody; len(remaining) > 0; {
		marker := bytes.Index(remaining, []byte("data:image/"))
		if marker < 0 {
			break
		}
		remaining = remaining[marker:]
		comma := bytes.IndexByte(remaining, ',')
		if comma < 0 {
			break
		}
		encoded := remaining[comma+1:]
		end := bytes.IndexByte(encoded, '"')
		if end < 0 {
			break
		}
		effectiveBytes -= end
		remaining = encoded[end+1:]
	}
	if effectiveBytes <= 0 {
		return 0
	}
	// Message-start usage is advisory; terminal upstream usage remains the
	// billing source of truth. A byte heuristic avoids running tiktoken over the
	// whole prompt before TTFT (1 MiB previously cost about 50 ms and 46 MiB).
	return max(1, (effectiveBytes+3)/4)
}

// handleAnthropicStreamingResponse reads Responses SSE events from upstream,
// converts each to Anthropic SSE events, and writes them to the client.
// When StreamKeepaliveInterval is configured, it uses a goroutine + channel
// pattern to send Anthropic ping events during periods of upstream silence,
// preventing proxy/client timeout disconnections.
func (s *OpenAIGatewayService) handleAnthropicStreamingResponse(
	resp *http.Response,
	c *gin.Context,
	account *Account,
	originalModel string,
	billingModel string,
	upstreamModel string,
	startTime time.Time,
	estimatedInputTokens int,
) (*OpenAIForwardResult, error) {
	requestID := resp.Header.Get("x-request-id")
	writeStreamHeaders := s.newStreamHeaderWriter(c, resp.Header)

	state := apicompat.NewResponsesEventToAnthropicState()
	state.Model = originalModel
	if estimatedInputTokens > 0 {
		state.InputTokens = estimatedInputTokens
	}
	var usage OpenAIUsage
	responseID := ""
	var firstTokenMs *int
	firstChunk := true
	clientDisconnected := false
	clientOutputStarted := false
	var streamFailoverErr error
	var streamNonFailoverErr error
	clientVisibleOutputStarted := false
	terminalClientErrorHandled := false
	var terminalClientError error
	var pendingClientSSE bytes.Buffer
	pendingClientSSE.Grow(4096)
	streamDiag := newOpenAIMessagesStreamDiagnostic(estimatedInputTokens)

	scanner := s.newUpstreamSSEScanner(resp.Body)

	streamInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamDataIntervalTimeout > 0 {
		streamInterval = time.Duration(s.cfg.Gateway.StreamDataIntervalTimeout) * time.Second
	}
	var intervalTicker *time.Ticker
	if streamInterval > 0 {
		intervalTicker = time.NewTicker(streamInterval)
		defer intervalTicker.Stop()
	}
	var intervalCh <-chan time.Time
	if intervalTicker != nil {
		intervalCh = intervalTicker.C
	}

	// resultWithUsage builds the final result snapshot.
	resultWithUsage := func() *OpenAIForwardResult {
		return &OpenAIForwardResult{
			RequestID:           requestID,
			ResponseID:          responseID,
			Usage:               usage,
			Model:               originalModel,
			BillingModel:        billingModel,
			UpstreamModel:       upstreamModel,
			Stream:              true,
			Duration:            time.Since(startTime),
			FirstTokenMs:        firstTokenMs,
			ClientDisconnect:    clientDisconnected,
			ClientOutputStarted: clientOutputStarted,
		}
	}

	flushPendingClientSSE := func() {
		if clientDisconnected || pendingClientSSE.Len() == 0 {
			return
		}
		writeStreamHeaders()
		written, err := pendingClientSSE.WriteTo(c.Writer)
		if written > 0 {
			clientOutputStarted = true
		}
		if err != nil {
			clientDisconnected = true
			logger.L().Info("openai messages stream: client disconnected during buffered flush",
				zap.String("request_id", requestID),
			)
		}
		if !clientDisconnected {
			c.Writer.Flush()
		}
	}

	// processDataLine handles a single "data: ..." SSE line from upstream.
	processDataLine := func(payload string) bool {
		if firstChunk {
			firstChunk = false
			ms := int(time.Since(startTime).Milliseconds())
			firstTokenMs = &ms
		}

		var event apicompat.ResponsesStreamEvent
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			logger.L().Warn("openai messages stream: failed to parse event",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
			return false
		}
		streamDiag.Record(event)

		eventType := strings.TrimSpace(event.Type)
		isBareErrorEvent := eventType == "error"
		isTerminalEvent := isOpenAICompatResponsesTerminalEvent(eventType) || isBareErrorEvent
		if isTerminalEvent {
			if event.Response != nil {
				if id := strings.TrimSpace(event.Response.ID); id != "" {
					responseID = id
				}
				if event.Response.Usage != nil {
					usage = copyOpenAIUsageFromResponsesUsage(event.Response.Usage)
				}
			}
			if event.Usage != nil {
				usage = copyOpenAIUsageFromResponsesUsage(event.Usage)
			}
			// cyber_policy 致命不可重试：标记供 handler 事后记录；以 Anthropic SSE error 事件
			// 回写让客户端感知并停止重试（F4），丢弃后续转换输出。
			if eventType == "response.failed" || isBareErrorEvent {
				payloadBytes := []byte(payload)
				if hit, code, msg := detectOpenAICyberPolicy(payloadBytes); hit {
					MarkOpsCyberPolicy(c, CyberPolicyMark{
						Code:           code,
						Message:        msg,
						Body:           truncateString(payload, 4096),
						UpstreamStatus: http.StatusOK,
						UpstreamInTok:  usage.InputTokens,
						UpstreamOutTok: usage.OutputTokens,
					})
					if !clientDisconnected {
						writeStreamHeaders()
						clientMsg := msg
						if clientMsg == "" {
							clientMsg = "Request blocked by upstream cyber-security policy"
						}
						if _, err := fmt.Fprint(c.Writer, buildAnthropicStreamErrorSSE("invalid_request_error", clientMsg)); err == nil {
							c.Writer.Flush()
						}
						clientDisconnected = true
					}
					terminalClientErrorHandled = true
					terminalClientError = fmt.Errorf("openai cyber_policy: %s", msg)
					return true
				}
				message := extractOpenAISSEErrorMessage(payloadBytes)
				if !clientOutputStarted && isOpenAIContextWindowError(message, payloadBytes) {
					if message == "" {
						message = "Your input exceeds the context window of this model. Please reduce the conversation context and try again."
					}
					streamFailoverErr = s.newOpenAIStreamClientError(
						c, account, requestID, http.StatusBadRequest, "invalid_request_error", message,
					)
					return true
				}
				// Once Anthropic output has started, switching accounts would splice
				// two model streams together. Surface a proper Anthropic error event
				// instead of returning a failover error that the handler cannot retry.
				if !clientOutputStarted && openAIStreamFailedEventShouldFailover(payloadBytes, message) {
					streamFailoverErr = s.newOpenAIStreamFailoverError(c, account, false, requestID, payloadBytes, message)
					return true
				}
				message = s.recordOpenAIStreamUpstreamError(c, account, false, requestID, "http_error", payloadBytes, message)
				errStatus, errType, errMsg := http.StatusBadGateway, "api_error", message
				// 统一走语义状态推断 + body 归一化（与 /v1/responses 路径一致），
				// 使按错误码配置的透传规则可命中。
				if status, et, em, matched := applyOpenAIStreamFailedErrorPassthroughRule(
					c, account.Platform, payloadBytes, message,
				); matched {
					if em == "" {
						em = errMsg
					}
					errStatus, errType, errMsg = status, et, em
					MarkResponseCommitted(c)
				}
				if !clientDisconnected {
					if !clientOutputStarted {
						writeAnthropicError(c, errStatus, errType, errMsg)
						clientOutputStarted = true
					} else {
						writeStreamHeaders()
						if _, err := fmt.Fprint(c.Writer, buildAnthropicStreamErrorSSE(errType, errMsg)); err == nil {
							c.Writer.Flush()
						}
					}
				}
				streamNonFailoverErr = fmt.Errorf("upstream response failed: %s", errMsg)
				return true
			}
		}

		// Convert to Anthropic events
		events := apicompat.ResponsesEventToAnthropicEvents(&event, state)
		if !clientDisconnected {
			for _, evt := range events {
				sse, err := apicompat.ResponsesAnthropicEventToSSE(evt)
				if err != nil {
					logger.L().Warn("openai messages stream: failed to marshal event",
						zap.Error(err),
						zap.String("request_id", requestID),
					)
					continue
				}
				if !clientVisibleOutputStarted {
					_, _ = pendingClientSSE.WriteString(sse)
					if anthropicStreamEventHasVisibleOutput(evt) {
						clientVisibleOutputStarted = true
						flushPendingClientSSE()
						if clientDisconnected {
							break
						}
					}
					continue
				}
				writeStreamHeaders()
				if _, err := fmt.Fprint(c.Writer, sse); err != nil {
					clientDisconnected = true
					logger.L().Info("openai messages stream: client disconnected, continuing to drain upstream for billing",
						zap.String("request_id", requestID),
					)
					break
				}
				clientOutputStarted = true
			}
		}
		if len(events) > 0 && !clientDisconnected && clientVisibleOutputStarted {
			c.Writer.Flush()
		}
		return isTerminalEvent
	}

	// finalizeStream sends any remaining Anthropic events and returns the result.
	finalizeStream := func() (*OpenAIForwardResult, error) {
		if streamFailoverErr != nil {
			return resultWithUsage(), streamFailoverErr
		}
		if streamNonFailoverErr != nil {
			return resultWithUsage(), streamNonFailoverErr
		}
		if finalEvents := apicompat.FinalizeResponsesAnthropicStream(state); len(finalEvents) > 0 && !clientDisconnected {
			for _, evt := range finalEvents {
				sse, err := apicompat.ResponsesAnthropicEventToSSE(evt)
				if err != nil {
					continue
				}
				if !clientVisibleOutputStarted {
					_, _ = pendingClientSSE.WriteString(sse)
					if anthropicStreamEventHasVisibleOutput(evt) {
						clientVisibleOutputStarted = true
						flushPendingClientSSE()
						if clientDisconnected {
							break
						}
					}
					continue
				}
				writeStreamHeaders()
				if _, err := fmt.Fprint(c.Writer, sse); err != nil {
					clientDisconnected = true
					logger.L().Info("openai messages stream: client disconnected during final flush",
						zap.String("request_id", requestID),
					)
					break
				}
				clientOutputStarted = true
			}
			if !clientDisconnected && clientVisibleOutputStarted {
				c.Writer.Flush()
			}
		}
		if terminalClientErrorHandled {
			if terminalClientError == nil {
				terminalClientError = errors.New("terminal stream error handled")
			}
			return nil, terminalClientError
		}
		if !clientVisibleOutputStarted {
			result := resultWithUsage()
			fields := []zap.Field{
				zap.String("request_id", requestID),
				zap.String("response_id", responseID),
				zap.Int64("account_id", account.ID),
				zap.String("model", originalModel),
				zap.String("upstream_model", upstreamModel),
			}
			fields = append(fields, streamDiag.ZapFields()...)
			logger.L().Warn("openai_messages.stream_completed_without_visible_output", fields...)
			if streamDiag.TerminalErrorCode == "context_length_exceeded" {
				message := strings.TrimSpace(streamDiag.TerminalErrorMessage)
				if message == "" {
					message = "Your input exceeds the context window of this model. Please reduce the conversation context and try again."
				}
				return result, s.newOpenAIStreamClientError(c, account, requestID, http.StatusBadRequest, "invalid_request_error", message)
			}
			if streamDiag.TerminalStatus == "incomplete" && streamDiag.TerminalIncompleteReason == "max_output_tokens" {
				message := "OpenAI response reached max_output_tokens before producing assistant content; reduce the conversation context or output budget and try again."
				return result, s.newOpenAIStreamClientError(c, account, requestID, http.StatusBadRequest, "invalid_request_error", message)
			}
			message := "OpenAI messages stream completed without assistant content or tool output"
			return result, s.newOpenAIStreamFailoverError(c, account, false, requestID, nil, message)
		}
		flushPendingClientSSE()
		return resultWithUsage(), nil
	}

	// handleScanErr logs scanner errors if meaningful.
	handleScanErr := func(err error) {
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			logger.L().Warn("openai messages stream: read error",
				zap.Error(err),
				zap.String("request_id", requestID),
			)
		}
	}
	missingTerminalErr := func() (*OpenAIForwardResult, error) {
		result := resultWithUsage()
		if clientDisconnected {
			return result, fmt.Errorf("stream usage incomplete: missing terminal event")
		}
		message := "OpenAI messages stream ended before a terminal event"
		if !clientOutputStarted {
			return result, s.newOpenAIStreamFailoverError(c, account, false, requestID, nil, message)
		}
		s.recordOpenAIMessagesStreamUpstreamError(c, account, requestID, "stream_missing_terminal", message)
		return result, fmt.Errorf("stream usage incomplete: missing terminal event")
	}
	processFrame := func(frame openAICompatSSEFrame) bool {
		payload := openAICompatPayloadWithEventType(frame.Data, frame.EventType)
		return processDataLine(payload)
	}

	// ── Determine keepalive interval ──
	keepaliveInterval := time.Duration(0)
	if s.cfg != nil && s.cfg.Gateway.StreamKeepaliveInterval > 0 {
		keepaliveInterval = time.Duration(s.cfg.Gateway.StreamKeepaliveInterval) * time.Second
	}

	// ── No keepalive: fast synchronous path (no goroutine overhead) ──
	if streamInterval <= 0 && keepaliveInterval <= 0 {
		var parser openAICompatSSEFrameParser
		for scanner.Scan() {
			line := scanner.Text()
			if isOpenAICompatDoneSentinelLine(line) {
				return missingTerminalErr()
			}
			frame, ok := parser.AddLine(line)
			if !ok {
				continue
			}
			if processFrame(frame) {
				return finalizeStream()
			}
		}
		if err := scanner.Err(); err != nil {
			handleScanErr(err)
			return resultWithUsage(), fmt.Errorf("stream usage incomplete: %w", err)
		}
		if frame, ok := parser.Finish(); ok {
			if strings.TrimSpace(frame.Data) == "[DONE]" {
				return missingTerminalErr()
			}
			if processFrame(frame) {
				return finalizeStream()
			}
		}
		return missingTerminalErr()
	}

	// ── With keepalive: goroutine + channel + select ──
	type scanEvent struct {
		line string
		err  error
	}
	events := make(chan scanEvent, 16)
	done := make(chan struct{})
	var lastReadAt int64
	atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
	sendEvent := func(ev scanEvent) bool {
		select {
		case events <- ev:
			return true
		case <-done:
			return false
		}
	}
	go func() {
		defer close(events)
		for scanner.Scan() {
			atomic.StoreInt64(&lastReadAt, time.Now().UnixNano())
			if !sendEvent(scanEvent{line: scanner.Text()}) {
				return
			}
		}
		if err := scanner.Err(); err != nil {
			_ = sendEvent(scanEvent{err: err})
		}
	}()
	defer close(done)

	var keepaliveTicker *time.Ticker
	if keepaliveInterval > 0 {
		keepaliveTicker = time.NewTicker(keepaliveInterval)
		defer keepaliveTicker.Stop()
	}
	var keepaliveCh <-chan time.Time
	if keepaliveTicker != nil {
		keepaliveCh = keepaliveTicker.C
	}
	lastDataAt := time.Now()
	var parser openAICompatSSEFrameParser

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				// Upstream closed
				if frame, ok := parser.Finish(); ok {
					if strings.TrimSpace(frame.Data) == "[DONE]" {
						return missingTerminalErr()
					}
					if processFrame(frame) {
						return finalizeStream()
					}
				}
				return missingTerminalErr()
			}
			if ev.err != nil {
				handleScanErr(ev.err)
				return resultWithUsage(), fmt.Errorf("stream usage incomplete: %w", ev.err)
			}
			lastDataAt = time.Now()
			line := ev.line
			if isOpenAICompatDoneSentinelLine(line) {
				return missingTerminalErr()
			}
			frame, ok := parser.AddLine(line)
			if !ok {
				continue
			}
			if processFrame(frame) {
				return finalizeStream()
			}

		case <-intervalCh:
			lastRead := time.Unix(0, atomic.LoadInt64(&lastReadAt))
			if time.Since(lastRead) < streamInterval {
				continue
			}
			if clientDisconnected {
				return resultWithUsage(), fmt.Errorf("stream usage incomplete after timeout")
			}
			logger.L().Warn("openai messages stream: data interval timeout",
				zap.String("request_id", requestID),
				zap.String("model", originalModel),
				zap.Duration("interval", streamInterval),
			)
			return resultWithUsage(), fmt.Errorf("stream data interval timeout")

		case <-keepaliveCh:
			if clientDisconnected {
				continue
			}
			if !clientVisibleOutputStarted {
				continue
			}
			if time.Since(lastDataAt) < keepaliveInterval {
				continue
			}
			// Send Anthropic-format ping event
			writeStreamHeaders()
			if _, err := fmt.Fprint(c.Writer, "event: ping\ndata: {\"type\":\"ping\"}\n\n"); err != nil {
				// Client disconnected
				logger.L().Info("openai messages stream: client disconnected during keepalive",
					zap.String("request_id", requestID),
				)
				clientDisconnected = true
				continue
			}
			clientOutputStarted = true
			c.Writer.Flush()
		}
	}
}

func anthropicStreamEventHasVisibleOutput(evt apicompat.AnthropicStreamEvent) bool {
	switch strings.TrimSpace(evt.Type) {
	case "content_block_start":
		if evt.ContentBlock == nil {
			return false
		}
		switch strings.TrimSpace(evt.ContentBlock.Type) {
		case "tool_use", "server_tool_use":
			return true
		default:
			return false
		}
	case "content_block_delta":
		if evt.Delta == nil {
			return false
		}
		return strings.TrimSpace(evt.Delta.Text) != "" ||
			strings.TrimSpace(evt.Delta.PartialJSON) != ""
	default:
		return false
	}
}

func isClaudeCodeCompactAnthropicRequest(req *apicompat.AnthropicRequest) bool {
	if req == nil || len(req.Messages) == 0 {
		return false
	}
	for i := len(req.Messages) - 1; i >= 0; i-- {
		msg := req.Messages[i]
		if strings.TrimSpace(msg.Role) != "user" {
			continue
		}
		return looksLikeClaudeCodeCompactPrompt(anthropicMessageText(msg.Content))
	}
	return false
}

func anthropicMessageText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var blocks []apicompat.AnthropicContentBlock
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, block := range blocks {
		if strings.TrimSpace(block.Type) == "text" && block.Text != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n\n")
}

func looksLikeClaudeCodeCompactPrompt(text string) bool {
	text = strings.TrimSpace(text)
	if !strings.Contains(text, "Your task is to create a detailed summary") {
		return false
	}
	for _, marker := range []string{
		"<analysis>",
		"<summary>",
		"All user messages",
		"Pending Tasks",
		"Current Work",
	} {
		if !strings.Contains(text, marker) {
			return false
		}
	}
	return true
}

func anthropicResponseHasVisibleOutput(resp *apicompat.AnthropicResponse) bool {
	if resp == nil {
		return false
	}
	for _, block := range resp.Content {
		switch strings.TrimSpace(block.Type) {
		case "text":
			if strings.TrimSpace(block.Text) != "" {
				return true
			}
		case "tool_use", "server_tool_use":
			return true
		}
	}
	return false
}

type openAIMessagesStreamDiagnostic struct {
	EstimatedInputTokens int
	EventTypes           map[string]int
	OutputItemTypes      map[string]int
	FinalOutputTypes     map[string]int

	OutputTextDeltaBytes       int
	OutputTextDoneCount        int
	ReasoningDeltaBytes        int
	ReasoningDoneCount         int
	FunctionArgsDeltaBytes     int
	FunctionArgsDoneCount      int
	TerminalType               string
	TerminalStatus             string
	TerminalOutputCount        int
	TerminalIncompleteReason   string
	TerminalErrorCode          string
	TerminalErrorMessage       string
	TerminalUsageInputTokens   int
	TerminalUsageOutputTokens  int
	TerminalUsageTotalTokens   int
	TerminalMessageTextBytes   int
	TerminalReasoningTextBytes int
}

func newOpenAIMessagesStreamDiagnostic(estimatedInputTokens int) *openAIMessagesStreamDiagnostic {
	return &openAIMessagesStreamDiagnostic{
		EstimatedInputTokens: estimatedInputTokens,
		EventTypes:           make(map[string]int),
		OutputItemTypes:      make(map[string]int),
		FinalOutputTypes:     make(map[string]int),
	}
}

func (d *openAIMessagesStreamDiagnostic) Record(evt apicompat.ResponsesStreamEvent) {
	if d == nil {
		return
	}
	eventType := strings.TrimSpace(evt.Type)
	if eventType == "" {
		eventType = "<empty>"
	}
	d.EventTypes[eventType]++
	switch eventType {
	case "response.output_item.added", "response.output_item.done":
		if evt.Item != nil {
			itemType := strings.TrimSpace(evt.Item.Type)
			if itemType == "" {
				itemType = "<empty>"
			}
			d.OutputItemTypes[itemType]++
		}
	case "response.output_text.delta":
		d.OutputTextDeltaBytes += len(evt.Delta)
	case "response.output_text.done":
		d.OutputTextDoneCount++
	case "response.reasoning_summary_text.delta", "response.reasoning_text.delta":
		d.ReasoningDeltaBytes += len(evt.Delta)
	case "response.reasoning_summary_text.done":
		d.ReasoningDoneCount++
	case "response.function_call_arguments.delta", "response.custom_tool_call_input.delta":
		d.FunctionArgsDeltaBytes += len(evt.Delta)
	case "response.function_call_arguments.done":
		d.FunctionArgsDoneCount++
	}
	if evt.Response != nil {
		d.TerminalType = eventType
		d.TerminalStatus = strings.TrimSpace(evt.Response.Status)
		d.TerminalOutputCount = len(evt.Response.Output)
		d.FinalOutputTypes = make(map[string]int)
		for _, out := range evt.Response.Output {
			outputType := strings.TrimSpace(out.Type)
			if outputType == "" {
				outputType = "<empty>"
			}
			d.FinalOutputTypes[outputType]++
			switch outputType {
			case "message":
				for _, part := range out.Content {
					if strings.TrimSpace(part.Type) == "output_text" {
						d.TerminalMessageTextBytes += len(part.Text)
					}
				}
			case "reasoning":
				for _, summary := range out.Summary {
					d.TerminalReasoningTextBytes += len(summary.Text)
				}
			}
		}
		if evt.Response.IncompleteDetails != nil {
			d.TerminalIncompleteReason = strings.TrimSpace(evt.Response.IncompleteDetails.Reason)
		}
		if evt.Response.Error != nil {
			d.TerminalErrorCode = strings.TrimSpace(evt.Response.Error.Code)
			d.TerminalErrorMessage = strings.TrimSpace(evt.Response.Error.Message)
		}
		if evt.Response.Usage != nil {
			d.TerminalUsageInputTokens = evt.Response.Usage.InputTokens
			d.TerminalUsageOutputTokens = evt.Response.Usage.OutputTokens
			d.TerminalUsageTotalTokens = evt.Response.Usage.TotalTokens
		}
	}
	if evt.Usage != nil {
		d.TerminalUsageInputTokens = evt.Usage.InputTokens
		d.TerminalUsageOutputTokens = evt.Usage.OutputTokens
		d.TerminalUsageTotalTokens = evt.Usage.TotalTokens
	}
}

func (d *openAIMessagesStreamDiagnostic) ZapFields() []zap.Field {
	if d == nil {
		return nil
	}
	return []zap.Field{
		zap.Int("estimated_input_tokens", d.EstimatedInputTokens),
		zap.Any("responses_event_types", d.EventTypes),
		zap.Any("responses_output_item_types", d.OutputItemTypes),
		zap.Any("responses_final_output_types", d.FinalOutputTypes),
		zap.Int("output_text_delta_bytes", d.OutputTextDeltaBytes),
		zap.Int("output_text_done_count", d.OutputTextDoneCount),
		zap.Int("reasoning_delta_bytes", d.ReasoningDeltaBytes),
		zap.Int("reasoning_done_count", d.ReasoningDoneCount),
		zap.Int("function_args_delta_bytes", d.FunctionArgsDeltaBytes),
		zap.Int("function_args_done_count", d.FunctionArgsDoneCount),
		zap.String("terminal_event_type", d.TerminalType),
		zap.String("terminal_status", d.TerminalStatus),
		zap.Int("terminal_output_count", d.TerminalOutputCount),
		zap.String("terminal_incomplete_reason", d.TerminalIncompleteReason),
		zap.String("terminal_error_code", d.TerminalErrorCode),
		zap.String("terminal_error_message", d.TerminalErrorMessage),
		zap.Int("terminal_usage_input_tokens", d.TerminalUsageInputTokens),
		zap.Int("terminal_usage_output_tokens", d.TerminalUsageOutputTokens),
		zap.Int("terminal_usage_total_tokens", d.TerminalUsageTotalTokens),
		zap.Int("terminal_message_text_bytes", d.TerminalMessageTextBytes),
		zap.Int("terminal_reasoning_text_bytes", d.TerminalReasoningTextBytes),
	}
}

// writeAnthropicError writes an error response in Anthropic Messages API format.
func writeAnthropicError(c *gin.Context, statusCode int, errType, message string) {
	c.JSON(statusCode, gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}

// buildAnthropicStreamErrorSSE builds one Anthropic SSE `error` event so a
// streaming response can terminate with a visible error (e.g. upstream
// cyber_policy) and programmatic clients stop retrying.
// Marshal 失败的兜底仅保留固定提示。
func buildAnthropicStreamErrorSSE(errType, message string) string {
	payload, err := json.Marshal(gin.H{
		"type": "error",
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
	if err != nil {
		return "event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"" + errType + "\",\"message\":\"upstream error\"}}\n\n"
	}
	return "event: error\ndata: " + string(payload) + "\n\n"
}

func copyOpenAIUsageFromResponsesUsage(usage *apicompat.ResponsesUsage) OpenAIUsage {
	if usage == nil {
		return OpenAIUsage{}
	}
	result := OpenAIUsage{
		InputTokens:              usage.InputTokens,
		OutputTokens:             usage.OutputTokens,
		CacheCreationInputTokens: usage.CacheCreationInputTokens,
	}
	if usage.InputTokensDetails != nil {
		result.CacheReadInputTokens = usage.InputTokensDetails.CachedTokens
	}
	return result
}

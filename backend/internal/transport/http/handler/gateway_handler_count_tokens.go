package handler

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/shared/ip"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// CountTokens handles token counting endpoint
// POST /v1/messages/count_tokens
// 特点：校验订阅/余额，但不计算并发、不记录使用量
func (h *GatewayHandler) CountTokens(c *gin.Context) {
	// 从context获取apiKey和user（ApiKeyAuth中间件已设置）
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	_, ok = middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(
		c,
		"handler.gateway.count_tokens",
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)
	defer h.maybeLogCompatibilityFallbackMetrics(reqLog)

	// 读取请求体
	body, err := readLenientJSONRequestBodyWithPrealloc(c.Request, h.cfg)
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

	setOpsRequestContext(c, "", false)

	bodyRef := service.NewRequestBodyRef(body)
	parsedReq, err := service.ParseGatewayRequest(bodyRef, domain.PlatformAnthropic)
	if err != nil {
		logRequestBodyParseFailure(reqLog, body, err)
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}
	body = parsedReq.Body.Bytes()
	// count_tokens 走 messages 严格校验时，复用已解析请求，避免二次反序列化。
	SetClaudeCodeClientContext(c, body, parsedReq)
	ensureCompositeTargetPlatform(c, apiKey, parsedReq.Model)
	reqLog = reqLog.With(zap.String("model", parsedReq.Model), zap.Bool("stream", parsedReq.Stream))
	// 在请求上下文中记录 thinking 状态，供 Antigravity 最终模型 key 推导/模型维度限流使用
	c.Request = c.Request.WithContext(service.WithThinkingEnabled(c.Request.Context(), parsedReq.ThinkingEnabled, h.metadataBridgeEnabled()))

	// 验证 model 必填
	if parsedReq.Model == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	if !compositeTargetPlatformResolved(c, apiKey, parsedReq.Model) {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Model is not supported by composite groups")
		return
	}

	setOpsRequestContext(c, parsedReq.Model, parsedReq.Stream)
	setOpsEndpointContext(c, "", int16(service.RequestTypeFromLegacy(parsedReq.Stream, false)))

	// 获取订阅信息（可能为nil）
	subscription, _ := middleware2.GetSubscriptionFromContext(c)

	// 校验 billing eligibility（订阅/余额）
	// 【注意】不计算并发，但需要校验订阅/余额
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
		status, code, message, retryAfter := billingErrorDetails(err)
		if retryAfter > 0 {
			c.Header("Retry-After", strconv.Itoa(retryAfter))
		}
		h.errorResponse(c, status, code, message)
		return
	}

	// 计算粘性会话 hash
	parsedReq.SessionContext = &service.SessionContext{
		ClientIP:  ip.GetClientIP(c),
		UserAgent: c.GetHeader("User-Agent"),
		APIKeyID:  apiKey.ID,
	}
	sessionHash := h.gatewayService.GenerateSessionHash(parsedReq)

	// 选择支持该模型的账号
	account, err := h.gatewayService.SelectAccountForModel(c.Request.Context(), apiKey.GroupID, sessionHash, parsedReq.Model)
	if err != nil {
		reqLog.Warn("gateway.count_tokens_select_account_failed", zap.Error(err))
		cls := classifyNoAccountErrorFromGin(c, h.gatewayService, apiKey, parsedReq.Model, parsedReq.Model, service.PlatformAnthropic)
		if !cls.ModelNotFound {
			markOpsRoutingCapacityLimitedIfNoAvailable(c, err)
		}
		h.errorResponse(c, cls.Status, cls.ErrType, cls.Message)
		return
	}
	setOpsSelectedAccount(c, account.ID, account.Platform)

	// 转发请求（不记录使用量）
	if err := h.gatewayService.ForwardCountTokens(c.Request.Context(), c, account, parsedReq); err != nil {
		reqLog.Error("gateway.count_tokens_forward_failed", zap.Int64("account_id", account.ID), zap.Error(err))
		// 错误响应已在 ForwardCountTokens 中处理
		return
	}
}

// InterceptType 表示请求拦截类型
type InterceptType int

const (
	InterceptTypeNone              InterceptType = iota
	InterceptTypeWarmup                          // 预热请求（返回 "New Conversation"）
	InterceptTypeSuggestionMode                  // SUGGESTION MODE（返回空字符串）
	InterceptTypeMaxTokensOneHaiku               // max_tokens=1 + haiku 探测请求（返回 "#"）
)

// isHaikuModel 检查模型名称是否包含 "haiku"（大小写不敏感）
func isHaikuModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "haiku")
}

// isMaxTokensOneHaikuRequest 检查是否为 max_tokens=1 + haiku 模型的探测请求
// 这类请求用于 Claude Code 验证 API 连通性（流式/非流式均会出现，如 cc-switch v3.9.0 起的健康检查探测为流式）
// 条件：max_tokens == 1 且 model 包含 "haiku"
func isMaxTokensOneHaikuRequest(model string, maxTokens int) bool {
	return maxTokens == 1 && isHaikuModel(model)
}

// detectInterceptType 检测请求是否需要拦截，返回拦截类型
// 参数说明：
//   - body: 请求体字节
//   - model: 请求的模型名称
//   - maxTokens: max_tokens 值
//   - isClaudeCodeClient: 是否已通过 Claude Code 客户端校验
func detectInterceptType(body []byte, model string, maxTokens int, isClaudeCodeClient bool) InterceptType {
	// 优先检查 max_tokens=1 + haiku 探测请求（流式/非流式均适用）
	if isClaudeCodeClient && isMaxTokensOneHaikuRequest(model, maxTokens) {
		return InterceptTypeMaxTokensOneHaiku
	}

	// 快速检查：如果不包含任何关键字，直接返回
	bodyStr := string(body)
	hasSuggestionMode := strings.Contains(bodyStr, "[SUGGESTION MODE:")
	hasWarmupKeyword := strings.Contains(bodyStr, "title") || strings.Contains(bodyStr, "Warmup")

	if !hasSuggestionMode && !hasWarmupKeyword {
		return InterceptTypeNone
	}

	// 解析请求（只解析一次）
	var req struct {
		Messages []struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
		System []struct {
			Text string `json:"text"`
		} `json:"system"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return InterceptTypeNone
	}

	// 检查 SUGGESTION MODE（最后一条 user 消息）
	if hasSuggestionMode && len(req.Messages) > 0 {
		lastMsg := req.Messages[len(req.Messages)-1]
		if lastMsg.Role == "user" && len(lastMsg.Content) > 0 &&
			lastMsg.Content[0].Type == "text" &&
			strings.HasPrefix(lastMsg.Content[0].Text, "[SUGGESTION MODE:") {
			return InterceptTypeSuggestionMode
		}
	}

	// 检查 Warmup 请求
	if hasWarmupKeyword {
		// 检查 messages 中的标题提示模式
		for _, msg := range req.Messages {
			for _, content := range msg.Content {
				if content.Type == "text" {
					if strings.Contains(content.Text, "Please write a 5-10 word title for the following conversation:") ||
						content.Text == "Warmup" {
						return InterceptTypeWarmup
					}
				}
			}
		}
		// 检查 system 中的标题提取模式
		for _, sys := range req.System {
			if strings.Contains(sys.Text, "nalyze if this message indicates a new conversation topic. If it does, extract a 2-3 word title") {
				return InterceptTypeWarmup
			}
		}
	}

	return InterceptTypeNone
}

// sendMockInterceptStream 发送流式 mock 响应（用于请求拦截）
func sendMockInterceptStream(c *gin.Context, model string, interceptType InterceptType) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// 根据拦截类型决定响应内容
	var msgID string
	var outputTokens int
	var textDeltas []string

	switch interceptType {
	case InterceptTypeSuggestionMode:
		msgID = "msg_mock_suggestion"
		outputTokens = 1
		textDeltas = []string{""} // 空内容
	default: // InterceptTypeWarmup
		msgID = "msg_mock_warmup"
		outputTokens = 2
		textDeltas = []string{"New", " Conversation"}
	}

	// Build message_start event with fixed schema.
	messageStartJSON := `{"type":"message_start","message":{"id":` + strconv.Quote(msgID) + `,"type":"message","role":"assistant","model":` + strconv.Quote(model) + `,"content":[],"stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":10,"output_tokens":0}}}`

	// Build events
	events := []string{
		`event: message_start` + "\n" + `data: ` + string(messageStartJSON),
		`event: content_block_start` + "\n" + `data: {"content_block":{"text":"","type":"text"},"index":0,"type":"content_block_start"}`,
	}

	// Add text deltas
	for _, text := range textDeltas {
		deltaJSON := `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":` + strconv.Quote(text) + `}}`
		events = append(events, `event: content_block_delta`+"\n"+`data: `+string(deltaJSON))
	}

	// Add final events
	messageDeltaJSON := `{"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"input_tokens":10,"output_tokens":` + strconv.Itoa(outputTokens) + `}}`

	events = append(events,
		`event: content_block_stop`+"\n"+`data: {"index":0,"type":"content_block_stop"}`,
		`event: message_delta`+"\n"+`data: `+string(messageDeltaJSON),
		`event: message_stop`+"\n"+`data: {"type":"message_stop"}`,
	)

	for _, event := range events {
		_, _ = c.Writer.WriteString(event + "\n\n")
		c.Writer.Flush()
		time.Sleep(20 * time.Millisecond)
	}
}

// generateRealisticMsgID 生成仿真的消息 ID（msg_bdrk_XXXXXXX 格式）
// 格式与 Claude API 真实响应一致，24 位随机字母数字
func generateRealisticMsgID() string {
	const charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	const idLen = 24
	randomBytes := make([]byte, idLen)
	if _, err := rand.Read(randomBytes); err != nil {
		return fmt.Sprintf("msg_bdrk_%d", time.Now().UnixNano())
	}
	b := make([]byte, idLen)
	for i := range b {
		b[i] = charset[int(randomBytes[i])%len(charset)]
	}
	return "msg_bdrk_" + string(b)
}

// sendMockInterceptResponse 发送非流式 mock 响应（用于请求拦截）
func sendMockInterceptResponse(c *gin.Context, model string, interceptType InterceptType) {
	var msgID, text, stopReason string
	var outputTokens int

	switch interceptType {
	case InterceptTypeSuggestionMode:
		msgID = "msg_mock_suggestion"
		text = ""
		outputTokens = 1
		stopReason = "end_turn"
	case InterceptTypeMaxTokensOneHaiku:
		msgID = generateRealisticMsgID()
		text = "#"
		outputTokens = 1
		stopReason = "max_tokens" // max_tokens=1 探测请求的 stop_reason 应为 max_tokens
	default: // InterceptTypeWarmup
		msgID = "msg_mock_warmup"
		text = "New Conversation"
		outputTokens = 2
		stopReason = "end_turn"
	}

	// 构建完整的响应格式（与 Claude API 响应格式一致）
	response := gin.H{
		"model":         model,
		"id":            msgID,
		"type":          "message",
		"role":          "assistant",
		"content":       []gin.H{{"type": "text", "text": text}},
		"stop_reason":   stopReason,
		"stop_sequence": nil,
		"usage": gin.H{
			"input_tokens":                10,
			"cache_creation_input_tokens": 0,
			"cache_read_input_tokens":     0,
			"cache_creation": gin.H{
				"ephemeral_5m_input_tokens": 0,
				"ephemeral_1h_input_tokens": 0,
			},
			"output_tokens": outputTokens,
			"total_tokens":  10 + outputTokens,
		},
	}

	c.JSON(http.StatusOK, response)
}

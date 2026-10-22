package service

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func openAIStreamFailedEventSemanticStatus(payload []byte, message string) int {
	if _, policy := openAIUpstreamPolicyErrorDetails(payload); policy {
		return http.StatusBadRequest
	}
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
	if _, policy := openAIUpstreamPolicyErrorDetails(payload); policy {
		return http.StatusBadRequest
	}
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
	if detail, policy := openAIUpstreamPolicyErrorDetails(payload); policy {
		return http.StatusBadRequest, detail.Type, detail.Message, true
	}
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
	if _, policy := openAIUpstreamPolicyErrorDetails(payload); policy {
		s.recordOpenAIStreamUpstreamError(c, account, passthrough, upstreamRequestID, "http_error", payload, message)
		return nil, false // The streaming caller forwards the terminal event.
	}
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
			if !writeOpenAIResponsesErrorAfterKeepalive(c, status, errType, errMsg) {
				c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
				c.JSON(status, gin.H{
					"error": gin.H{
						"type":    errType,
						"message": errMsg,
					},
				})
			}
		}
		return fmt.Errorf("upstream response failed: passthrough rule matched message=%s", errMsg), true
	}
	return nil, false
}

func openAIStreamFailedEventShouldFailover(payload []byte, message string) bool {
	if _, policy := openAIUpstreamPolicyErrorDetails(payload); policy {
		return false
	}
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
	if _, policy := openAIUpstreamPolicyErrorDetails(payload); policy {
		return false
	}
	if gjson.GetBytes(payload, "type").String() == "error" && openAIStreamFailedEventErrorCode(payload) == "server_error" {
		return openAIStreamFailedEventShouldFailover(payload, message)
	}
	if IsOpenAIGenericUpstreamFailureBody(payload) {
		return true
	}
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

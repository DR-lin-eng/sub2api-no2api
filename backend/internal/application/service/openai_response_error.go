package service

import (
	"bytes"
	"fmt"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/Wei-Shaw/sub2api/internal/shared/responseheaders"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func extractOpenAISSEErrorMessage(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	for _, path := range []string{"response.error.message", "error.message", "message"} {
		if msg := strings.TrimSpace(gjson.GetBytes(payload, path).String()); msg != "" {
			return sanitizeUpstreamErrorMessage(msg)
		}
	}
	return sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(payload)))
}

const openAIReplayUnsafeTransientFailureMessage = "Upstream service temporarily unavailable. Please retry your request."

func (s *OpenAIGatewayService) handleOpenAINonStreamingFailureEnvelope(
	resp *http.Response,
	c *gin.Context,
	account *Account,
	failurePayload []byte,
	body []byte,
	passthrough bool,
) error {
	if c != nil && c.Request != nil {
		s.observeCodexEncryptedContentPayload(c.Request.Context(), c, account, openAICodexTurnStateModel(c), failurePayload, "http_failure")
	}
	if policyErr, handled := writeOpenAIUpstreamPolicyError(c, resp.StatusCode, failurePayload); handled {
		return policyErr
	}
	message := extractOpenAISSEErrorMessage(failurePayload)
	if message == "" {
		message = "Upstream response failed"
	}
	requestID := strings.TrimSpace(resp.Header.Get("x-request-id"))
	if !openAIJSONFailureHasSemanticOutput(body) {
		if handledErr, handled := s.handleOpenAIResponseFailedBeforeOutput(
			c, account, passthrough, requestID, failurePayload, message, resp.Header,
		); handled {
			return handledErr
		}
	} else {
		s.recordOpenAIStreamUpstreamError(c, account, passthrough, requestID, "http_error", failurePayload, message)
		logger.LegacyPrintf("service.openai_gateway", "OpenAI HTTP 200 failure envelope after semantic output; failover skipped: response already produced output")
		if openAIStreamFailureIsExplicitlyRetryable(failurePayload, message) {
			message = openAIReplayUnsafeTransientFailureMessage
		}
	}
	return s.writeOpenAINonStreamingProtocolError(resp, c, message)
}

func sanitizeOpenAIResponseFailedEventForClient(payload []byte, eventType string, clientOutputStarted bool) ([]byte, bool) {
	if (eventType != "response.failed" && eventType != "error") || len(payload) == 0 || !gjson.ValidBytes(payload) {
		return payload, false
	}
	updated := payload
	capacityShed := isOpenAIUpstreamCapacityShedEvent(payload)
	if capacityShed {
		if rewritten, changed := sanitizeOpenAICapacityShedErrorCodeForClient(updated); changed {
			updated = rewritten
		}
	}
	message := extractOpenAISSEErrorMessage(payload)
	if clientOutputStarted && openAIStreamFailureIsExplicitlyRetryable(payload, message) {
		errorPath := "error"
		if gjson.GetBytes(updated, "response.error").Exists() {
			errorPath = "response.error"
		}
		code := "upstream_unavailable"
		if capacityShed {
			code = openAICapacityShedRetryableClientCode
		}
		for path, value := range map[string]string{
			errorPath + ".type":    "upstream_error",
			errorPath + ".code":    code,
			errorPath + ".message": openAIReplayUnsafeTransientFailureMessage,
		} {
			next, err := sjson.SetBytes(updated, path, value)
			if err != nil {
				return payload, false
			}
			updated = next
		}
	}
	if clientOutputStarted && isOpenAIContextWindowError(message, payload) {
		errorPath := ""
		switch {
		case gjson.GetBytes(updated, "response.error").Exists():
			errorPath = "response.error"
		case gjson.GetBytes(updated, "error").Exists():
			errorPath = "error"
		}
		if errorPath != "" {
			next, err := sjson.SetBytes(updated, errorPath+".type", "invalid_request_error")
			if err != nil {
				return payload, false
			}
			updated = next
			next, err = sjson.SetBytes(updated, errorPath+".code", "context_length_exceeded")
			if err != nil {
				return payload, false
			}
			updated = next
		}
	}
	if !gjson.GetBytes(updated, "response").Exists() {
		return updated, !bytes.Equal(updated, payload)
	}
	for _, path := range []string{
		"response.instructions",
		"response.output",
		"response.usage",
		"response.metadata",
		"response.reasoning",
		"response.tools",
		"response.tool_choice",
		"response.parallel_tool_calls",
		"response.text",
		"response.truncation",
		"response.max_output_tokens",
		"response.incomplete_details",
	} {
		next, err := sjson.DeleteBytes(updated, path)
		if err != nil {
			return payload, false
		}
		updated = next
	}
	return updated, !bytes.Equal(updated, payload)
}

func (s *OpenAIGatewayService) writeOpenAINonStreamingProtocolError(resp *http.Response, c *gin.Context, message string) error {
	message = sanitizeUpstreamErrorMessage(strings.TrimSpace(message))
	if message == "" {
		message = "Upstream returned an invalid non-streaming response"
	}
	setOpsUpstreamError(c, http.StatusBadGateway, message, "")
	// body-signal compact 心跳可能已把响应头提交为 200，此时只能以
	// response.failed 终止事件回传错误，不能再写 JSON+状态码。
	if openAICompactClientWantsStream(c) && StopOpenAICompactSSEKeepaliveCommitted(c) {
		writeOpenAICompactSSEFailureMessage(c, http.StatusBadGateway, "upstream_error", message)
		return fmt.Errorf("non-streaming openai protocol error: %s", message)
	}
	responseheaders.WriteFilteredHeaders(c.Writer.Header(), resp.Header, s.responseHeaderFilter)
	c.Writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.JSON(http.StatusBadGateway, gin.H{
		"error": gin.H{
			"type":    "upstream_error",
			"message": message,
		},
	})
	return fmt.Errorf("non-streaming openai protocol error: %s", message)
}

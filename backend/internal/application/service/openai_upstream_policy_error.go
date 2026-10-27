package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

type openAIUpstreamPolicyError struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
	Param   string `json:"param,omitempty"`
}

// Inspect error fields only: generated text and tool arguments may quote a
// policy error without the upstream actually rejecting the request.
func openAIUpstreamPolicyErrorDetails(payload []byte) (openAIUpstreamPolicyError, bool) {
	if !gjson.ValidBytes(payload) {
		return openAIUpstreamPolicyError{}, false
	}
	for _, path := range []string{"response.error", "error"} {
		value := gjson.GetBytes(payload, path)
		if !value.IsObject() {
			continue
		}
		detail := openAIUpstreamPolicyError{
			Type:    strings.TrimSpace(value.Get("type").String()),
			Code:    strings.TrimSpace(value.Get("code").String()),
			Message: strings.TrimSpace(value.Get("message").String()),
			Param:   strings.TrimSpace(value.Get("param").String()),
		}
		// Specific credential, billing and validation codes keep their existing
		// semantics even when their diagnostic message mentions a policy.
		switch strings.ToLower(detail.Code) {
		case "invalid_api_key", "account_deactivated", "account_disabled", "token_expired", "token_revoked",
			"rate_limit_exceeded", "insufficient_quota", "insufficient_balance", "invalid_function_parameters", "context_length_exceeded":
			continue
		}
		policyCode := ""
		for _, signal := range []string{detail.Code, detail.Type} {
			switch strings.ToLower(signal) {
			case "cyber_policy", "session_blocked_by_cyber_policy", "content_policy_violation", "content_filter", "content_filter_error",
				"safety_violation", "safety_policy_violation", "security_policy_violation", "policy_violation":
				policyCode = signal
			}
		}
		if policyCode == "" && !isOpenAIPolicyRejectionMessage(detail.Message) {
			continue
		}
		if detail.Code == "" {
			detail.Code = policyCode
			if detail.Code == "" {
				detail.Code = "content_policy_violation"
			}
		}
		if detail.Type == "" {
			detail.Type = "invalid_request_error"
		}
		detail.Message = sanitizeUpstreamErrorMessage(detail.Message)
		if detail.Message == "" {
			detail.Message = "Request blocked by upstream safety policy"
			if strings.EqualFold(detail.Code, "cyber_policy") {
				detail.Message = "Request blocked by upstream cyber-security policy"
			}
		}
		return detail, true
	}
	return openAIUpstreamPolicyError{}, false
}

func isOpenAIPolicyRejectionMessage(message string) bool {
	message = strings.ToLower(strings.TrimSpace(message))
	policy := false
	for _, marker := range []string{"content policy", "safety policy", "security policy", "usage policy", "safety system", "high-risk cyber", "biological risk", "安全策略", "内容政策"} {
		policy = policy || strings.Contains(message, marker)
	}
	if !policy {
		return false
	}
	for _, marker := range []string{"reject", "block", "violat", "flagged", "not allowed", "拒绝", "拦截", "违反", "屏蔽"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func openAIPolicyClientStatus(upstreamStatus int) int {
	switch upstreamStatus {
	case http.StatusBadRequest, http.StatusForbidden, http.StatusUnprocessableEntity:
		return upstreamStatus
	default:
		return http.StatusBadRequest
	}
}

// Only selected error fields reach the client; transport headers, request
// metadata and provider response bodies remain subject to the existing filters.
func writeOpenAIUpstreamPolicyError(c *gin.Context, upstreamStatus int, payload []byte) (error, bool) {
	detail, policy := openAIUpstreamPolicyErrorDetails(payload)
	if !policy {
		return nil, false
	}
	markOpenAICyberPolicyEvent(c, payload, upstreamStatus, nil)
	setOpsUpstreamError(c, upstreamStatus, detail.Message, "")
	MarkResponseCommitted(c)
	body, _ := json.Marshal(gin.H{"error": detail})
	writeOpenAIUpstreamClientError(c, openAIPolicyClientStatus(upstreamStatus), body, detail.Message)
	if strings.EqualFold(detail.Code, "cyber_policy") {
		return fmt.Errorf("openai cyber_policy: %s", detail.Message), true
	}
	return fmt.Errorf("upstream response failed: %s", detail.Message), true
}

// Responses clients require a terminal Responses event. A bare type:error
// policy rejection otherwise looks like a truncated response even at HTTP 200.
func normalizeOpenAIPolicyErrorEvent(payload []byte) ([]byte, bool) {
	eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	if eventType != "" && eventType != "error" {
		return payload, false
	}
	detail, policy := openAIUpstreamPolicyErrorDetails(payload)
	if !policy {
		return payload, false
	}
	response := gin.H{
		"id":     "resp_" + strings.ReplaceAll(uuid.NewString(), "-", ""),
		"object": "response", "created_at": time.Now().Unix(),
		"status": "failed", "output": []any{}, "error": detail,
	}
	if usage := gjson.GetBytes(payload, "usage"); usage.IsObject() {
		response["usage"] = json.RawMessage(usage.Raw)
	}
	normalized, err := json.Marshal(gin.H{
		"type": "response.failed", "sequence_number": gjson.GetBytes(payload, "sequence_number").Int(),
		"response": response,
	})
	if err != nil {
		return payload, false
	}
	return normalized, true
}

func isOpenAISSEErrorEventLine(line string) bool {
	return strings.HasPrefix(line, "event:") && strings.TrimSpace(strings.TrimPrefix(line, "event:")) == "error"
}

func openAIPolicySSEEventHeader(line string, pendingErrorEvent, normalizedPolicyEvent bool) string {
	if normalizedPolicyEvent {
		return "event: response.failed\n" + line
	}
	if pendingErrorEvent {
		return "event: error\n" + line
	}
	return line
}

// API gateways may wrap a policy rejection in an HTTP 200 stream. Stop with
// the inbound protocol's error envelope instead of manufacturing a success or
// treating the error as an interrupted transport.
func writeOpenAIUpstreamPolicyErrorForEndpoint(c *gin.Context, status int, payload []byte) (error, bool) {
	// Some upstream gateways send SSE errors even for stream=false requests.
	if bodyHasSSEFraming(payload) {
		if failure, found := extractOpenAISSEFailureEvent(string(payload)); found {
			payload = failure
		}
	}
	detail, policy := openAIUpstreamPolicyErrorDetails(payload)
	if !policy {
		return nil, false
	}
	protocol := APIProtocolResponses
	if c != nil && c.Request != nil && c.Request.URL != nil {
		switch {
		case strings.HasSuffix(strings.TrimRight(c.Request.URL.Path, "/"), "/messages"):
			protocol = APIProtocolAnthropic
		case strings.HasSuffix(strings.TrimRight(c.Request.URL.Path, "/"), "/chat/completions"):
			protocol = APIProtocolChatCompletions
		}
	}
	if c == nil || c.Writer == nil {
		return fmt.Errorf("upstream response failed: %s", detail.Message), true
	}
	if protocol != APIProtocolAnthropic && !c.Writer.Written() {
		return writeOpenAIUpstreamPolicyError(c, status, payload)
	}
	markOpenAICyberPolicyEvent(c, payload, status, nil)
	setOpsUpstreamError(c, status, detail.Message, "")
	MarkResponseCommitted(c)
	if !c.Writer.Written() {
		writeAnthropicError(c, openAIPolicyClientStatus(status), detail.Type, detail.Message)
	} else {
		switch protocol {
		case APIProtocolResponses:
			writeOpenAICompactSSEFailureMessage(c, openAIPolicyClientStatus(status), detail.Code, detail.Message)
		case APIProtocolAnthropic:
			_, _ = fmt.Fprint(c.Writer, buildAnthropicStreamErrorSSE(detail.Type, detail.Message))
			c.Writer.Flush()
		default:
			body, _ := json.Marshal(gin.H{"error": detail})
			_, _ = fmt.Fprintf(c.Writer, "data: %s\n\ndata: [DONE]\n\n", body)
			c.Writer.Flush()
		}
	}
	return fmt.Errorf("upstream response failed: %s", detail.Message), true
}

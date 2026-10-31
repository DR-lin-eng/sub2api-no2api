package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const openAIWSHTTPRecoveryAccountKey = "openai_ws_http_recovery_account"

// HTTP recovery uses the immutable body, not the WS payload. The prewarm
// path rewrites nested roles and adds a connection-local previous_response_id.
func (s *OpenAIGatewayService) openAIWSHTTPTransportAllowed(ctx context.Context, c *gin.Context, account *Account) bool {
	if !s.openAIWSHTTPBridgeEnabled() || account == nil || !account.IsOpenAI() || account.IsOpenAIAgentIdentity() ||
		(ctx != nil && ctx.Err() != nil) || c == nil || c.Request == nil || c.Request.Context().Err() != nil ||
		GetOpenAIClientTransport(c) == OpenAIClientTransportWS ||
		s.IsDistillationGroupRequest(c, account) || codexContinuationRecoveryForbidden(c) || codexContinuationRequiresExactConnection(c) ||
		IsResponseCommitted(c) || openAIStreamClientOutputStarted(c, false) {
		return false
	}
	return true
}

func (s *OpenAIGatewayService) openAIWSHTTPRoutingAllowed(ctx context.Context, c *gin.Context, account *Account, body, canonicalBody []byte) bool {
	return s.openAIWSHTTPTransportAllowed(ctx, c, account) &&
		openAIWSHTTPFullInputBody(body) && openAIWSHTTPFullInputBody(canonicalBody)
}

func (s *OpenAIGatewayService) openAIWSHTTPRecoveryAllowed(ctx context.Context, c *gin.Context, account *Account, body, canonicalBody []byte) bool {
	return s.openAIWSHTTPTransportAllowed(ctx, c, account) &&
		openAIWSHTTPReplaySafeBody(body) && openAIWSHTTPReplaySafeBody(canonicalBody)
}

// Choosing HTTP before a WS send cannot duplicate hosted tools or stored
// responses. Only continuation state can prevent this initial transport choice.
func openAIWSHTTPFullInputBody(body []byte) bool {
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() ||
		!openAIWSHTTPUnambiguousFields(gjson.ParseBytes(body), "store", "background", "conversation", "previous_response_id", "input", "tools") {
		return false
	}
	classification := classifyCodexContinuationBody(body)
	return classification.parseErr == nil && classification.kind == codexContinuationFull
}

func openAIWSHTTPReplaySafeBody(body []byte) bool {
	if !openAIWSHTTPFullInputBody(body) ||
		(gjson.GetBytes(body, "store").Exists() && gjson.GetBytes(body, "store").Type != gjson.False) ||
		(gjson.GetBytes(body, "background").Exists() && gjson.GetBytes(body, "background").Type != gjson.False) ||
		gjson.GetBytes(body, "conversation").Exists() {
		return false
	}
	if !openAIWSHTTPReplaySafeTools(gjson.GetBytes(body, "tools")) {
		return false
	}
	// Responses Lite can carry hosted tools inside input.additional_tools.
	if input := gjson.GetBytes(body, "input"); input.IsArray() {
		for _, item := range input.Array() {
			if !openAIWSHTTPUnambiguousFields(item, "type", "tools") {
				return false
			}
			if item.Get("type").String() == "additional_tools" && !openAIWSHTTPReplaySafeTools(item.Get("tools")) {
				return false
			}
		}
	}
	return true
}

func openAIWSHTTPReplaySafeTools(tools gjson.Result) bool {
	return openAIWSHTTPReplaySafeToolsAtDepth(tools, 0)
}

func openAIWSHTTPReplaySafeToolsAtDepth(tools gjson.Result, depth int) bool {
	if !tools.Exists() {
		return true
	}
	if !tools.IsArray() || depth > 16 {
		return false
	}
	for _, tool := range tools.Array() {
		if !openAIWSHTTPUnambiguousFields(tool, "type", "tools") {
			return false
		}
		switch tool.Get("type").String() {
		case "function", "custom":
			// Client-executed tool declarations cannot run before their call
			// has been delivered. Paired historical calls/results stay intact.
		case "namespace":
			if !openAIWSHTTPReplaySafeToolsAtDepth(tool.Get("tools"), depth+1) {
				return false
			}
		default:
			// Hosted tools may already have run despite no downstream output.
			return false
		}
	}
	return true
}

func openAIWSHTTPUnambiguousFields(object gjson.Result, fields ...string) bool {
	if !object.IsObject() {
		return false
	}
	seen := make(map[string]bool, len(fields))
	ok := true
	object.ForEach(func(key, _ gjson.Result) bool {
		name := strings.ToLower(key.String())
		for _, field := range fields {
			if name == field {
				if seen[name] || name != key.String() {
					ok = false
					return false
				}
				seen[name] = true
			}
		}
		return true
	})
	return ok
}

func (s *OpenAIGatewayService) shouldBypassOpenAIWSForSize(ctx context.Context, c *gin.Context, account *Account, body, canonicalBody []byte) bool {
	requestAlreadyFellBack := false
	if c != nil && account != nil {
		value, _ := c.Get(openAIWSHTTPRecoveryAccountKey)
		id, _ := value.(int64)
		requestAlreadyFellBack = id == account.ID
	}
	if requestAlreadyFellBack {
		return s.openAIWSHTTPRecoveryAllowed(ctx, c, account, body, canonicalBody)
	}
	return s.shouldRouteOpenAIWSHTTPPayload(body) && s.openAIWSHTTPRoutingAllowed(ctx, c, account, body, canonicalBody)
}

func (s *OpenAIGatewayService) shouldRouteOpenAIWSHTTPPayload(payload []byte) bool {
	return s.openAIWSHTTPBridgeEnabled() &&
		(int64(len(payload)) >= s.openAIWSHTTPBridgeThresholdBytes() || openAIWSHasInlineImageInput(payload))
}

// Inline image bytes can exceed the upstream's WS limit independently of token
// count or the configured text threshold. Inspect only structured input parts;
// text, tool declarations and remote URLs must not trigger this route.
func openAIWSHasInlineImageInput(payload []byte) bool {
	var hasImage func(gjson.Result, int) bool
	hasImage = func(value gjson.Result, depth int) bool {
		if depth > 8 {
			return false
		}
		if value.IsArray() {
			found := false
			value.ForEach(func(_, item gjson.Result) bool {
				found = hasImage(item, depth+1)
				return !found
			})
			return found
		}
		if !value.IsObject() {
			return false
		}
		switch value.Get("type").String() {
		case "input_image", "image_url":
			imageURL := value.Get("image_url")
			if imageURL.IsObject() {
				imageURL = imageURL.Get("url")
			}
			url := strings.TrimSpace(imageURL.String())
			const prefix = "data:image/"
			if len(url) >= len(prefix) && strings.EqualFold(url[:len(prefix)], prefix) {
				return true
			}
		}
		return hasImage(value.Get("content"), depth+1) || hasImage(value.Get("output"), depth+1)
	}
	return hasImage(gjson.GetBytes(payload, "input"), 0)
}

func (s *OpenAIGatewayService) prepareOpenAIWSHTTPFallback(ctx context.Context, c *gin.Context, account *Account, wsErr error, body, canonicalBody []byte) bool {
	var fallback *openAIWSFallbackError
	if errors.As(wsErr, &fallback) && fallback != nil {
		reason := strings.TrimPrefix(strings.TrimSpace(fallback.Reason), "prewarm_")
		recoverable := reason == "message_too_big" || reason == "read_event" || reason == "write_request" ||
			reason == "write" || reason == "missing_final_response"
		if recoverable {
			if !s.openAIWSHTTPRecoveryAllowed(ctx, c, account, body, canonicalBody) {
				return false
			}
			transportReason := "ws_transport_retry_http"
			message := "Upstream WebSocket disconnected before output; recovering through HTTP"
			if reason == "message_too_big" {
				transportReason = "ws_message_too_big_http"
				message = "Upstream WebSocket closed with code 1009; recovering through HTTP"
			}
			s.markOpenAIWSHTTPRecovery(c, account, body, transportReason)
			appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
				Platform: account.Platform, AccountID: account.ID, AccountName: account.Name,
				UpstreamStatusCode: http.StatusBadGateway, Kind: "ws_http_fallback", Stage: "websocket",
				Reason: reason, Message: message,
			})
			return true
		}
	}
	if !shouldFallbackOpenAIWSToHTTP(wsErr) && !shouldFallbackCodexPrewarmWSForbiddenToHTTP(account, wsErr) {
		return false
	}
	s.markOpenAIWSFallbackCooling(account.ID, "websocket_unavailable")
	if c != nil {
		c.Set("openai_ws_transport_decision", string(OpenAIUpstreamTransportHTTPSSE))
		c.Set("openai_ws_transport_reason", "ws_fallback_http_sse")
	}
	logOpenAIWSModeInfo("fallback_http_sse account_id=%d reason=upstream_websocket_unavailable cooldown=%s", account.ID, s.openAIWSFallbackCooldown())
	return true
}

func (s *OpenAIGatewayService) markOpenAIWSHTTPRecovery(c *gin.Context, account *Account, body []byte, reason string) {
	c.Set(openAIWSHTTPRecoveryAccountKey, account.ID)
	c.Set("openai_ws_transport_decision", string(OpenAIUpstreamTransportHTTPSSE))
	c.Set("openai_ws_transport_reason", reason)
	logOpenAIWSModeInfo("fallback_http_sse account_id=%d reason=%s request_bytes=%d input_bytes=%d tools_bytes=%d threshold_bytes=%d",
		account.ID, reason, len(body), len(gjson.GetBytes(body, "input").Raw), len(gjson.GetBytes(body, "tools").Raw), s.openAIWSHTTPBridgeThresholdBytes())
}

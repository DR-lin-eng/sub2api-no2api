package service

import (
	"context"
	"errors"
	"net/http"
	"strings"
)

const openAIWSAuthenticationFailureClientMessage = "Upstream websocket authentication failed"

func openAIWSDialAuthStatus(err error) int {
	if err == nil {
		return 0
	}
	var fallbackErr *openAIWSFallbackError
	if errors.As(err, &fallbackErr) && fallbackErr != nil &&
		strings.TrimPrefix(strings.TrimSpace(fallbackErr.Reason), "prewarm_") != "auth_failed" {
		return 0
	}
	var dialErr *openAIWSDialError
	if !errors.As(err, &dialErr) || dialErr == nil {
		return 0
	}
	statusCode := dialErr.StatusCode
	if statusCode == 0 {
		errorText := dialErr.Error()
		if dialErr.Err != nil {
			errorText += " " + dialErr.Err.Error()
		}
		lowerErr := strings.ToLower(strings.TrimSpace(errorText))
		if strings.Contains(lowerErr, "got 401") || strings.Contains(lowerErr, "status=401") {
			statusCode = http.StatusUnauthorized
		} else if strings.Contains(lowerErr, "got 403") || strings.Contains(lowerErr, "status=403") {
			statusCode = http.StatusForbidden
		}
	}
	if statusCode != http.StatusUnauthorized && statusCode != http.StatusForbidden {
		return 0
	}
	return statusCode
}

// newOpenAIWSDialAuthFailover converts an upstream WS 401/403 handshake into
// the same account-failover contract used by HTTP inference. It deliberately
// does not expose the dialer's transport error, which can otherwise be written
// as JSON before the outer streaming handler emits response.failed.
func (s *OpenAIGatewayService) newOpenAIWSDialAuthFailover(
	ctx context.Context,
	account *Account,
	canonicalModel string,
	wsErr error,
) *UpstreamFailoverError {
	if wsErr == nil {
		return nil
	}
	var fallbackErr *openAIWSFallbackError
	var dialErr *openAIWSDialError
	if errors.As(wsErr, &fallbackErr) && fallbackErr != nil {
		if strings.TrimPrefix(strings.TrimSpace(fallbackErr.Reason), "prewarm_") != "auth_failed" {
			return nil
		}
		if !errors.As(fallbackErr.Err, &dialErr) || dialErr == nil {
			return nil
		}
	} else if !errors.As(wsErr, &dialErr) || dialErr == nil {
		return nil
	}
	statusCode := openAIWSDialAuthStatus(wsErr)
	if statusCode == 0 {
		return nil
	}

	responseBody := append([]byte(nil), dialErr.ResponseBody...)
	if len(responseBody) == 0 {
		responseBody = []byte(`{"error":{"type":"upstream_error","message":"Upstream websocket authentication failed"}}`)
	}
	// Agent Identity owns its own task recovery/disable policy. A generic WS 401
	// must not accidentally run the OAuth token-invalid policy against it.
	if account != nil && !account.IsOpenAIAgentIdentity() {
		s.handleOpenAIAccountUpstreamError(
			ctx,
			account,
			statusCode,
			dialErr.ResponseHeaders,
			responseBody,
			canonicalModel,
		)
	}

	safeBody, marshalErr := marshalOpenAIUpstreamJSON(map[string]any{
		"error": map[string]any{
			"type":    "upstream_error",
			"message": openAIWSAuthenticationFailureClientMessage,
		},
	})
	if marshalErr != nil {
		safeBody = []byte(`{"error":{"type":"upstream_error","message":"Upstream websocket authentication failed"}}`)
	}
	return &UpstreamFailoverError{
		StatusCode:        statusCode,
		ResponseBody:      safeBody,
		ResponseHeaders:   cloneHeader(dialErr.ResponseHeaders),
		Scope:             GatewayFailureScopeAccount,
		NextAccountAction: NextAccountRetry,
	}
}

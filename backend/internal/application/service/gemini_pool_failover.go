package service

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
)

const geminiCustomCodeSkippedClientMessage = "Upstream gateway error"

// poolModeSkippedFailoverError turns a skipped custom error policy into the
// standard pool retry/failover contract. A nil account or a status that is not
// eligible for failover leaves the existing passthrough behavior unchanged.
func (s *GeminiMessagesCompatService) poolModeSkippedFailoverError(
	c *gin.Context,
	account *Account,
	statusCode int,
	respBody []byte,
	upstreamRequestID string,
) *UpstreamFailoverError {
	if account == nil || !account.IsPoolMode() || !s.shouldFailoverGeminiUpstreamError(statusCode) {
		return nil
	}

	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
	upstreamDetail := ""
	if s != nil && s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(respBody), maxBytes)
	}
	if c != nil {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: statusCode,
			UpstreamRequestID:  upstreamRequestID,
			Kind:               "failover",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
		})
	}
	return &UpstreamFailoverError{
		StatusCode:             statusCode,
		ResponseBody:           respBody,
		RetryableOnSameAccount: account.IsPoolModeRetryableStatus(statusCode),
	}
}

// skippedErrorPolicyFailoverError applies the same failover decision to both
// pool-mode and custom-error-code-skipped accounts. Skipping local account
// state updates must not turn a retryable upstream failure into a hard 500.
func (s *GeminiMessagesCompatService) skippedErrorPolicyFailoverError(
	c *gin.Context,
	account *Account,
	statusCode int,
	respBody []byte,
	upstreamRequestID string,
) *UpstreamFailoverError {
	// Keep the established pool helper as the single implementation for pool
	// accounts while extending skipped-policy failover to custom-code accounts.
	if account != nil && account.IsPoolMode() {
		return s.poolModeSkippedFailoverError(c, account, statusCode, respBody, upstreamRequestID)
	}
	if account == nil || !s.shouldFailoverGeminiUpstreamError(statusCode) {
		return nil
	}

	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(respBody)))
	upstreamDetail := ""
	if s != nil && s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(respBody), maxBytes)
	}
	if c != nil {
		appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
			Platform:           account.Platform,
			AccountID:          account.ID,
			AccountName:        account.Name,
			UpstreamStatusCode: statusCode,
			UpstreamRequestID:  upstreamRequestID,
			Kind:               "failover",
			Message:            upstreamMsg,
			Detail:             upstreamDetail,
		})
	}
	return &UpstreamFailoverError{
		StatusCode:             statusCode,
		ResponseBody:           respBody,
		RetryableOnSameAccount: account.IsPoolMode() && account.IsPoolModeRetryableStatus(statusCode),
	}
}

// recordGeminiCustomCodeSkippedError records the real upstream failure while
// the client receives a stable generic error. This keeps custom-code policy
// details private without losing observability.
func (s *GeminiMessagesCompatService) recordGeminiCustomCodeSkippedError(
	c *gin.Context,
	account *Account,
	statusCode int,
	upstreamRequestID string,
	body []byte,
) error {
	if account == nil {
		return fmt.Errorf("gemini upstream error: %d (not in custom error codes)", statusCode)
	}
	upstreamMsg := sanitizeUpstreamErrorMessage(strings.TrimSpace(extractUpstreamErrorMessage(body)))
	upstreamDetail := ""
	if s != nil && s.cfg != nil && s.cfg.Gateway.LogUpstreamErrorBody {
		maxBytes := s.cfg.Gateway.LogUpstreamErrorBodyMaxBytes
		if maxBytes <= 0 {
			maxBytes = 2048
		}
		upstreamDetail = truncateString(string(body), maxBytes)
	}
	setOpsUpstreamError(c, statusCode, upstreamMsg, upstreamDetail)
	appendOpsUpstreamError(c, OpsUpstreamErrorEvent{
		Platform:           account.Platform,
		AccountID:          account.ID,
		AccountName:        account.Name,
		UpstreamStatusCode: statusCode,
		UpstreamRequestID:  upstreamRequestID,
		Kind:               "http_error",
		Message:            upstreamMsg,
		Detail:             upstreamDetail,
	})
	if upstreamMsg == "" {
		return fmt.Errorf("gemini upstream error: %d (not in custom error codes)", statusCode)
	}
	return fmt.Errorf("gemini upstream error: %d (not in custom error codes) message=%s", statusCode, upstreamMsg)
}

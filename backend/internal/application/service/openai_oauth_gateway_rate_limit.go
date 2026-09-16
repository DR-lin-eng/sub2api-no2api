package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/shared/ctxkey"
	"github.com/google/uuid"
)

const (
	openAIOAuthGatewayRateLimitReason             GatewayFailureReason = "openai_oauth_account_rate_limit"
	openAIOAuthGatewayRateLimitMessage                                 = "OpenAI OAuth account request rate limit exceeded"
	openAIOAuthGatewayRateLimitUnavailableMessage                      = "OpenAI OAuth account rate limiter is temporarily unavailable"
)

type OpenAIOAuthGatewayRateLimitDecision struct {
	Allowed           bool
	Existing          bool
	RetryAfterSeconds int
}

// OpenAIOAuthGatewayRateLimitCache stores one cluster-wide bucket per account.
// Every account uses the same global RPM/burst settings, while the state must
// not be partitioned by model or application instance.
type OpenAIOAuthGatewayRateLimitCache interface {
	AdmitOpenAIOAuthGatewayRequest(ctx context.Context, accountID int64, requestKey string, rpm, burst int) (OpenAIOAuthGatewayRateLimitDecision, error)
}

type openAIOAuthGatewayRateLimitError struct {
	retryAfterSeconds int
	unavailable       bool
	cause             error
}

func (e *openAIOAuthGatewayRateLimitError) Error() string {
	if e == nil {
		return "openai oauth gateway rate limit error"
	}
	if e.unavailable {
		return openAIOAuthGatewayRateLimitUnavailableMessage
	}
	return openAIOAuthGatewayRateLimitMessage
}

func (e *openAIOAuthGatewayRateLimitError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

type openAIOAuthGatewayRequestKeyContextKey struct{}

func withOpenAIOAuthGatewayTurnKey(ctx context.Context, turn int) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	base := openAIOAuthGatewayRequestKey(ctx)
	return context.WithValue(ctx, openAIOAuthGatewayRequestKeyContextKey{}, fmt.Sprintf("%s:turn:%d", base, turn))
}

func openAIOAuthGatewayRequestKey(ctx context.Context) string {
	if ctx != nil {
		if key, _ := ctx.Value(openAIOAuthGatewayRequestKeyContextKey{}).(string); strings.TrimSpace(key) != "" {
			return strings.TrimSpace(key)
		}
		if key, _ := ctx.Value(ctxkey.ClientRequestID).(string); strings.TrimSpace(key) != "" {
			return "http:" + strings.TrimSpace(key)
		}
		if key, _ := ctx.Value(ctxkey.RequestID).(string); strings.TrimSpace(key) != "" {
			return "request:" + strings.TrimSpace(key)
		}
	}
	return "generated:" + uuid.NewString()
}

func isOpenAIOAuthGatewayModelRequest(req *http.Request, account *Account) bool {
	if req == nil || req.URL == nil || account == nil || !account.IsOpenAIOAuth() || req.Method != http.MethodPost {
		return false
	}
	path := strings.TrimSpace(req.URL.Path)
	const responsesRoot = "/backend-api/codex/responses"
	return path == responsesRoot ||
		strings.HasPrefix(path, responsesRoot+"/") ||
		path == "/v1/responses/input_tokens" ||
		path == "/backend-api/codex/alpha/search" ||
		path == "/backend-api/codex/realtime/calls"
}

func (s *OpenAIGatewayService) openAIOAuthGatewayRateLimitSettings(ctx context.Context) (enabled bool, rpm, burst int) {
	settings := s.openAIAdvancedSchedulerRuntimeSettings(ctx)
	return settings.oauthGatewayRateLimitEnabled, settings.oauthGatewayRateLimitRPM, settings.oauthGatewayRateLimitBurst
}

func openAIOAuthGatewayRateLimitAccountID(account *Account) int64 {
	if account == nil {
		return 0
	}
	if account.ParentAccountID != nil && *account.ParentAccountID > 0 {
		return *account.ParentAccountID
	}
	return account.ID
}

func (s *OpenAIGatewayService) admitOpenAIOAuthGatewayModelRequest(ctx context.Context, account *Account) error {
	if s == nil || account == nil || !account.IsOpenAIOAuth() {
		return nil
	}
	enabled, rpm, burst := s.openAIOAuthGatewayRateLimitSettings(ctx)
	if !enabled {
		return nil
	}
	if rpm <= 0 || burst <= 0 || s.oauthGatewayRateLimitCache == nil {
		return &openAIOAuthGatewayRateLimitError{unavailable: true, cause: errors.New("gateway limiter is not configured")}
	}
	decision, err := s.oauthGatewayRateLimitCache.AdmitOpenAIOAuthGatewayRequest(ctx, openAIOAuthGatewayRateLimitAccountID(account), openAIOAuthGatewayRequestKey(ctx), rpm, burst)
	if err != nil {
		return &openAIOAuthGatewayRateLimitError{unavailable: true, cause: err}
	}
	if decision.Allowed {
		return nil
	}
	retryAfter := decision.RetryAfterSeconds
	if retryAfter < 1 {
		retryAfter = 1
	}
	return &openAIOAuthGatewayRateLimitError{retryAfterSeconds: retryAfter}
}

func openAIOAuthGatewayRateLimitFailover(err error) *UpstreamFailoverError {
	var limited *openAIOAuthGatewayRateLimitError
	if !errors.As(err, &limited) || limited == nil {
		return nil
	}
	status := http.StatusTooManyRequests
	message := openAIOAuthGatewayRateLimitMessage
	scope := GatewayFailureScopeAccount
	action := NextAccountRetry
	if limited.unavailable {
		status = http.StatusServiceUnavailable
		message = openAIOAuthGatewayRateLimitUnavailableMessage
		scope = GatewayFailureScopeRequest
		action = NextAccountStop
	}
	headers := make(http.Header)
	if limited.retryAfterSeconds > 0 {
		headers.Set("Retry-After", strconv.Itoa(limited.retryAfterSeconds))
	}
	return &UpstreamFailoverError{
		StatusCode:        status,
		ResponseHeaders:   headers,
		Scope:             scope,
		Reason:            openAIOAuthGatewayRateLimitReason,
		NextAccountAction: action,
		ClientStatusCode:  status,
		ClientMessage:     message,
	}
}

func openAIOAuthGatewayRateLimitWSFailover(err error, turn int, payload []byte) error {
	failoverErr := openAIOAuthGatewayRateLimitFailover(err)
	if failoverErr == nil {
		return err
	}
	if turn > 1 && failoverErr.ShouldRetryNextAccount() {
		return newOpenAIWSCurrentTurnFailoverError(failoverErr, payload)
	}
	return failoverErr
}

func (e *UpstreamFailoverError) IsOpenAIOAuthGatewayRateLimit() bool {
	return e != nil && e.Reason == openAIOAuthGatewayRateLimitReason
}

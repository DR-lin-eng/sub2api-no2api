package service

import (
	"context"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/gin-gonic/gin"
)

const openAITransportTLSHandshakeRetryBackoff = 250 * time.Millisecond

// One recovery window per Forward attempt, independent of OAuth refresh and
// request-validation retries. It does not reset while rebuilding requests.
type openAITLSHandshakeRecovery struct {
	tried   bool
	written *atomic.Bool
}

func (r *openAITLSHandshakeRecovery) trace(req *http.Request) *http.Request {
	// A redirect or transport-internal retry can fail TLS after an earlier
	// request was sent. Do not replay that request's possible side effects.
	written := &atomic.Bool{}
	r.written = written
	return req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
		WroteHeaders: func() { written.Store(true) },
		WroteRequest: func(httptrace.WroteRequestInfo) { written.Store(true) },
	}))
}

func (r *openAITLSHandshakeRecovery) retry(ctx context.Context, c *gin.Context, account *Account, err error) (bool, error) {
	if r.tried || r.written == nil || r.written.Load() ||
		account == nil || !account.IsOpenAI() || isDistillationGroupRequest(c, account) ||
		!isOpenAITransportTLSHandshakeTimeout(err) || ctx.Err() != nil ||
		OpenAICompactKeepaliveAdjustedWrittenSize(c) >= 0 {
		return false, nil
	}
	r.tried = true
	logger.LegacyPrintf("service.openai_gateway", "[OpenAI] Retrying TLS handshake timeout once: account_id=%d", account.ID)
	return true, waitOpenAITransportRetry(ctx, openAITransportTLSHandshakeRetryBackoff)
}

// Match the standard and fingerprint TLS timeout errors. A generic context
// deadline or response-header timeout does not prove that no request was sent.
func isOpenAITransportTLSHandshakeTimeout(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "tls handshake timeout") {
		return true
	}
	return strings.Contains(message, "tls handshake") &&
		(strings.Contains(message, "deadline exceeded") || strings.Contains(message, "i/o timeout"))
}

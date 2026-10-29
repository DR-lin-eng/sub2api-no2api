//go:build unit

package service

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGeminiSameAccountRetriesHonorZeroAndOneAcrossProtocols(t *testing.T) {
	for _, retries := range []int{0, 1} {
		for _, forwarder := range geminiSingleCheckForwarders {
			t.Run(fmt.Sprintf("%s/%d", forwarder.name, retries), func(t *testing.T) {
				svc, upstream, _ := newGeminiSingleCheckService(geminiSequenceUpstreamResponse{
					status: http.StatusServiceUnavailable,
					body:   `{"error":{"code":503,"message":"overloaded","status":"UNAVAILABLE"}}`,
				})
				svc.cfg.Gateway.MaxSameAccountRetriesGemini = retries
				account := &Account{ID: 8287, Platform: PlatformGemini, Type: AccountTypeAPIKey,
					Credentials: map[string]any{"api_key": "fixture-key"}}
				err, _ := forwarder.forward(t, svc, account)
				var failover *UpstreamFailoverError
				require.True(t, errors.As(err, &failover))
				require.Equal(t, retries+1, upstream.calls)
			})
		}
	}
}

func TestGeminiSignatureDowngradesDoNotConsumeTransientRetryBudget(t *testing.T) {
	signatureFailure := geminiSequenceUpstreamResponse{status: http.StatusBadRequest,
		body: `{"error":{"code":400,"message":"Function call is missing a thought_signature in functionCall parts.","status":"INVALID_ARGUMENT"}}`}
	success := geminiSequenceUpstreamResponse{status: http.StatusOK,
		body: `{"candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":3,"candidatesTokenCount":1}}`}
	svc, upstream, _ := newGeminiSingleCheckService(signatureFailure, signatureFailure, success)
	svc.cfg.Gateway.MaxSameAccountRetriesGemini = 0
	account := &Account{ID: 8287, Platform: PlatformGemini, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "fixture-key"}}
	err, _ := geminiSingleCheckForwarders[1].forward(t, svc, account)
	require.NoError(t, err)
	require.Equal(t, 3, upstream.calls)
}

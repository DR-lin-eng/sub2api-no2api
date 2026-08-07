package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShouldFailoverUpstreamError_405IsFailoverEligible(t *testing.T) {
	svc := &OpenAIGatewayService{}
	require.True(t, svc.shouldFailoverUpstreamError(http.StatusMethodNotAllowed))
}

func TestShouldFailoverUpstreamError_ExistingBoundaries(t *testing.T) {
	svc := &OpenAIGatewayService{}
	for _, code := range []int{401, 402, 403, 405, 429, 529, 500, 502, 503, 504} {
		require.True(t, svc.shouldFailoverUpstreamError(code), "status %d", code)
	}
	for _, code := range []int{200, 201, 400, 404, 408, 422} {
		require.False(t, svc.shouldFailoverUpstreamError(code), "status %d", code)
	}
}

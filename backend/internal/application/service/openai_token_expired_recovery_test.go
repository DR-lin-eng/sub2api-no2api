package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExtractUpstreamErrorCode_TokenExpiredEnvelope(t *testing.T) {
	body := []byte(`{
  "detail": {"code": "token_expired", "message": "Your authentication token has expired."},
  "error": {"code": "token_expired", "message": "Your authentication token has expired.", "param": null, "type": "invalid_request_error"},
  "status": 401
}`)

	require.Equal(t, "token_expired", extractUpstreamErrorCode(body))
}

func TestIsOpenAIWSDialTokenExpired(t *testing.T) {
	err := &openAIWSDialError{
		StatusCode:   http.StatusUnauthorized,
		ResponseBody: []byte(`{"error":{"code":"token_expired"}}`),
	}

	require.True(t, isOpenAIWSDialTokenExpired(err))
	require.False(t, isOpenAIWSDialTokenExpired(&openAIWSDialError{
		StatusCode:   http.StatusUnauthorized,
		ResponseBody: []byte(`{"error":{"code":"token_revoked"}}`),
	}))
}

func TestIsOpenAIWSTokenExpiredEvent(t *testing.T) {
	require.True(t, isOpenAIWSTokenExpiredEvent([]byte(`{"type":"error","error":{"code":"token_expired"}}`)))
	require.False(t, isOpenAIWSTokenExpiredEvent([]byte(`{"type":"error","error":{"code":"token_revoked"}}`)))
}

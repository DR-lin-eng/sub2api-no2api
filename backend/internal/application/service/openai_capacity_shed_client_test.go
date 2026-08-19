package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSanitizeOpenAICapacityShedErrorCodeForClient(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		wantChanged bool
		wantCode    string
	}{
		{
			name:        "nested response error",
			payload:     `{"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"overloaded"}}}`,
			wantChanged: true,
			wantCode:    `"code":"server_error"`,
		},
		{
			name:        "top level error",
			payload:     `{"type":"error","error":{"code":"slow_down","message":"slow down"}}`,
			wantChanged: true,
			wantCode:    `"code":"server_error"`,
		},
		{
			name:        "rate limit remains intact",
			payload:     `{"type":"error","error":{"code":"rate_limit_exceeded","message":"limited"}}`,
			wantChanged: false,
			wantCode:    `"code":"rate_limit_exceeded"`,
		},
		{
			name:        "message-only capacity adds retryable code",
			payload:     `{"type":"response.failed","response":{"error":{"message":"Our servers are currently overloaded. Please try again later."}}}`,
			wantChanged: true,
			wantCode:    `"code":"server_error"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, changed := sanitizeOpenAICapacityShedErrorCodeForClient([]byte(tt.payload))
			require.Equal(t, tt.wantChanged, changed)
			require.Contains(t, string(out), tt.wantCode)
			if changed {
				require.NotContains(t, string(out), "server_is_overloaded")
				require.NotContains(t, string(out), "slow_down")
			}
		})
	}
}

func TestSanitizeOpenAIResponseFailedEventForClientPreservesRetryableCapacityCode(t *testing.T) {
	payload := []byte(`{"type":"response.failed","response":{"error":{"code":"server_is_overloaded","message":"overloaded"}}}`)

	out, changed := sanitizeOpenAIResponseFailedEventForClient(payload, "response.failed", true)

	require.True(t, changed)
	require.Contains(t, string(out), `"code":"server_error"`)
	require.NotContains(t, string(out), "server_is_overloaded")
}

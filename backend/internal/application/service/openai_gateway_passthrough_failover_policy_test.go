package service

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestShouldFailoverOpenAIPassthroughResponseAvoidsRequestAmplification(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{name: "invalid request", status: http.StatusBadRequest, body: `{"error":{"type":"invalid_request_error","message":"bad input"}}`, want: false},
		{name: "context window", status: http.StatusBadRequest, body: `{"error":{"code":"context_length_exceeded","message":"input exceeds the context window"}}`, want: false},
		{name: "cyber policy", status: http.StatusForbidden, body: `{"error":{"code":"cyber_policy","message":"flagged"}}`, want: false},
		{name: "request conflict", status: http.StatusConflict, body: `{"error":{"message":"conflict"}}`, want: false},
		{name: "unprocessable request", status: http.StatusUnprocessableEntity, body: `{"error":{"message":"invalid field"}}`, want: false},
		{name: "credential rejected", status: http.StatusUnauthorized, body: `{"error":{"message":"unauthorized"}}`, want: true},
		{name: "account billing rejected", status: http.StatusPaymentRequired, body: `{"error":{"message":"billing disabled"}}`, want: true},
		{name: "account forbidden", status: http.StatusForbidden, body: `{"error":{"message":"forbidden"}}`, want: true},
		{name: "model missing", status: http.StatusNotFound, body: `{"error":{"message":"not found"}}`, want: true},
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{"error":{"message":"limited"}}`, want: true},
		{name: "server error", status: http.StatusBadGateway, body: `{"error":{"message":"bad gateway"}}`, want: true},
		{name: "account body limit", status: http.StatusRequestEntityTooLarge, body: `{"error":{"message":"request body is too large"}}`, want: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, shouldFailoverOpenAIPassthroughResponse(nil, tc.status, []byte(tc.body)))
		})
	}
}

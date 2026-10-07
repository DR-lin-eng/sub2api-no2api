package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/shared/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestUsageBillingRequestIDIgnoresCorrelationIDs(t *testing.T) {
	for _, clientID := range []string{"anything", " local:chosen ", "订阅请求"} {
		t.Run(clientID, func(t *testing.T) {
			ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, clientID)
			ctx = context.WithValue(ctx, ctxkey.RequestID, "attacker-fixed")
			first := context.WithValue(ctx, ctxkey.UsageBillingRequestID, "server-first")
			second := context.WithValue(ctx, ctxkey.UsageBillingRequestID, "server-second")
			require.Equal(t, "local:server-first", resolveUsageBillingRequestID(first, "upstream-fixed"))
			require.Equal(t, "local:server-second", resolveUsageBillingRequestID(second, "upstream-fixed"))
			require.Equal(t, "local:server-first", resolveUsageBillingPayloadFingerprint(first, ""))
			require.Equal(t, "payload-hash", resolveUsageBillingPayloadFingerprint(first, "payload-hash"))
		})
	}
}

func TestUsageBillingRequestIDWithoutTrustedContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "anything")
	ctx = context.WithValue(ctx, ctxkey.RequestID, "anything")
	first := resolveUsageBillingRequestID(ctx, "")
	second := resolveUsageBillingRequestID(ctx, "")
	// Correlation metadata must not turn separate billable operations into retries.
	require.Contains(t, first, "generated:")
	require.NotEqual(t, first, second)
	require.Empty(t, resolveUsageBillingPayloadFingerprint(ctx, ""))
	// Background jobs retain the existing explicit, stable settlement ID contract.
	require.Equal(t, "server-job-id", resolveUsageBillingRequestID(nil, "server-job-id"))
}

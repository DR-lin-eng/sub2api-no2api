package handler

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/shared/ctxkey"
	"github.com/stretchr/testify/require"
)

func TestSubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "request-456")
	parent = context.WithValue(parent, ctxkey.UsageBillingRequestID, "server-billing-789")

	var gotClientRequestID string
	var gotRequestID string
	var gotBillingID string
	h := &GatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
		gotBillingID, _ = ctx.Value(ctxkey.UsageBillingRequestID).(string)
	})

	require.Equal(t, "client-request-123", gotClientRequestID)
	require.Equal(t, "request-456", gotRequestID)
	require.Equal(t, "server-billing-789", gotBillingID)
}

func TestOpenAISubmitUsageRecordTaskCopiesRequestContext(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.ClientRequestID, "openai-client-request-123")
	parent = context.WithValue(parent, ctxkey.RequestID, "openai-request-456")
	parent = context.WithValue(parent, ctxkey.UsageBillingRequestID, "openai-server-billing-789")

	var gotClientRequestID string
	var gotRequestID string
	var gotBillingID string
	h := &OpenAIGatewayHandler{}
	h.submitUsageRecordTask(parent, func(ctx context.Context) {
		gotClientRequestID, _ = ctx.Value(ctxkey.ClientRequestID).(string)
		gotRequestID, _ = ctx.Value(ctxkey.RequestID).(string)
		gotBillingID, _ = ctx.Value(ctxkey.UsageBillingRequestID).(string)
	})

	require.Equal(t, "openai-client-request-123", gotClientRequestID)
	require.Equal(t, "openai-request-456", gotRequestID)
	require.Equal(t, "openai-server-billing-789", gotBillingID)
}

func TestOpenAIWSTurnUsageContextSeparatesTurnsAndPreservesRetries(t *testing.T) {
	parent := context.WithValue(context.Background(), ctxkey.UsageBillingRequestID, "server-connection")
	parent = context.WithValue(parent, ctxkey.RequestID, "attacker-fixed")
	first := openAIWSTurnUsageContext(parent, 1)
	retry := openAIWSTurnUsageContext(parent, 1)
	second := openAIWSTurnUsageContext(parent, 2)
	require.Equal(t, "server-connection:turn:1", first.Value(ctxkey.UsageBillingRequestID))
	require.Equal(t, first.Value(ctxkey.UsageBillingRequestID), retry.Value(ctxkey.UsageBillingRequestID))
	require.Equal(t, "server-connection:turn:2", second.Value(ctxkey.UsageBillingRequestID))
	require.Equal(t, "server-connection", parent.Value(ctxkey.UsageBillingRequestID))
	worker := usageRecordContext(second, context.Background())
	require.Equal(t, second.Value(ctxkey.UsageBillingRequestID), worker.Value(ctxkey.UsageBillingRequestID))
	// A relay restart after a later-turn failover begins again at turn 1.
	// It must not reuse any completed turn from the previous proxy attempt.
	firstAttempt := openAIWSAttemptUsageContext(parent)
	secondAttempt := openAIWSAttemptUsageContext(parent)
	firstAttemptTurn := openAIWSTurnUsageContext(firstAttempt, 1)
	secondAttemptTurn := openAIWSTurnUsageContext(secondAttempt, 1)
	require.NotEqual(t, firstAttemptTurn.Value(ctxkey.UsageBillingRequestID), secondAttemptTurn.Value(ctxkey.UsageBillingRequestID))
	require.Equal(t, firstAttemptTurn.Value(ctxkey.UsageBillingRequestID), openAIWSTurnUsageContext(firstAttempt, 1).Value(ctxkey.UsageBillingRequestID))
	for _, fallbackParent := range []context.Context{nil, context.Background()} {
		require.NotEmpty(t, openAIWSTurnUsageContext(fallbackParent, 1).Value(ctxkey.UsageBillingRequestID))
		require.NotEmpty(t, openAIWSAttemptUsageContext(fallbackParent).Value(ctxkey.UsageBillingRequestID))
	}
}

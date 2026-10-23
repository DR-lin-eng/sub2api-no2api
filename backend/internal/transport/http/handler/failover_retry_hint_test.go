package handler

import (
	"context"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestSameAccountRetryDelayForHint(t *testing.T) {
	for _, tc := range []struct {
		name    string
		hint    time.Duration
		count   int
		request bool
		want    time.Duration
	}{
		{"first retry hint", 6 * time.Second, 1, true, 6 * time.Second},
		{"hint floor", 6 * time.Second, 3, true, 6 * time.Second},
		{"retain exponential", time.Second, 3, true, 2 * time.Second},
		{"retain first retry", time.Millisecond, 1, true, 500 * time.Millisecond},
		{"cap hint", time.Hour, 1, true, 8 * time.Second},
		{"cap retry", time.Second, 1000, true, 8 * time.Second},
		{"ignore negative", -time.Second, 1, true, 500 * time.Millisecond},
		{"ordinary account failure", time.Hour, 10, false, 500 * time.Millisecond},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := &service.UpstreamFailoverError{SameAccountRetryDelay: tc.hint}
			if tc.request {
				e.Scope = service.GatewayFailureScopeRequest
			}
			require.Equal(t, tc.want, sameAccountRetryDelayFor(e, tc.count))
		})
	}
}

func TestHandleFailoverError_CapacityRetryHintCanceled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	fs := NewFailoverState(3, false)
	mock := &mockTempUnscheduler{}
	failure := &service.UpstreamFailoverError{
		Scope: service.GatewayFailureScopeRequest, RetryableOnSameAccount: true,
		SameAccountRetryDelay: 8 * time.Second,
	}
	start := time.Now()
	require.Equal(t, FailoverCanceled, fs.HandleFailoverError(ctx, mock, 100, "openai", 1, failure))
	require.Less(t, time.Since(start), time.Second)
	require.Equal(t, 1, fs.SameAccountRetryCount[100])
	require.Zero(t, fs.SwitchCount)
	require.Empty(t, mock.calls)
}

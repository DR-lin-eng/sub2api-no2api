//go:build unit

package service

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type insufficientBalanceAccountRepoStub struct {
	rateLimitAccountRepoStub
	setSchedulableCalls int
	lastSchedulableID   int64
	lastSchedulable     bool
	setSchedulableErr   error
}

func (r *insufficientBalanceAccountRepoStub) SetSchedulable(_ context.Context, id int64, schedulable bool) error {
	r.setSchedulableCalls++
	r.lastSchedulableID = id
	r.lastSchedulable = schedulable
	return r.setSchedulableErr
}

func TestIsUpstreamInsufficientBalanceError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		statusCode int
		body       string
		want       bool
	}{
		{
			name:       "known billing error envelope",
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"message":"insufficient balance","type":"billing_error"},"type":"error"}`,
			want:       true,
		},
		{
			name:       "direct structured code",
			statusCode: http.StatusForbidden,
			body:       `{"error":{"code":"INSUFFICIENT_BALANCE","message":"request rejected"}}`,
			want:       true,
		},
		{
			name:       "nested JSON message",
			statusCode: http.StatusBadGateway,
			body:       `{"error":{"message":"{\"error\":{\"type\":\"billing_error\",\"message\":\"credit balance depleted\"}}"}}`,
			want:       true,
		},
		{
			name:       "plain text credits",
			statusCode: http.StatusTooManyRequests,
			body:       `Upstream rejected the request: insufficient credits`,
			want:       true,
		},
		{
			name:       "localized balance message",
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"message":"账户余额不足，请充值"}}`,
			want:       true,
		},
		{
			name:       "successful response never matches",
			statusCode: http.StatusOK,
			body:       `{"error":{"type":"billing_error","message":"insufficient balance"}}`,
			want:       false,
		},
		{
			name:       "rolling quota exhaustion is not balance",
			statusCode: http.StatusTooManyRequests,
			body:       `{"error":{"type":"rate_limit_error","code":"quota_exhausted","message":"weekly quota exhausted"}}`,
			want:       false,
		},
		{
			name:       "billing quota code without balance evidence is not enough",
			statusCode: http.StatusTooManyRequests,
			body:       `{"error":{"code":"billing_quota_exceeded","message":"weekly quota exhausted"}}`,
			want:       false,
		},
		{
			name:       "insufficient quota is not monetary balance",
			statusCode: http.StatusTooManyRequests,
			body:       `{"error":{"message":"insufficient quota for this model"}}`,
			want:       false,
		},
		{
			name:       "unrelated billing validation",
			statusCode: http.StatusBadRequest,
			body:       `{"error":{"type":"billing_error","message":"billing address is invalid"}}`,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, isUpstreamInsufficientBalanceError(tt.statusCode, []byte(tt.body)))
		})
	}
}

func TestRateLimitServiceInsufficientBalanceAutoDisable(t *testing.T) {
	t.Run("missing switch keeps existing behavior", func(t *testing.T) {
		repo := &insufficientBalanceAccountRepoStub{}
		svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		account := &Account{ID: 601, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Schedulable: true}

		shouldDisable := svc.HandleUpstreamError(
			context.Background(),
			account,
			http.StatusBadRequest,
			http.Header{},
			[]byte(`{"error":{"type":"billing_error","message":"insufficient balance"}}`),
		)

		require.False(t, shouldDisable)
		require.Zero(t, repo.setSchedulableCalls)
		require.True(t, account.Schedulable)
	})

	t.Run("enabled switch preempts pool and custom error exclusions", func(t *testing.T) {
		repo := &insufficientBalanceAccountRepoStub{}
		blocker := &runtimeBlockRecorder{}
		svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		svc.SetAccountRuntimeBlocker(blocker)
		account := &Account{
			ID:          602,
			Platform:    PlatformOpenAI,
			Type:        AccountTypeAPIKey,
			Schedulable: true,
			Credentials: map[string]any{
				"pool_mode":                  true,
				"custom_error_codes_enabled": true,
				"custom_error_codes":         []any{float64(http.StatusTooManyRequests)},
			},
			Extra: map[string]any{AutoDisableOnUpstreamInsufficientBalanceExtraKey: true},
		}

		shouldDisable := svc.HandleUpstreamError(
			context.Background(),
			account,
			http.StatusBadRequest,
			http.Header{},
			[]byte(`{"error":{"type":"billing_error","message":"insufficient balance"}}`),
		)

		require.True(t, shouldDisable)
		require.Equal(t, 1, repo.setSchedulableCalls)
		require.Equal(t, account.ID, repo.lastSchedulableID)
		require.False(t, repo.lastSchedulable)
		require.True(t, account.Schedulable, "request snapshots remain immutable after persistence")
		require.Len(t, blocker.accounts, 1)
		require.Equal(t, "upstream_insufficient_balance", blocker.reasons[0])
		require.Zero(t, repo.setErrorCalls)
	})

	t.Run("persistence failure still fails over without a permanent runtime block", func(t *testing.T) {
		repo := &insufficientBalanceAccountRepoStub{setSchedulableErr: errors.New("db unavailable")}
		blocker := &runtimeBlockRecorder{}
		svc := NewRateLimitService(repo, nil, &config.Config{}, nil, nil)
		svc.SetAccountRuntimeBlocker(blocker)
		account := &Account{
			ID:          603,
			Platform:    PlatformAnthropic,
			Type:        AccountTypeAPIKey,
			Schedulable: true,
			Extra:       map[string]any{AutoDisableOnUpstreamInsufficientBalanceExtraKey: true},
		}

		shouldDisable := svc.HandleUpstreamError(
			context.Background(), account, http.StatusBadRequest, http.Header{},
			[]byte(`{"error":{"type":"billing_error","message":"insufficient balance"}}`),
		)

		require.True(t, shouldDisable)
		require.Equal(t, 1, repo.setSchedulableCalls)
		require.True(t, account.Schedulable)
		require.Empty(t, blocker.accounts)
	})
}

func TestAccountAutoDisableOnUpstreamInsufficientBalanceEnabled(t *testing.T) {
	t.Parallel()
	require.False(t, (&Account{}).AutoDisableOnUpstreamInsufficientBalanceEnabled())
	require.False(t, (&Account{Extra: map[string]any{AutoDisableOnUpstreamInsufficientBalanceExtraKey: "true"}}).AutoDisableOnUpstreamInsufficientBalanceEnabled())
	require.True(t, (&Account{Extra: map[string]any{AutoDisableOnUpstreamInsufficientBalanceExtraKey: true}}).AutoDisableOnUpstreamInsufficientBalanceEnabled())
}

//go:build unit

package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHandleGrokAccountUpstreamErrorMultiAgentCapacityBlocksOnlyModel(t *testing.T) {
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 9120, Platform: PlatformGrok, Type: AccountTypeOAuth}
	model := "grok-4.20-multi-agent-0309"
	ctx := withGrokTeamRateLimitModel(context.Background(), model)

	svc.handleGrokAccountUpstreamError(
		ctx,
		account,
		http.StatusBadGateway,
		nil,
		[]byte(`{"error":{"message":"engine_overloaded"}}`),
	)

	require.Zero(t, repo.tempUnschedCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.True(t, svc.isOpenAIAccountModelRuntimeBlocked(account, model))
	require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "grok-4.5"))
}

func TestHandleGrokAccountUpstreamErrorRegularModelCapacityKeepsExistingPolicy(t *testing.T) {
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{ID: 9121, Platform: PlatformGrok, Type: AccountTypeOAuth}
	ctx := withGrokTeamRateLimitModel(context.Background(), "grok-4.5")

	svc.handleGrokAccountUpstreamError(
		ctx,
		account,
		http.StatusBadGateway,
		nil,
		[]byte(`{"error":{"message":"engine_overloaded"}}`),
	)

	require.Equal(t, 1, repo.tempUnschedCalls)
	require.True(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, "grok-4.5"))
}

func TestHandleGrokAccountUpstreamErrorPoolModeKeepsCapacityWithPool(t *testing.T) {
	repo := &grokQuotaAccountRepo{}
	svc := &OpenAIGatewayService{accountRepo: repo}
	account := &Account{
		ID: 9122, Platform: PlatformGrok, Type: AccountTypeAPIKey,
		Credentials: map[string]any{"pool_mode": true},
	}
	model := "grok-4.20-multi-agent-0309"

	svc.handleGrokAccountUpstreamError(
		withGrokTeamRateLimitModel(context.Background(), model),
		account,
		http.StatusBadGateway,
		nil,
		[]byte(`{"error":{"message":"engine_overloaded"}}`),
	)

	require.Zero(t, repo.tempUnschedCalls)
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	require.False(t, svc.isOpenAIAccountModelRuntimeBlocked(account, model))
}

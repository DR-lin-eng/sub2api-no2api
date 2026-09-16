package service

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/ctxkey"
	"github.com/stretchr/testify/require"
)

type openAIOAuthGatewayRateLimitCacheStub struct {
	calls      int
	accountIDs []int64
	keys       []string
	rpms       []int
	bursts     []int
	decision   OpenAIOAuthGatewayRateLimitDecision
	err        error
}

func (s *openAIOAuthGatewayRateLimitCacheStub) AdmitOpenAIOAuthGatewayRequest(_ context.Context, accountID int64, key string, rpm, burst int) (OpenAIOAuthGatewayRateLimitDecision, error) {
	s.calls++
	s.accountIDs = append(s.accountIDs, accountID)
	s.keys = append(s.keys, key)
	s.rpms = append(s.rpms, rpm)
	s.bursts = append(s.bursts, burst)
	return s.decision, s.err
}

func withOpenAIOAuthGatewayRuntimeSettings(t *testing.T, enabled bool, rpm, burst int) {
	t.Helper()
	previous, _ := openAIAdvancedSchedulerSettingCache.Load().(*cachedOpenAIAdvancedSchedulerSetting)
	t.Cleanup(func() {
		if previous != nil {
			openAIAdvancedSchedulerSettingCache.Store(previous)
			return
		}
		openAIAdvancedSchedulerSettingCache.Store(&cachedOpenAIAdvancedSchedulerSetting{expiresAt: 0})
	})
	openAIAdvancedSchedulerSettingCache.Store(&cachedOpenAIAdvancedSchedulerSetting{
		oauthGatewayRateLimitEnabled: enabled,
		oauthGatewayRateLimitRPM:     rpm,
		oauthGatewayRateLimitBurst:   burst,
		expiresAt:                    time.Now().Add(time.Minute).UnixNano(),
	})
}

func TestOpenAIOAuthGatewayRateLimitCanBeDisabled(t *testing.T) {
	withOpenAIOAuthGatewayRuntimeSettings(t, false, 60, 5)
	cache := &openAIOAuthGatewayRateLimitCacheStub{decision: OpenAIOAuthGatewayRateLimitDecision{Allowed: false}}
	svc := &OpenAIGatewayService{oauthGatewayRateLimitCache: cache}
	err := svc.admitOpenAIOAuthGatewayModelRequest(context.Background(), &Account{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth})
	require.NoError(t, err)
	require.Zero(t, cache.calls)
}

func TestOpenAIOAuthGatewayRateLimitUsesPerAccountLogicalRequestKeys(t *testing.T) {
	withOpenAIOAuthGatewayRuntimeSettings(t, true, 60, 5)
	cache := &openAIOAuthGatewayRateLimitCacheStub{decision: OpenAIOAuthGatewayRateLimitDecision{Allowed: true}}
	svc := &OpenAIGatewayService{oauthGatewayRateLimitCache: cache}
	ctx := context.WithValue(context.Background(), ctxkey.ClientRequestID, "client-request")
	account := &Account{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.NoError(t, svc.admitOpenAIOAuthGatewayModelRequest(ctx, account))
	require.NoError(t, svc.admitOpenAIOAuthGatewayModelRequest(ctx, account))
	require.Equal(t, []string{"http:client-request", "http:client-request"}, cache.keys)
	require.Equal(t, []int64{11, 11}, cache.accountIDs)

	require.NoError(t, svc.admitOpenAIOAuthGatewayModelRequest(withOpenAIOAuthGatewayTurnKey(ctx, 2), account))
	require.Equal(t, "http:client-request:turn:2", cache.keys[2])

	otherAccount := &Account{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.NoError(t, svc.admitOpenAIOAuthGatewayModelRequest(ctx, otherAccount))
	require.Equal(t, []int64{11, 11, 11, 12}, cache.accountIDs)
	require.Equal(t, "http:client-request", cache.keys[3])

	parentID := int64(11)
	shadow := &Account{ID: 21, ParentAccountID: &parentID, Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.NoError(t, svc.admitOpenAIOAuthGatewayModelRequest(ctx, shadow))
	require.Equal(t, int64(11), cache.accountIDs[4])
	require.Equal(t, []int{60, 60, 60, 60, 60}, cache.rpms)
	require.Equal(t, []int{5, 5, 5, 5, 5}, cache.bursts)
}

func TestOpenAIOAuthGatewayRateLimitExcludesAPIKeysAndAuxiliaryRequests(t *testing.T) {
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	for _, path := range []string{
		"/backend-api/codex/responses",
		"/backend-api/codex/responses/compact",
		"/backend-api/codex/responses/compact/detail",
		"/backend-api/codex/responses/input_tokens",
		"/v1/responses/input_tokens",
		"/backend-api/codex/alpha/search",
		"/backend-api/codex/realtime/calls",
	} {
		request, err := http.NewRequest(http.MethodPost, "https://chatgpt.com"+path, nil)
		require.NoError(t, err)
		require.True(t, isOpenAIOAuthGatewayModelRequest(request, oauth), path)
		require.False(t, isOpenAIOAuthGatewayModelRequest(request, apiKey), path)
	}

	request, err := http.NewRequest(http.MethodGet, "https://chatgpt.com/backend-api/codex/models", nil)
	require.NoError(t, err)
	require.False(t, isOpenAIOAuthGatewayModelRequest(request, oauth))
}

func TestOpenAIOAuthGatewayRateLimitFailsClosedWhenEnabled(t *testing.T) {
	withOpenAIOAuthGatewayRuntimeSettings(t, true, 60, 5)
	cacheErr := errors.New("redis unavailable")
	svc := &OpenAIGatewayService{oauthGatewayRateLimitCache: &openAIOAuthGatewayRateLimitCacheStub{err: cacheErr}}
	err := svc.admitOpenAIOAuthGatewayModelRequest(context.Background(), &Account{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeOAuth})
	var limited *openAIOAuthGatewayRateLimitError
	require.ErrorAs(t, err, &limited)
	require.True(t, limited.unavailable)
	require.ErrorIs(t, err, cacheErr)
}

func TestOpenAIOAuthGatewayRateLimitFailoverIsPerAccount(t *testing.T) {
	limited := openAIOAuthGatewayRateLimitFailover(&openAIOAuthGatewayRateLimitError{retryAfterSeconds: 3})
	require.NotNil(t, limited)
	require.Equal(t, http.StatusTooManyRequests, limited.StatusCode)
	require.Equal(t, GatewayFailureScopeAccount, limited.Scope)
	require.Equal(t, NextAccountRetry, limited.NextAccountAction)
	require.True(t, limited.ShouldRetryNextAccount())
	require.False(t, limited.ShouldReportAccountScheduleFailure())
	require.Equal(t, "3", limited.ResponseHeaders.Get("Retry-After"))

	unavailable := openAIOAuthGatewayRateLimitFailover(&openAIOAuthGatewayRateLimitError{unavailable: true})
	require.NotNil(t, unavailable)
	require.Equal(t, http.StatusServiceUnavailable, unavailable.StatusCode)
	require.Equal(t, GatewayFailureScopeRequest, unavailable.Scope)
	require.Equal(t, NextAccountStop, unavailable.NextAccountAction)
	require.False(t, unavailable.ShouldRetryNextAccount())
	require.False(t, unavailable.ShouldReportAccountScheduleFailure())
}

func TestOpenAIOAuthGatewayRateLimitWSFailoverPreservesLaterTurn(t *testing.T) {
	payload := []byte(`{"type":"response.create","model":"gpt-5.4"}`)
	limitedErr := &openAIOAuthGatewayRateLimitError{retryAfterSeconds: 2}

	firstTurn := openAIOAuthGatewayRateLimitWSFailover(limitedErr, 1, payload)
	var firstFailover *UpstreamFailoverError
	require.ErrorAs(t, firstTurn, &firstFailover)
	require.True(t, firstFailover.ShouldRetryNextAccount())
	_, isCurrentTurn := OpenAIWSCurrentTurnRetryPayload(firstTurn)
	require.False(t, isCurrentTurn)

	laterTurn := openAIOAuthGatewayRateLimitWSFailover(limitedErr, 2, payload)
	var laterFailover *UpstreamFailoverError
	require.ErrorAs(t, laterTurn, &laterFailover)
	require.True(t, laterFailover.ShouldRetryNextAccount())
	retryPayload, isCurrentTurn := OpenAIWSCurrentTurnRetryPayload(laterTurn)
	require.True(t, isCurrentTurn)
	require.Equal(t, payload, retryPayload)

	unavailable := openAIOAuthGatewayRateLimitWSFailover(&openAIOAuthGatewayRateLimitError{unavailable: true}, 2, payload)
	var unavailableFailover *UpstreamFailoverError
	require.ErrorAs(t, unavailable, &unavailableFailover)
	require.False(t, unavailableFailover.ShouldRetryNextAccount())
	_, isCurrentTurn = OpenAIWSCurrentTurnRetryPayload(unavailable)
	require.False(t, isCurrentTurn)
}

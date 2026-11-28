package service

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRedeemReductionUsesLockedExpiryAndPreservesRemainingHours(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	for _, remaining := range []time.Duration{12 * time.Hour, 24 * time.Hour, 36 * time.Hour, 49 * time.Hour} {
		t.Run(remaining.String(), func(t *testing.T) {
			currentExpiry := now.Add(remaining)
			repo := &lockingRenewalRepo{
				stale: UserSubscription{ID: 7, UserID: 11, GroupID: 13, ExpiresAt: now.Add(10 * 24 * time.Hour)},
				current: UserSubscription{ID: 7, UserID: 11, GroupID: 13, ExpiresAt: currentExpiry,
					Status: SubscriptionStatusActive, Notes: "current"},
			}
			subscriptions := &SubscriptionService{userSubRepo: repo, now: func() time.Time { return now }}
			svc := &RedeemService{subscriptionService: subscriptions}
			require.NoError(t, svc.reduceOrCancelSubscription(context.Background(), 11, 13, 1, "refund"))
			wantExpiry := currentExpiry.AddDate(0, 0, -1)
			wantStatus := SubscriptionStatusActive
			if !wantExpiry.After(now) {
				wantExpiry, wantStatus = now, SubscriptionStatusExpired
			}
			require.Equal(t, 1, repo.lockReads)
			require.Equal(t, wantExpiry, repo.current.ExpiresAt)
			require.Equal(t, wantStatus, repo.current.Status)
			require.Contains(t, repo.current.Notes, "current\n")
		})
	}
}

func TestProxyFallbackSkipsInactiveNodeAndFollowsItsChain(t *testing.T) {
	now := time.Now()
	id2, id3 := int64(2), int64(3)
	start := Proxy{ID: 1, FallbackMode: FallbackModeProxy, BackupProxyID: &id2}
	for _, status := range []string{StatusDisabled, ProxyStatusInactive, StatusExpired, ""} {
		t.Run(status, func(t *testing.T) {
			byID := map[int64]Proxy{2: {ID: 2, Status: status, FallbackMode: FallbackModeNone}}
			target, change := ResolveProxyFallbackTarget(start, byID, now)
			require.False(t, change)
			require.Nil(t, target)
			byID[2] = Proxy{ID: 2, Status: status, FallbackMode: FallbackModeProxy, BackupProxyID: &id3}
			byID[3] = Proxy{ID: 3, Status: StatusActive}
			target, change = ResolveProxyFallbackTarget(start, byID, now)
			require.True(t, change)
			require.Equal(t, &id3, target)
		})
	}
}

func TestOpenAIStreamStatusAliasesPreserveSemanticAndOutwardContracts(t *testing.T) {
	for _, path := range []string{"response", "error", "root"} {
		for _, spelling := range []string{"status", "status_code"} {
			for _, status := range []int{401, 403, 429, 529, 500} {
				t.Run(fmt.Sprintf("%s/%s/%d", path, spelling, status), func(t *testing.T) {
					errJSON := fmt.Sprintf(`{"code":"server_error",%q:%d,"message":"upstream rejected request"}`, spelling, status)
					payload := errJSON
					switch path {
					case "response":
						payload = `{"type":"response.failed","response":{"status":"failed","error":` + errJSON + `}}`
					case "error":
						payload = `{"type":"error","error":` + errJSON + `}`
					}
					wantSemantic := status
					if status == 500 {
						wantSemantic = http.StatusBadGateway
					}
					require.Equal(t, wantSemantic, openAIStreamFailedEventSemanticStatus([]byte(payload), ""))
					wantOutward := http.StatusBadGateway
					if status == 429 {
						wantOutward = http.StatusTooManyRequests
					}
					require.Equal(t, wantOutward, openAIStreamFailureStatus([]byte(payload), ""))
				})
			}
		}
	}
}

func TestGeminiSDKCommentCompatibility(t *testing.T) {
	require.True(t, geminiClientRejectsSSEComments("google-genai-sdk/1.71.0 gl-go/go1.28"))
	require.True(t, geminiClientRejectsSSEComments("google-genai-sdk/1.0 gl-python/3.12"))
	require.True(t, geminiClientRejectsSSEComments("google-genai-sdk/1.30.0 gl-node/v24.22.3"))
	require.False(t, geminiClientRejectsSSEComments("google-genai-sdk/1.32.0 gl-node/24"))
	require.False(t, geminiClientRejectsSSEComments("google-genai-sdk/invalid gl-node/24"))
	require.False(t, geminiClientRejectsSSEComments("custom-client gl-go/go1.28"))
}

func TestGeminiRetryInfoRoundsUpAndRejectsInvalidDelay(t *testing.T) {
	start := time.Now().Unix()
	reset := ParseGeminiRateLimitResetTime([]byte(`{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":"1.2s"}]}}`))
	require.NotNil(t, reset)
	require.GreaterOrEqual(t, *reset, start+2)
	require.LessOrEqual(t, *reset, start+3)
	for _, delay := range []string{"-1s", "0s", "invalid", "8760h"} {
		body := []byte(fmt.Sprintf(`{"error":{"details":[{"@type":"type.googleapis.com/google.rpc.RetryInfo","retryDelay":%q}]}}`, delay))
		require.Nil(t, geminiRetryInfoResetTime(body))
	}
}

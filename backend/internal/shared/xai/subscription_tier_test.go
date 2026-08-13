//go:build unit

package xai

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMapJWTSubscriptionTierNumber(t *testing.T) {
	t.Parallel()

	require.Equal(t, "free", MapJWTSubscriptionTier(0))
	require.Equal(t, "supergrok", MapJWTSubscriptionTier(1))
	require.Equal(t, "x_basic", MapJWTSubscriptionTier(2))
	require.Equal(t, "x_premium", MapJWTSubscriptionTier(3))
	require.Equal(t, "x_premium_plus", MapJWTSubscriptionTier(4))
	require.Equal(t, "supergrok_heavy", MapJWTSubscriptionTier(5))
	require.Equal(t, "supergrok_lite", MapJWTSubscriptionTier(6))
	require.Equal(t, "supergrok_plus", MapJWTSubscriptionTier(7))
	require.Equal(t, "9", MapJWTSubscriptionTier(9))
}

func TestNormalizeSubscriptionTierAliases(t *testing.T) {
	t.Parallel()

	require.Equal(t, "free", NormalizeSubscriptionTier("free-tier"))
	require.Equal(t, "supergrok", NormalizeSubscriptionTier("SuperGrok"))
	require.Equal(t, "supergrok_heavy", NormalizeSubscriptionTier("SuperGrok Heavy"))
	require.Equal(t, "supergrok_pro", NormalizeSubscriptionTier("SuperGrokPro"))
	require.Equal(t, "supergrok_lite", NormalizeSubscriptionTier("SuperGrok Lite"))
	require.Equal(t, "x_basic", NormalizeSubscriptionTier("X Basic"))
}

func TestSubscriptionTierFromJWTUsesAccessTokenClaim(t *testing.T) {
	t.Parallel()

	require.Equal(t, "supergrok_heavy", SubscriptionTierFromJWT(jwtWithTierClaims(t, map[string]any{"tier": 5})))
	require.Equal(t, "free", SubscriptionTierFromJWT(jwtWithTierClaims(t, map[string]any{"tier": 0})))
	require.Equal(t, "supergrok_lite", SubscriptionTierFromJWT(jwtWithTierClaims(t, map[string]any{"tier": "6"})))
	require.Empty(t, SubscriptionTierFromJWT(jwtWithTierClaims(t, map[string]any{"sub": "user"})))
	require.Empty(t, SubscriptionTierFromJWT("not-a-jwt"))
}

func TestCanonicalGrokPlanUsesOnlyFreshGrok45ResponsesWindow(t *testing.T) {
	t.Parallel()

	zero := float64(0)
	heavyRequests := int64(8_300)
	fresh := time.Now().UTC().Format(time.RFC3339)
	stale := time.Now().Add(-GrokQuotaSignalMaxAge - time.Hour).UTC().Format(time.RFC3339)

	require.Equal(t, "supergrok", CanonicalGrokPlan(&zero, "SuperGrokPro", nil))
	require.Equal(t, "supergrok_heavy", CanonicalGrokPlan(&zero, "SuperGrokPro", &QuotaSnapshot{
		Model: "grok-4.5", Requests: &QuotaWindow{Limit: &heavyRequests}, LastHeadersSeenAt: fresh,
	}))
	require.Equal(t, "supergrok", CanonicalGrokPlan(&zero, "SuperGrokPro", &QuotaSnapshot{
		Model: "grok-4.6", Requests: &QuotaWindow{Limit: &heavyRequests}, LastHeadersSeenAt: fresh,
	}))
	require.Equal(t, "supergrok", CanonicalGrokPlan(&zero, "SuperGrokPro", &QuotaSnapshot{
		Model: "grok-4.5", Requests: &QuotaWindow{Limit: &heavyRequests}, LastHeadersSeenAt: stale,
	}))
	heavyCents := float64(SuperGrokHeavyLimitCents)
	require.Equal(t, "supergrok_heavy", CanonicalGrokPlan(&heavyCents, "SuperGrokPro", nil))
}

func TestApplyGrok45ResponsesPlanSignalCarriesHint(t *testing.T) {
	t.Parallel()

	heavyRequests := int64(8_300)
	fresh := time.Now().UTC().Format(time.RFC3339)
	previous := &QuotaSnapshot{
		Model: "grok-4.5", Requests: &QuotaWindow{Limit: &heavyRequests}, LastHeadersSeenAt: fresh,
	}
	previous.ApplyGrok45ResponsesPlanSignal(nil)
	require.Equal(t, "supergrok_heavy", previous.PlanFrom45Responses)

	next := &QuotaSnapshot{Model: "grok-4.6", LastHeadersSeenAt: fresh}
	next.ApplyGrok45ResponsesPlanSignal(previous)
	require.Equal(t, previous.PlanFrom45Responses, next.PlanFrom45Responses)
	require.Equal(t, previous.PlanFrom45ResponsesAt, next.PlanFrom45ResponsesAt)
}

func jwtWithTierClaims(t *testing.T, claims map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	require.NoError(t, err)
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

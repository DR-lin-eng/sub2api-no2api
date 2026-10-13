//go:build unit

package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestIsModelRateLimited_AntigravityImageDoesNotInheritGeminiFamilyLimit(t *testing.T) {
	future := time.Now().Add(10 * time.Minute).Format(time.RFC3339)
	account := &Account{
		Platform: PlatformAntigravity,
		Extra: map[string]any{
			modelRateLimitsKey: map[string]any{
				antigravityGeminiModelRateLimitKey: map[string]any{
					"rate_limit_reset_at": future,
				},
			},
		},
	}

	for _, model := range []string{"gemini-3.1-flash-image", "gemini-3.1-flash-image-preview", "gemini-3-pro-image"} {
		require.False(t, account.isModelRateLimitedWithContext(context.Background(), model), model)
	}
	require.True(t, account.isModelRateLimitedWithContext(context.Background(), "gemini-3-flash"))
	rawLimits, ok := account.Extra[modelRateLimitsKey].(map[string]any)
	require.True(t, ok)
	rawLimits["gemini-3.1-flash-image"] = map[string]any{
		"rate_limit_reset_at": future,
	}
	require.True(t, account.isModelRateLimitedWithContext(context.Background(), "gemini-3-pro-image-preview"))
}

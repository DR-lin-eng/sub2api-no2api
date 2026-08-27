package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetPassiveUsageOpenAIOAuthUsesNormalizedPersistedWindows(t *testing.T) {
	t.Parallel()

	fiveHourReset := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	sevenDayReset := time.Now().Add(5 * 24 * time.Hour).UTC().Truncate(time.Second)
	repo := &stubOpenAIAccountRepo{accounts: []Account{{
		ID:       701,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"codex_usage_updated_at":  time.Now().Add(-time.Minute).UTC().Format(time.RFC3339),
			"codex_5h_used_percent":   17.0,
			"codex_5h_window_minutes": 300,
			"codex_5h_reset_at":       fiveHourReset.Format(time.RFC3339),
			"codex_7d_used_percent":   63.0,
			"codex_7d_window_minutes": 10080,
			"codex_7d_reset_at":       sevenDayReset.Format(time.RFC3339),
		},
	}}}
	usageService := &AccountUsageService{accountRepo: repo}

	usage, err := usageService.GetPassiveUsage(context.Background(), 701)

	require.NoError(t, err)
	require.Equal(t, "passive", usage.Source)
	require.NotNil(t, usage.FiveHour)
	require.Equal(t, 17.0, usage.FiveHour.Utilization)
	require.WithinDuration(t, fiveHourReset, *usage.FiveHour.ResetsAt, time.Second)
	require.NotNil(t, usage.SevenDay)
	require.Equal(t, 63.0, usage.SevenDay.Utilization)
	require.WithinDuration(t, sevenDayReset, *usage.SevenDay.ResetsAt, time.Second)
}

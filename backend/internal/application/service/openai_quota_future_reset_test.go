package service

import (
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestStaleOpenAIQuotaRetainsKnownFutureReset(t *testing.T) {
	now := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	for _, window := range []string{"5h", "7d"} {
		t.Run(window, func(t *testing.T) {
			extra := map[string]any{"codex_" + window + "_used_percent": 99.0, "codex_usage_updated_at": now.Add(-3 * time.Hour).Format(time.RFC3339), "codex_" + window + "_reset_at": now.Add(time.Hour).Format(time.RFC3339)}
			utilization, valid := resolveOpenAIQuotaUtilization(extra, window, now)
			require.True(t, valid)
			require.Equal(t, 0.99, utilization)
			extra["codex_"+window+"_reset_at"] = now.Add(-time.Second).Format(time.RFC3339)
			_, valid = resolveOpenAIQuotaUtilization(extra, window, now)
			require.False(t, valid)
			delete(extra, "codex_"+window+"_reset_at")
			_, valid = resolveOpenAIQuotaUtilization(extra, window, now)
			require.False(t, valid)
		})
	}
}

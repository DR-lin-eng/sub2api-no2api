package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/timezone"
	"github.com/stretchr/testify/require"
)

func useGroupUsageTestTimezone(t *testing.T, name string) {
	t.Helper()
	previousName := timezone.Name()
	require.NoError(t, timezone.Init(name))
	t.Cleanup(func() { require.NoError(t, timezone.Init(previousName)) })
}

func TestGroupUsageDateUsesConfiguredTimezoneBoundary(t *testing.T) {
	useGroupUsageTestTimezone(t, "America/New_York")

	beforeMidnight := time.Date(2026, 3, 9, 3, 59, 59, 0, time.UTC)
	atMidnight := time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC)

	require.Equal(t, "2026-03-08", GroupUsageDate(beforeMidnight))
	require.Equal(t, "2026-03-09", GroupUsageDate(atMidnight))
	require.Equal(t, atMidnight, GroupUsageTodayStart(atMidnight))
}

func TestGroupUsageYesterdayStartHandlesDST(t *testing.T) {
	useGroupUsageTestTimezone(t, "America/New_York")

	todayStart := time.Date(2026, 3, 9, 4, 0, 0, 0, time.UTC)
	yesterdayStart := GroupUsageYesterdayStart(todayStart)

	require.Equal(t, time.Date(2026, 3, 8, 5, 0, 0, 0, time.UTC), yesterdayStart)
	require.Equal(t, 23*time.Hour, todayStart.Sub(yesterdayStart))
	require.Equal(t, "America/New_York", GroupUsageTimezoneName())
}

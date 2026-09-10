package service

import (
	"testing"
	"time"
)

func TestOpenAISessionIDRateMetricsCountsDistinctIDsPerMinute(t *testing.T) {
	metrics := NewOpenAISessionIDRateMetrics()
	minute := time.Unix(1_700_000_000, 0).UTC().Truncate(time.Minute)
	metrics.Record(1, "session-a", minute)
	metrics.Record(1, "session-a", minute.Add(10*time.Second))
	metrics.Record(1, "session-b", minute.Add(20*time.Second))
	metrics.Record(2, "session-c", minute.Add(20*time.Second))

	snapshot := metrics.Snapshot([]int64{1, 2}, minute.Add(30*time.Second))
	if snapshot.Counts[1] != 2 || snapshot.Counts[2] != 1 {
		t.Fatalf("counts = %#v, want account 1=2 account 2=1", snapshot.Counts)
	}
	if snapshot.TotalPerMinute != 3 || snapshot.MaxPerMinute != 2 || snapshot.MaxAccountID != 1 {
		t.Fatalf("summary = %+v, want total=3 max=2 account=1", snapshot)
	}
}

func TestOpenAISessionIDRateMetricsRotatesAtMinuteBoundary(t *testing.T) {
	metrics := NewOpenAISessionIDRateMetrics()
	minute := time.Unix(1_700_000_000, 0).UTC().Truncate(time.Minute)
	metrics.Record(1, "session-a", minute)
	if got := metrics.Snapshot([]int64{1}, minute.Add(59*time.Second)).TotalPerMinute; got != 1 {
		t.Fatalf("same-minute total = %d, want 1", got)
	}
	metrics.Record(1, "session-a", minute.Add(time.Minute))
	if got := metrics.Snapshot([]int64{1}, minute.Add(time.Minute)).TotalPerMinute; got != 0 {
		t.Fatalf("next-minute total = %d, want 0", got)
	}
}

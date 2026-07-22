package repository

import (
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
)

func TestBuildOpsThroughputTrendQueryIncludesLegacyErrorSources(t *testing.T) {
	start := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	query, args := buildOpsThroughputTrendQuery(&service.OpsDashboardFilter{}, start, end, 60)

	for _, fragment := range []string{"error_buckets AS", "switch_buckets AS", "jsonb_array_elements"} {
		if !strings.Contains(query, fragment) {
			t.Fatalf("query missing %q: %s", fragment, query)
		}
	}
	if len(args) != 4 {
		t.Fatalf("args = %d, want 4", len(args))
	}
}

func TestBuildOpsThroughputTrendQueryReusesSnapshotErrorTrend(t *testing.T) {
	start := time.Date(2026, 7, 16, 0, 0, 0, 0, time.UTC)
	end := start.Add(time.Hour)
	filter := &service.OpsDashboardFilter{
		ReuseErrorTrendCounts: true,
		ExcludeSwitchCounts:   true,
	}
	query, args := buildOpsThroughputTrendQuery(filter, start, end, 60)

	for _, fragment := range []string{"error_buckets AS", "switch_buckets AS", "jsonb_array_elements", "FROM ops_error_logs"} {
		if strings.Contains(query, fragment) {
			t.Fatalf("optimized query still contains %q: %s", fragment, query)
		}
	}
	if len(args) != 2 {
		t.Fatalf("args = %d, want 2", len(args))
	}
}

package service

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/timezone"
)

const groupUsageDateFormat = "2006-01-02"

// GroupUsageRollupRepository is the optional persistence capability used by
// the dashboard aggregation service to publish closed group-usage day buckets.
type GroupUsageRollupRepository interface {
	SyncGroupUsageRollups(ctx context.Context, todayStart time.Time) error
}

// GroupUsageTimezoneName returns the configured server timezone.
func GroupUsageTimezoneName() string {
	return timezone.Location().String()
}

// GroupUsageTodayStart returns the UTC instant for the configured timezone's
// local start of day containing at.
func GroupUsageTodayStart(at time.Time) time.Time {
	return timezone.StartOfDay(at).UTC()
}

// GroupUsageYesterdayStart returns the UTC instant for the prior local
// calendar day's start. AddDate preserves DST-aware calendar semantics.
func GroupUsageYesterdayStart(at time.Time) time.Time {
	return timezone.StartOfDay(at).AddDate(0, 0, -1).UTC()
}

// GroupUsageDate formats at as a local calendar date in the server timezone.
func GroupUsageDate(at time.Time) string {
	return at.In(timezone.Location()).Format(groupUsageDateFormat)
}

// ParseGroupUsageDate parses a local calendar date in the server timezone.
func ParseGroupUsageDate(value string) (time.Time, error) {
	return timezone.ParseInLocation(groupUsageDateFormat, value)
}

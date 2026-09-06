package repository

import (
	"fmt"
	"strings"
)

const (
	activityCenterMigration = "237_add_activity_center.sql"
	// SHA256 of the trimmed, immutable published file, not the execution SQL.
	activityCenterMigrationChecksum = "db456740b1da8c7a7dd47b93ab1d8d4db77f38e08373fe20bcc786cd65db88a6"
	activityCenterCompatibleTypes   = "'lottery', 'inflate', 'redeem', 'custom', 'checkin', 'external_link', 'announcement'"
)

// migrationSQLForExecution repairs only the known published activity migration.
// Its concatenated historical steps temporarily exclude checkin, and its final
// constraints exclude older external_link/announcement rows. A preceding ALTER
// cannot fix that: 237 immediately drops and recreates those constraints.
//
// Widen all six type checks without changing rows, skipping SQL, or changing the
// recorded checksum. Migration 238 makes already-applied installations converge
// to the same constraints. All other SQL and all checksum checks remain intact.
func migrationSQLForExecution(name, checksum, content string) (string, error) {
	if name != activityCenterMigration {
		return content, nil
	}
	if checksum != activityCenterMigrationChecksum {
		return "", fmt.Errorf("activity-center compatibility requires original migration %s checksum %s (file=%s)", name, activityCenterMigrationChecksum, checksum)
	}

	for _, column := range []string{"type", "campaign_type"} {
		for _, oldTypes := range []string{
			"'lottery', 'redeem', 'custom'",
			"'lottery', 'inflate', 'redeem', 'custom'",
			"'lottery', 'inflate', 'redeem', 'custom', 'checkin'",
		} {
			oldCheck := "CHECK (" + column + " IN (" + oldTypes + "))"
			if strings.Count(content, oldCheck) != 1 {
				return "", fmt.Errorf("unexpected type constraint in migration %s: %s", name, oldCheck)
			}
			newCheck := "CHECK (" + column + " IN (" + activityCenterCompatibleTypes + "))"
			content = strings.Replace(content, oldCheck, newCheck, 1)
		}
	}
	return content, nil
}

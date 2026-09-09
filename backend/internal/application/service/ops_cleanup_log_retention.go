package service

import (
	"context"
	"encoding/json"
)

// The existing runtime log retention field owns system logs. Absent or invalid
// legacy values keep the previous cleanup policy. This read runs once per
// scheduled cleanup and adds no work to log ingestion or gateway requests.
func (s *OpsCleanupService) systemLogRetentionDays(ctx context.Context, fallback int) int {
	if s.settingRepo == nil {
		return fallback
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyOpsRuntimeLogConfig)
	if err != nil {
		return fallback
	}
	var config struct {
		RetentionDays int `json:"retention_days"`
	}
	if json.Unmarshal([]byte(raw), &config) != nil || config.RetentionDays < 1 || config.RetentionDays > 3650 {
		return fallback
	}
	return config.RetentionDays
}

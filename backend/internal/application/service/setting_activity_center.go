package service

import (
	"context"
	"errors"
	"log/slog"
	"strings"
)

// IsActivityCenterEnabled reads the user-facing activity center switch.
// Fail-closed because the feature is opt-in and exposes a new authenticated
// surface.
func (s *SettingService) IsActivityCenterEnabled(ctx context.Context) bool {
	if s == nil || s.settingRepo == nil {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	value, err := s.settingRepo.GetValue(ctx, SettingKeyActivityCenterEnabled)
	if err != nil {
		if !errors.Is(err, ErrSettingNotFound) {
			slog.Warn("failed to read activity center feature setting", "error", err)
		}
		return false
	}
	return strings.EqualFold(strings.TrimSpace(value), "true")
}

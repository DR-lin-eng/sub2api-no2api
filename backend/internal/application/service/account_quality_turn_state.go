package service

import (
	"context"
	"encoding/json"
)

// loadAccountQualityTurnStateSettings reads the control-plane pool once per
// quality round. A missing or malformed pool disables injection for that round
// without turning an otherwise valid quality check into an operational error.
func (s *AccountQualityMonitoringService) loadAccountQualityTurnStateSettings(ctx context.Context) CodexSimulationSettings {
	if s == nil || s.settingRepo == nil {
		return CodexSimulationSettings{}
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyCodexSimulationSettings)
	if err != nil {
		return CodexSimulationSettings{}
	}
	var settings CodexSimulationSettings
	if json.Unmarshal([]byte(raw), &settings) != nil {
		return CodexSimulationSettings{}
	}
	settings, err = validateCodexSimulationSettings(settings)
	if err != nil {
		return CodexSimulationSettings{}
	}
	return settings
}

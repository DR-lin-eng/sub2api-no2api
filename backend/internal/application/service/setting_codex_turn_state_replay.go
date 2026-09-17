package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"golang.org/x/net/http/httpguts"
)

const (
	codexTurnStateMaxEntries           = 4096
	codexTurnStateMaxValueBytes        = 8 << 10
	codexTurnStateMaxTotalBytes        = 256 << 10
	codexTurnStateWatchModelMaxEntries = 100
	codexTurnStateWatchModelMaxBytes   = 128
)

func normalizeCodexTurnStateWatchModels(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		model := strings.ToLower(strings.TrimSpace(raw))
		if model == "" {
			continue
		}
		if len(model) > codexTurnStateWatchModelMaxBytes {
			return nil, infraerrors.BadRequest("INVALID_CODEX_TURN_STATE_MODEL", fmt.Sprintf("turn state watch model exceeds %d bytes", codexTurnStateWatchModelMaxBytes))
		}
		if !httpguts.ValidHeaderFieldValue(model) {
			return nil, infraerrors.BadRequest("INVALID_CODEX_TURN_STATE_MODEL", "turn state watch model contains invalid bytes")
		}
		if _, ok := seen[model]; ok {
			continue
		}
		if len(result) >= codexTurnStateWatchModelMaxEntries {
			return nil, infraerrors.BadRequest("INVALID_CODEX_TURN_STATE_MODEL", fmt.Sprintf("turn state watch model count exceeds %d", codexTurnStateWatchModelMaxEntries))
		}
		seen[model] = struct{}{}
		result = append(result, model)
	}
	return result, nil
}

func codexTurnStateModelIsWatched(settings CodexSimulationSettings, model string) bool {
	model = strings.ToLower(strings.TrimSpace(model))
	if model == "" {
		return false
	}
	for _, watched := range settings.TurnStateWatchModels {
		if strings.EqualFold(strings.TrimSpace(watched), model) {
			return true
		}
	}
	return false
}

func normalizeCodexTurnStates(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	total := 0
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if len(value) > codexTurnStateMaxValueBytes {
			return nil, infraerrors.BadRequest("INVALID_CODEX_TURN_STATE", fmt.Sprintf("turn state exceeds %d bytes", codexTurnStateMaxValueBytes))
		}
		if !httpguts.ValidHeaderFieldValue(value) {
			return nil, infraerrors.BadRequest("INVALID_CODEX_TURN_STATE", "turn state contains invalid HTTP header bytes")
		}
		if _, ok := seen[value]; ok {
			continue
		}
		if len(result) >= codexTurnStateMaxEntries {
			return nil, infraerrors.BadRequest("INVALID_CODEX_TURN_STATE", fmt.Sprintf("turn state count exceeds %d", codexTurnStateMaxEntries))
		}
		total += len(value)
		if total > codexTurnStateMaxTotalBytes {
			return nil, infraerrors.BadRequest("INVALID_CODEX_TURN_STATE", fmt.Sprintf("turn state values exceed %d total bytes", codexTurnStateMaxTotalBytes))
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func normalizeCodexTurnStateAccountIDs(states []string, bindings map[string][]int64) map[string][]int64 {
	if len(states) == 0 || len(bindings) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(states))
	for _, state := range states {
		allowed[state] = struct{}{}
	}
	result := make(map[string][]int64)
	for state, ids := range bindings {
		if _, ok := allowed[state]; !ok {
			continue
		}
		seen := make(map[int64]struct{}, len(ids))
		for _, id := range ids {
			if id <= 0 {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			result[state] = append(result[state], id)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func codexTurnStateEligibleForAccount(settings CodexSimulationSettings, state string, accountID int64) bool {
	ids := settings.TurnStateAccountIDs[state]
	if len(ids) == 0 {
		return true
	}
	for _, id := range ids {
		if id == accountID {
			return true
		}
	}
	return false
}

func randomCodexTurnStateForAccount(settings CodexSimulationSettings, accountID int64) string {
	eligible := 0
	for _, state := range settings.TurnStates {
		if codexTurnStateEligibleForAccount(settings, state, accountID) {
			eligible++
		}
	}
	if eligible == 0 {
		return ""
	}
	wanted := 0
	if eligible > 1 {
		wanted = rand.IntN(eligible)
	}
	for _, state := range settings.TurnStates {
		if !codexTurnStateEligibleForAccount(settings, state, accountID) {
			continue
		}
		if wanted == 0 {
			return state
		}
		wanted--
	}
	return ""
}

func (s *SettingService) SyncCodexTurnStatesFromAccountQuality(ctx context.Context) (*CodexSimulationSettings, error) {
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("setting service is unavailable")
	}
	s.codexSimulationSettingsMu.Lock()
	defer s.codexSimulationSettingsMu.Unlock()

	current, err := s.readCodexSimulationSettings(ctx)
	if err != nil {
		return nil, err
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAccountQualityState)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return nil, infraerrors.BadRequest("CODEX_TURN_STATE_NOT_FOUND", "no account-quality turn state is available to sync")
		}
		return nil, fmt.Errorf("get account quality state: %w", err)
	}
	var quality AccountQualityRunState
	if err := json.Unmarshal([]byte(raw), &quality); err != nil {
		return nil, fmt.Errorf("decode account quality state: %w", err)
	}
	if quality.Status != AccountInspectionStatusSucceeded {
		return nil, infraerrors.BadRequest("CODEX_TURN_STATE_RUN_INCOMPLETE", "account quality run must succeed before syncing turn states")
	}

	manual := make([]string, 0, len(current.TurnStates))
	manualSet := make(map[string]struct{}, len(current.TurnStates))
	for _, state := range current.TurnStates {
		if len(current.TurnStateAccountIDs[state]) == 0 {
			manual = append(manual, state)
			manualSet[state] = struct{}{}
		}
	}
	states := append([]string(nil), manual...)
	bindings := make(map[string][]int64)
	captured := 0
	for _, result := range quality.Results {
		if result.QualityStatus != "healthy" || result.AccountID <= 0 {
			continue
		}
		for _, state := range result.QualityTurnStates {
			state = strings.TrimSpace(state)
			if state == "" {
				continue
			}
			if _, global := manualSet[state]; global {
				continue
			}
			if _, exists := bindings[state]; !exists {
				states = append(states, state)
				captured++
			}
			bindings[state] = append(bindings[state], result.AccountID)
		}
	}
	if captured == 0 {
		return nil, infraerrors.BadRequest("CODEX_TURN_STATE_NOT_FOUND", "no healthy account-quality turn state is available to sync")
	}
	current.TurnStates = states
	current.TurnStateAccountIDs = bindings
	validated, err := validateCodexSimulationSettings(current)
	if err != nil {
		return nil, err
	}
	return s.persistCodexSimulationSettings(ctx, validated)
}

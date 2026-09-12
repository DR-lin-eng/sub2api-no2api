package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

func (s *AccountQualityMonitoringService) validateQualityGroups(ctx context.Context, settings AccountQualitySettings) error {
	if settings.SourceGroupID == nil && settings.DegradedGroupID == nil {
		return nil
	}
	if s.groupRepo == nil {
		return fmt.Errorf("quality group repository is unavailable")
	}
	var source, target *Group
	var err error
	if settings.SourceGroupID != nil {
		source, err = s.groupRepo.GetByID(ctx, *settings.SourceGroupID)
		if err != nil || source == nil || source.Status != StatusActive || (source.Platform != PlatformOpenAI && source.Platform != PlatformGemini) {
			return fmt.Errorf("quality source group is missing, inactive, or unsupported")
		}
	}
	if settings.DegradedGroupID != nil {
		target, err = s.groupRepo.GetByID(ctx, *settings.DegradedGroupID)
		if err != nil || target == nil || target.Status != StatusActive || (target.Platform != PlatformOpenAI && target.Platform != PlatformGemini) {
			return fmt.Errorf("quality degraded group is missing, inactive, or unsupported")
		}
	}
	if source != nil && target != nil && source.Platform != target.Platform {
		return fmt.Errorf("quality source and degraded groups must use the same platform")
	}
	return nil
}

type accountQualityStateWriter interface {
	UpdateExtra(context.Context, int64, map[string]any) error
}

const (
	accountQualityStatusExtraKey      = "account_quality_status"
	accountQualityFailuresExtraKey    = "account_quality_consecutive_failures"
	accountQualityPassesExtraKey      = "account_quality_consecutive_passes"
	accountQualityLastCheckedExtraKey = "account_quality_last_checked_at"
	accountQualityHistoryExtraKey     = "account_quality_history"
	accountQualityProbeTimeout        = 2 * time.Minute
)

// runQualityMonitoring performs bounded, account-specific probes. It is kept
// separate from the usage-stat inspection so operators can enable quality
// probes without changing billing or request accounting.
func (s *AccountQualityMonitoringService) runQualityMonitoring(ctx context.Context, accounts []Account, results []AccountInspectionAccountResult, previous *AccountQualityRunState, settings AccountQualitySettings, now time.Time) error {
	previousByID := make(map[int64]AccountInspectionAccountResult)
	if previous != nil {
		for _, result := range previous.Results {
			previousByID[result.AccountID] = result
		}
	}
	sem := make(chan struct{}, settings.MaxConcurrent)
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex
	for i := range accounts {
		account := accounts[i]
		if !qualityProbeEligible(&account, settings.SourceGroupID) {
			continue
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(index int, account Account) {
			defer wg.Done()
			defer func() { <-sem }()
			model := strings.TrimSpace(settings.Model)
			probeCtx, cancel := context.WithTimeout(ctx, accountQualityProbeTimeout)
			defer cancel()
			probe, err := s.accountTestSvc.RunQualityTestBackground(probeCtx, account.ID, model, settings.Prompt, settings.Effort)
			if err != nil {
				probe = &ScheduledTestResult{Status: "failed", ErrorMessage: err.Error()}
			}
			requestOK := probe != nil && probe.Status == "success"
			passed := false
			classified := false
			artifactStatus := "error"
			var artifact *QualityArtifact
			processErr := error(nil)
			if requestOK && s.qualityProcessor == nil {
				requestOK = false
				probe.ErrorMessage = "renderer/classifier is not configured"
			}
			if requestOK && s.qualityProcessor != nil {
				artifact, processErr = s.qualityProcessor.Process(probeCtx, probe.ResponseText)
				if processErr != nil {
					requestOK = false
					probe.ErrorMessage = "render/classification failed: " + processErr.Error()
				} else if artifact == nil || artifact.Confidence < settings.MinConfidence {
					requestOK = false
					artifactStatus = "uncertain"
					probe.ErrorMessage = "classifier confidence below threshold"
				} else if artifact.Label != "normal" && artifact.Label != "unnormal" {
					requestOK = false
					artifactStatus = "uncertain"
					probe.ErrorMessage = "classifier returned unknown label"
				} else {
					classified = true
					passed = artifact.Label == "normal"
					artifactStatus = "ready"
					if s.qualityArtifacts != nil {
						run := &AccountQualityRun{AccountID: account.ID, Model: model, Effort: settings.Effort, Status: "ready", Label: artifact.Label, Confidence: artifact.Confidence, StartedAt: probe.StartedAt, FinishedAt: probe.FinishedAt, LatencyMs: probe.LatencyMs, ModelVersion: artifact.ModelVersion, PNG: artifact.PNG, WebP: artifact.WebP}
						if !passed {
							run.Status = "wrong"
						}
						if err := s.qualityArtifacts.Save(ctx, run); err != nil {
							probe.ErrorMessage = "artifact save failed: " + err.Error()
						}
						result := &results[index]
						result.QualityRunID, result.QualityLabel, result.QualityConfidence = run.ID, run.Label, run.Confidence
					}
				}
			}
			failures, passes, status := qualityCounters(account.Extra)
			if !requestOK {
				// Transport/auth/rate-limit failures belong to the existing account
				// health circuit; they must not move an otherwise healthy account to
				// a quality group.
				result := &results[index]
				// Keep consecutive counters unchanged for transport/auth/timeout
				// failures, but expose the current probe as an error instead of
				// showing a stale healthy/degraded label beside the error text.
				result.QualityStatus, result.QualityConsecutiveFailures, result.QualityConsecutivePasses = "error", failures, passes
				if status == "degraded" {
					result.Reasons = appendUniqueReason(result.Reasons, "quality_probe_degraded")
				}
				if probe != nil {
					result.QualityError = strings.TrimSpace(probe.ErrorMessage)
					result.QualityLatencyMs = probe.LatencyMs
				}
				startedAt, finishedAt, latency := now, now, int64(0)
				if probe != nil {
					startedAt, finishedAt, latency = probe.StartedAt, probe.FinishedAt, probe.LatencyMs
				}
				if s.qualityArtifacts != nil {
					run := &AccountQualityRun{AccountID: account.ID, Model: model, Effort: settings.Effort, Status: artifactStatus, Confidence: 0, StartedAt: startedAt, FinishedAt: finishedAt, LatencyMs: latency, Error: result.QualityError}
					if artifact != nil {
						run.Label, run.Confidence, run.ModelVersion = artifact.Label, artifact.Confidence, artifact.ModelVersion
					}
					if saveErr := s.qualityArtifacts.Save(ctx, run); saveErr == nil {
						result.QualityRunID = run.ID
						result.QualityLabel = run.Label
						result.QualityConfidence = run.Confidence
					}
				}
				writeQualityExtra(s.accountRepo, ctx, account.ID, qualityExtraUpdate(account.Extra, status, failures, passes, now, "error", result.QualityError))
				return
			}
			if !classified {
				// A renderer/classifier error is operationally distinct from a
				// wrong answer and must not move the account between groups.
				result := &results[index]
				result.QualityStatus, result.QualityConsecutiveFailures, result.QualityConsecutivePasses = "error", failures, passes
				result.QualityError = strings.TrimSpace(probe.ErrorMessage)
				result.QualityLatencyMs = probe.LatencyMs
				writeQualityExtra(s.accountRepo, ctx, account.ID, qualityExtraUpdate(account.Extra, status, failures, passes, now, "error", result.QualityError))
				return
			}
			if passed {
				passes++
				failures = 0
			} else {
				failures++
				passes = 0
			}
			if passed {
				status = "healthy"
			} else {
				status = "degraded"
			}
			if prior, ok := previousByID[account.ID]; ok && prior.QualityStatus == "degraded" && passed && passes < settings.RecoveryThreshold {
				status = "degraded"
			}
			if failures >= settings.FailureThreshold {
				status = "degraded"
			}
			if status == "degraded" && failures < settings.FailureThreshold && previousByID[account.ID].QualityStatus != "degraded" {
				status = "healthy"
			}
			if prior, ok := previousByID[account.ID]; ok && prior.QualityStatus == "degraded" && passes >= settings.RecoveryThreshold {
				status = "healthy"
			}
			result := &results[index]
			result.QualityStatus, result.QualityConsecutiveFailures, result.QualityConsecutivePasses = status, failures, passes
			if probe != nil {
				result.QualityError = strings.TrimSpace(probe.ErrorMessage)
				result.QualityLatencyMs = probe.LatencyMs
			}
			if status == "degraded" {
				result.Reasons = appendUniqueReason(result.Reasons, "quality_probe_degraded")
			}
			if status == "degraded" && failures >= settings.FailureThreshold {
				if err := s.switchQualityGroup(ctx, &account, settings.DegradedGroupID, result); err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
				}
			} else if status == "healthy" && passes >= settings.RecoveryThreshold {
				if err := s.restoreQualityGroups(ctx, &account, result); err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
				}
			}
			outcome := "wrong"
			if passed {
				outcome = "passed"
			}
			writeQualityExtra(s.accountRepo, ctx, account.ID, qualityExtraUpdate(account.Extra, status, failures, passes, now, outcome, result.QualityError))
		}(i, account)
	}
	wg.Wait()
	return firstErr
}

func writeQualityExtra(repo AccountRepository, ctx context.Context, accountID int64, updates map[string]any) {
	if writer, ok := repo.(accountQualityStateWriter); ok {
		_ = writer.UpdateExtra(ctx, accountID, updates)
	}
}

func writeQualityExtraErr(repo AccountRepository, ctx context.Context, accountID int64, updates map[string]any) error {
	if writer, ok := repo.(accountQualityStateWriter); ok {
		return writer.UpdateExtra(ctx, accountID, updates)
	}
	return nil
}

func qualityExtraUpdate(extra map[string]any, status string, failures, passes int, checkedAt time.Time, outcome, errorMessage string) map[string]any {
	history := make([]map[string]any, 0, 24)
	if raw, ok := extra[accountQualityHistoryExtraKey].([]any); ok {
		for _, value := range raw {
			if item, ok := value.(map[string]any); ok {
				history = append(history, item)
			}
		}
	}
	if raw, ok := extra[accountQualityHistoryExtraKey].([]map[string]any); ok {
		history = append(history, raw...)
	}
	entry := map[string]any{"checked_at": checkedAt.UTC().Format(time.RFC3339Nano), "outcome": outcome}
	if strings.TrimSpace(errorMessage) != "" {
		entry["error"] = truncateQualityError(strings.TrimSpace(errorMessage))
	}
	history = append(history, entry)
	if len(history) > 24 {
		history = history[len(history)-24:]
	}
	return map[string]any{accountQualityStatusExtraKey: status, accountQualityFailuresExtraKey: failures, accountQualityPassesExtraKey: passes, accountQualityLastCheckedExtraKey: checkedAt.UTC().Format(time.RFC3339Nano), accountQualityHistoryExtraKey: history}
}

func truncateQualityError(value string) string {
	runes := []rune(value)
	if len(runes) > 240 {
		return string(runes[:240])
	}
	return value
}

func qualityProbeSupported(account *Account) bool {
	if account == nil || !account.IsActive() {
		return false
	}
	// The probe prompt is part of the Gemini/OpenAI request body. Other
	// platforms currently use fixed connection probes and must not be judged by
	// this quality grader until their prompt path is explicit.
	supportedPlatform := account.Platform == PlatformOpenAI || account.Platform == PlatformGemini
	supportedType := account.Type == AccountTypeOAuth || account.Type == AccountTypeAPIKey || (account.Type == AccountTypeServiceAccount && account.Platform == PlatformGemini)
	return supportedPlatform && supportedType
}

func qualityProbeEligible(account *Account, sourceGroupID *int64) bool {
	if !qualityProbeSupported(account) {
		return false
	}
	if sourceGroupID == nil || *sourceGroupID <= 0 {
		return true
	}
	if accountHasGroup(account, *sourceGroupID) {
		return true
	}
	// Continue probing accounts already moved to the degraded group so a
	// healthy streak can restore their original source-group binding.
	if account == nil || account.Extra == nil {
		return false
	}
	target, ok := resolveAccountExtraNumber(account.Extra, AccountQualityRoutingGroupExtraKey)
	return ok && target > 0 && accountHasGroup(account, int64(target))
}

func accountHasGroup(account *Account, groupID int64) bool {
	if account == nil || groupID <= 0 {
		return false
	}
	for _, id := range account.GroupIDs {
		if id == groupID {
			return true
		}
	}
	for _, group := range account.AccountGroups {
		if group.GroupID == groupID {
			return true
		}
	}
	return false
}

func qualityAnswerPasses(answer string) bool {
	return regexp.MustCompile(`(^|[^0-9])21([^0-9]|$)`).MatchString(answer)
}

func qualityCounters(extra map[string]any) (failures, passes int, status string) {
	if extra == nil {
		return 0, 0, "healthy"
	}
	if value, ok := resolveAccountExtraNumber(extra, accountQualityFailuresExtraKey); ok && value > 0 {
		failures = int(value)
	}
	if value, ok := resolveAccountExtraNumber(extra, accountQualityPassesExtraKey); ok && value > 0 {
		passes = int(value)
	}
	if raw, ok := extra[accountQualityStatusExtraKey].(string); ok && raw == "degraded" {
		status = raw
	} else {
		status = "healthy"
	}
	return
}

func appendUniqueReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

func (s *AccountQualityMonitoringService) switchQualityGroup(ctx context.Context, account *Account, targetID *int64, result *AccountInspectionAccountResult) error {
	if targetID == nil || *targetID <= 0 || s.groupRepo == nil {
		result.QualityAction = "degraded"
		return nil
	}
	target, err := s.groupRepo.GetByID(ctx, *targetID)
	if err != nil || target == nil || target.Status != StatusActive || target.Platform != account.Platform {
		result.QualityAction, result.QualityError = "group_switch_failed", "degraded group is missing, inactive, or platform-mismatched"
		return nil
	}
	original := append([]int64(nil), account.GroupIDs...)
	if len(original) == 1 && original[0] == *targetID {
		result.QualityAction = "degraded_group_already_active"
		return nil
	}
	marker, exists := account.Extra[AccountQualityOriginalGroupsExtraKey]
	if !exists || marker == nil {
		if err := writeQualityExtraErr(s.accountRepo, ctx, account.ID, map[string]any{AccountQualityOriginalGroupsExtraKey: original, AccountQualityRoutingGroupExtraKey: *targetID}); err != nil {
			return err
		}
	}
	if err := s.accountRepo.BindGroups(ctx, account.ID, []int64{*targetID}); err != nil {
		writeQualityExtra(s.accountRepo, ctx, account.ID, map[string]any{AccountQualityOriginalGroupsExtraKey: nil, AccountQualityRoutingGroupExtraKey: nil})
		return err
	}
	result.QualityAction = "switched_group"
	return nil
}

func (s *AccountQualityMonitoringService) restoreQualityGroups(ctx context.Context, account *Account, result *AccountInspectionAccountResult) error {
	if account == nil || len(account.Extra) == 0 {
		result.QualityAction = "healthy"
		return nil
	}
	raw := account.Extra[AccountQualityOriginalGroupsExtraKey]
	if target, exists := account.Extra[AccountQualityRoutingGroupExtraKey]; exists {
		if targetID, ok := resolveAccountExtraNumber(map[string]any{"v": target}, "v"); ok && !sameGroupIDs(account.GroupIDs, []int64{int64(targetID)}) {
			result.QualityAction = "manual_group_change_preserved"
			return nil
		}
	}
	values, ok := raw.([]any)
	if !ok {
		if ids, typed := raw.([]int64); typed {
			if err := s.accountRepo.BindGroups(ctx, account.ID, ids); err != nil {
				return err
			}
			writeQualityExtra(s.accountRepo, ctx, account.ID, map[string]any{AccountQualityOriginalGroupsExtraKey: nil, AccountQualityRoutingGroupExtraKey: nil})
			result.QualityAction = "restored_group"
			return nil
		}
		result.QualityAction = "healthy"
		return nil
	}
	original := make([]int64, 0, len(values))
	for _, value := range values {
		if n, ok := resolveAccountExtraNumber(map[string]any{"v": value}, "v"); ok && n > 0 {
			original = append(original, int64(n))
		}
	}
	if err := s.accountRepo.BindGroups(ctx, account.ID, original); err != nil {
		return err
	}
	writeQualityExtra(s.accountRepo, ctx, account.ID, map[string]any{AccountQualityOriginalGroupsExtraKey: nil, AccountQualityRoutingGroupExtraKey: nil})
	result.QualityAction = "restored_group"
	return nil
}

func sameGroupIDs(left, right []int64) bool {
	if len(left) != len(right) {
		return false
	}
	seen := make(map[int64]struct{}, len(left))
	for _, id := range left {
		seen[id] = struct{}{}
	}
	for _, id := range right {
		if _, ok := seen[id]; !ok {
			return false
		}
	}
	return true
}

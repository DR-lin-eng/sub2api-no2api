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
)

// qualityStageOutcome separates answer failures from operational errors.
type qualityStageOutcome struct {
	status          string
	passed          bool
	operational     bool
	errorMessage    string
	latencyMs       int64
	reasoningTokens *int64
	artifact        *QualityArtifact
	detail          AccountQualityStageDetail
}

// qualityStageContext bounds the initial wait for a drawing response. Once a
// streamed content/image event arrives, the upstream request is allowed to
// finish without the short probe timeout; the outer quality-run context still
// provides the worker's hard safety budget.
func qualityStageContext(ctx context.Context, stage string, timeoutSeconds int) (context.Context, func()) {
	if stage != "stage2" {
		return context.WithTimeout(ctx, time.Duration(timeoutSeconds)*time.Second)
	}
	probeCtx, cancel := context.WithCancel(ctx)
	probeCtx, signal := withQualityProbeOutputSignal(probeCtx)
	timer := time.NewTimer(time.Duration(timeoutSeconds) * time.Second)
	timerDone := make(chan struct{})
	go func() {
		defer close(timerDone)
		select {
		case <-signal.done:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
			cancel()
		case <-probeCtx.Done():
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}
	}()
	cleanup := func() {
		cancel()
		<-timerDone
	}
	return probeCtx, cleanup
}

func (s *AccountQualityMonitoringService) runQualityStage(ctx context.Context, account Account, settings AccountQualitySettings, stage, prompt string, beforeRender ...func()) (outcome qualityStageOutcome) {
	probeCtx, cancel := qualityStageContext(ctx, stage, settings.TimeoutSeconds)
	defer cancel()
	started := time.Now().UTC()
	defer func() { outcome.detail.Status = outcome.status }()
	probe, err := s.accountTestSvc.RunQualityTestBackground(probeCtx, account.ID, strings.TrimSpace(settings.Model), prompt, settings.Effort)
	outcome.status, outcome.operational = "error", true
	if err != nil {
		outcome.errorMessage = err.Error()
		outcome.latencyMs = time.Since(started).Milliseconds()
		return
	}
	if probe == nil {
		outcome.errorMessage = "quality probe returned no result"
		outcome.latencyMs = time.Since(started).Milliseconds()
		return
	}
	outcome.latencyMs, outcome.reasoningTokens = probe.LatencyMs, probe.ReasoningTokens
	outcome.detail = qualityStageDetail(probe)
	if probe.Status != "success" {
		outcome.errorMessage = strings.TrimSpace(probe.ErrorMessage)
		if outcome.errorMessage == "" {
			outcome.errorMessage = "quality probe request failed"
		}
		return outcome
	}
	if stage == "stage1" {
		if !qualityAnswerPassesExpected(probe.ResponseText, settings.Stage1Answer) {
			outcome.status, outcome.operational, outcome.errorMessage = "wrong", false, "answer did not match the configured answer"
			return outcome
		}
		if settings.MinReasoningTokens > 0 {
			if probe.ReasoningTokens == nil {
				outcome.status, outcome.errorMessage = "uncertain", "upstream did not report reasoning tokens"
				outcome.operational = false
				return outcome
			}
			if *probe.ReasoningTokens < settings.MinReasoningTokens {
				outcome.status, outcome.errorMessage = "wrong", fmt.Sprintf("reasoning tokens below threshold: %d < %d", *probe.ReasoningTokens, settings.MinReasoningTokens)
				outcome.operational = false
				return outcome
			}
		}
		outcome.status, outcome.passed, outcome.operational = "passed", true, false
		return outcome
	}
	if s.qualityProcessor == nil {
		outcome.errorMessage = "renderer/classifier is unavailable"
		return outcome
	}
	for _, callback := range beforeRender {
		if callback != nil {
			callback()
		}
	}
	artifact, processErr := s.qualityProcessor.Process(probeCtx, probe.ResponseText)
	if processErr != nil {
		outcome.errorMessage = "render/classification failed: " + processErr.Error()
		return outcome
	}
	outcome.artifact = artifact
	if artifact == nil || artifact.Confidence < settings.MinConfidence {
		outcome.status, outcome.operational, outcome.errorMessage = "uncertain", false, "classifier confidence below threshold"
		return outcome
	}
	switch artifact.Label {
	case "normal":
		outcome.status, outcome.passed, outcome.operational = "passed", true, false
	case "unnormal":
		outcome.status, outcome.passed, outcome.operational = "wrong", false, false
	default:
		outcome.status, outcome.operational, outcome.errorMessage = "uncertain", false, "classifier returned unknown label"
	}

	return outcome
}

// runQualityMonitoring performs enabled stages in order; disabling either
// stage skips its upstream request.
func (s *AccountQualityMonitoringService) runQualityMonitoring(ctx context.Context, accounts []Account, results []AccountInspectionAccountResult, previous *AccountQualityRunState, settings AccountQualitySettings, now time.Time, progress ...func(int, string, bool)) error {
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
	recordErr := func(err error) {
		if err != nil {
			errMu.Lock()
			if firstErr == nil {
				firstErr = err
			}
			errMu.Unlock()
		}
	}
	for i := range accounts {
		account := accounts[i]
		if !qualityProbeEligible(&account, settings.SourceGroupID) {
			continue
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			recordErr(ctx.Err())
			continue
		}
		wg.Add(1)
		go func(index int, account Account) {
			defer wg.Done()
			defer func() { <-sem }()
			result := &results[index]
			notify := func(stage string, done bool) {
				result.QualityPhase = stage
				if done {
					finished := time.Now().UTC()
					result.QualityCompletedAt = &finished
				}
				for _, callback := range progress {
					if callback != nil {
						callback(index, stage, done)
					}
				}
			}
			failures, passes, status := qualityCounters(account.Extra)
			result.QualityStage1Status, result.QualityStage2Status = "disabled", "disabled"
			if !settings.Stage1Enabled && !settings.Stage2Enabled {
				result.QualityStatus = "disabled"
				notify("skipped", true)
				return
			}
			started := time.Now().UTC()
			result.QualityStartedAt = &started
			result.QualityStatus = "running"
			details := AccountQualityProbeDetails{}
			var artifact *QualityArtifact
			var stageErrors []string
			wrong, operational, uncertain := false, false, false
			observe := func(name string, stage qualityStageOutcome) {
				result.QualityLatencyMs += stage.latencyMs
				if stage.status == "wrong" {
					wrong = true
				}
				if stage.operational {
					operational = true
				}
				if stage.status == "uncertain" {
					uncertain = true
				}
				if stage.errorMessage != "" {
					stageErrors = append(stageErrors, name+": "+stage.errorMessage)
				}
			}
			if settings.Stage1Enabled {
				result.QualityStage1Status = "running"
				notify("stage1", false)
				stage := s.runQualityStage(ctx, account, settings, "stage1", settings.Stage1Prompt)
				result.QualityStage1Status, result.QualityReasoningTokens = stage.status, stage.reasoningTokens
				details.Stage1 = &stage.detail
				observe("stage1", stage)
			}
			if settings.Stage2Enabled {
				result.QualityStage2Status = "running"
				notify("stage2", false)
				stage := s.runQualityStage(ctx, account, settings, "stage2", settings.Stage2Prompt, func() { notify("rendering", false) })
				result.QualityStage2Status = stage.status
				details.Stage2, artifact = &stage.detail, stage.artifact
				if stage.artifact != nil {
					result.QualityLabel, result.QualityConfidence = stage.artifact.Label, stage.artifact.Confidence
				}
				observe("stage2", stage)
			}
			result.QualityError = strings.Join(stageErrors, "; ")
			if wrong {
				failures++
				passes = 0
				status = "degraded"
				result.QualityStatus = "degraded"
			} else if operational || uncertain {
				result.QualityStatus = "error"
				if !operational {
					result.QualityStatus = "uncertain"
				}
			} else {
				passes++
				failures = 0
				if prior, ok := previousByID[account.ID]; ok && prior.QualityStatus == "degraded" && passes < settings.RecoveryThreshold {
					status = "degraded"
				} else {
					status = "healthy"
				}
				result.QualityStatus = status
			}
			result.QualityConsecutiveFailures, result.QualityConsecutivePasses = failures, passes
			if status == "degraded" {
				result.Reasons = appendUniqueReason(result.Reasons, "quality_probe_degraded")
			}
			if wrong && failures >= settings.FailureThreshold {
				recordErr(s.switchQualityGroup(ctx, &account, settings.DegradedGroupID, result))
			} else if !wrong && !operational && !uncertain && status == "healthy" && passes >= settings.RecoveryThreshold {
				recordErr(s.restoreQualityGroups(ctx, &account, result))
			}
			outcome := "passed"
			if wrong {
				outcome = "wrong"
			}
			if operational {
				outcome = "error"
			}
			if uncertain {
				outcome = "uncertain"
			}
			if wrong {
				outcome = "wrong"
			} // Match the combined verdict when another stage errors.
			notify("saving", false)
			if s.qualityArtifacts != nil {
				run := &AccountQualityRun{AccountID: account.ID, Model: strings.TrimSpace(settings.Model), Effort: settings.Effort, Status: outcome, StartedAt: started, FinishedAt: time.Now().UTC(), LatencyMs: result.QualityLatencyMs, Details: details, Error: result.QualityError}
				if outcome == "passed" {
					run.Status = "ready"
				}
				if artifact != nil {
					run.Label, run.Confidence, run.ModelVersion = artifact.Label, artifact.Confidence, artifact.ModelVersion
					run.PNG, run.WebP = artifact.PNG, artifact.WebP
				}
				if err := s.qualityArtifacts.Save(ctx, run); err != nil {
					recordErr(fmt.Errorf("save quality conversation: %w", err))
				} else {
					result.QualityRunID = run.ID
				}
			}
			updates := qualityExtraUpdate(account.Extra, status, failures, passes, now, outcome, result.QualityError)
			if history, ok := updates[accountQualityHistoryExtraKey].([]map[string]any); ok && len(history) > 0 {
				entry := history[len(history)-1]
				entry["stage1_status"], entry["stage2_status"] = result.QualityStage1Status, result.QualityStage2Status
				if result.QualityReasoningTokens != nil {
					entry["reasoning_tokens"] = *result.QualityReasoningTokens
				}
			}
			recordErr(writeQualityExtraErr(s.accountRepo, ctx, account.ID, updates))
			notify("complete", true)
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

func qualityAnswerPassesExpected(answer, expected string) bool {
	// Stage one is an exact-answer gate: any explanation, alternate number, or
	// extra text is considered a failed answer. Keep the legacy
	// qualityAnswerPasses helper for the account-inspection compatibility path.
	expected = strings.TrimSpace(expected)
	if expected == "" {
		expected = accountQualityDefaultStage1Answer
	}
	return strings.TrimSpace(answer) == expected
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

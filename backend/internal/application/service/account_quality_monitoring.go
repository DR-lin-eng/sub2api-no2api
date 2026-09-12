package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/google/uuid"
)

const (
	accountQualityLeaderLockKey            = "account-quality:run:leader"
	accountQualityStateKey                 = SettingKeyAccountQualityState
	accountQualityLeaderLockTTL            = 6 * time.Minute
	accountQualityDefaultIntervalMinutes   = 10
	accountQualityDefaultFailureThreshold  = 2
	accountQualityDefaultRecoveryThreshold = 2
	accountQualityMaxConcurrent            = 4
	accountQualityDefaultProbeTimeoutSec   = 120
	accountQualityMinProbeTimeoutSec       = 30
	accountQualityMaxProbeTimeoutSec       = 300
	accountQualityDefaultPrompt            = "创建一个html，内容是SVG绘制一个鹈鹕骑自行车的2D动画。页面必须自包含，只使用内联SVG、CSS关键帧动画和少量JavaScript；鹈鹕、车轮、脚踏、道路和背景都要画出来，动画要可见。只返回完整HTML源码，不要 Markdown 代码围栏，不要解释。"
)

// AccountQualitySettings owns the account-level quality probe policy. It is
// deliberately separate from AccountInspectionSettings so saving or running
// one policy never starts the other policy.
type AccountQualitySettings struct {
	Enabled           bool    `json:"enabled"`
	IntervalMinutes   int     `json:"interval_minutes"`
	TimeoutSeconds    int     `json:"timeout_seconds"`
	Model             string  `json:"model"`
	Effort            string  `json:"effort"`
	Prompt            string  `json:"prompt"`
	FailureThreshold  int     `json:"failure_threshold"`
	RecoveryThreshold int     `json:"recovery_threshold"`
	DegradedGroupID   *int64  `json:"degraded_group_id"`
	SourceGroupID     *int64  `json:"source_group_id"`
	MaxConcurrent     int     `json:"max_concurrent"`
	MinConfidence     float64 `json:"min_confidence"`
	PublicEnabled     bool    `json:"public_enabled"`
}

func DefaultAccountQualitySettings() AccountQualitySettings {
	return AccountQualitySettings{
		IntervalMinutes:   accountQualityDefaultIntervalMinutes,
		TimeoutSeconds:    accountQualityDefaultProbeTimeoutSec,
		Effort:            "medium",
		Prompt:            accountQualityDefaultPrompt,
		FailureThreshold:  accountQualityDefaultFailureThreshold,
		RecoveryThreshold: accountQualityDefaultRecoveryThreshold,
		MaxConcurrent:     accountQualityMaxConcurrent,
		MinConfidence:     0.85,
	}
}

func (s *AccountQualitySettings) normalize() {
	if s.IntervalMinutes < 1 {
		s.IntervalMinutes = accountQualityDefaultIntervalMinutes
	}
	if s.TimeoutSeconds <= 0 {
		s.TimeoutSeconds = accountQualityDefaultProbeTimeoutSec
	}
	if s.FailureThreshold < 1 {
		s.FailureThreshold = accountQualityDefaultFailureThreshold
	}
	if s.RecoveryThreshold < 1 {
		s.RecoveryThreshold = accountQualityDefaultRecoveryThreshold
	}
	if strings.TrimSpace(s.Prompt) == "" {
		s.Prompt = accountQualityDefaultPrompt
	}
	s.Effort = strings.ToLower(strings.TrimSpace(s.Effort))
	switch s.Effort {
	case "minimal", "low", "medium", "high", "xhigh", "max":
	default:
		s.Effort = "medium"
	}
	if s.MaxConcurrent < 1 {
		s.MaxConcurrent = 1
	}
	if s.MaxConcurrent > accountQualityMaxConcurrent {
		s.MaxConcurrent = accountQualityMaxConcurrent
	}
	if s.MinConfidence <= 0 {
		s.MinConfidence = 0.85
	}
	if s.MinConfidence > 1 {
		s.MinConfidence = 1
	}
}

func (s AccountQualitySettings) validate() error {
	if s.IntervalMinutes < 1 || s.TimeoutSeconds < accountQualityMinProbeTimeoutSec || s.TimeoutSeconds > accountQualityMaxProbeTimeoutSec || s.FailureThreshold < 1 || s.RecoveryThreshold < 1 || s.MaxConcurrent < 1 || s.MaxConcurrent > accountQualityMaxConcurrent {
		return infraerrors.BadRequest("INVALID_ACCOUNT_QUALITY_SETTINGS", "quality settings are invalid")
	}
	if s.MinConfidence <= 0 || s.MinConfidence > 1 {
		return infraerrors.BadRequest("INVALID_ACCOUNT_QUALITY_CONFIDENCE", "min_confidence must be between 0 and 1")
	}
	if s.SourceGroupID != nil && *s.SourceGroupID <= 0 {
		return infraerrors.BadRequest("INVALID_ACCOUNT_QUALITY_SOURCE_GROUP", "source_group_id must be positive")
	}
	if s.DegradedGroupID != nil && *s.DegradedGroupID <= 0 {
		return infraerrors.BadRequest("INVALID_ACCOUNT_QUALITY_DEGRADED_GROUP", "degraded_group_id must be positive")
	}
	if s.SourceGroupID != nil && s.DegradedGroupID != nil && *s.SourceGroupID == *s.DegradedGroupID {
		return infraerrors.BadRequest("INVALID_ACCOUNT_QUALITY_GROUPS", "source and degraded groups must differ")
	}
	return nil
}

type AccountQualitySummary struct {
	Inspected int `json:"inspected"`
	Passed    int `json:"passed"`
	Degraded  int `json:"degraded"`
	Uncertain int `json:"uncertain"`
	Errors    int `json:"errors"`
	Switched  int `json:"switched"`
}

type AccountQualityRunState struct {
	RunID            string                           `json:"run_id,omitempty"`
	Status           string                           `json:"status"`
	Trigger          string                           `json:"trigger,omitempty"`
	StartedAt        *time.Time                       `json:"started_at,omitempty"`
	CompletedAt      *time.Time                       `json:"completed_at,omitempty"`
	NextRunAt        *time.Time                       `json:"next_run_at,omitempty"`
	LastRunAt        *time.Time                       `json:"last_run_at,omitempty"`
	Summary          AccountQualitySummary            `json:"summary"`
	Results          []AccountInspectionAccountResult `json:"results,omitempty"`
	ResultsTruncated bool                             `json:"results_truncated,omitempty"`
	Error            string                           `json:"error,omitempty"`
}

type AccountQualityOverview struct {
	Settings AccountQualitySettings `json:"settings"`
	Run      AccountQualityRunView  `json:"run"`
	Results  AccountInspectionPage  `json:"results"`
}

type AccountQualityRunView struct {
	RunID            string                `json:"run_id,omitempty"`
	Status           string                `json:"status"`
	Trigger          string                `json:"trigger,omitempty"`
	StartedAt        *time.Time            `json:"started_at,omitempty"`
	CompletedAt      *time.Time            `json:"completed_at,omitempty"`
	NextRunAt        *time.Time            `json:"next_run_at,omitempty"`
	LastRunAt        *time.Time            `json:"last_run_at,omitempty"`
	Summary          AccountQualitySummary `json:"summary"`
	ResultsTruncated bool                  `json:"results_truncated,omitempty"`
	Error            string                `json:"error,omitempty"`
}

func qualityRunView(state *AccountQualityRunState) AccountQualityRunView {
	if state == nil {
		return AccountQualityRunView{Status: AccountInspectionStatusIdle}
	}
	return AccountQualityRunView{RunID: state.RunID, Status: state.Status, Trigger: state.Trigger, StartedAt: state.StartedAt, CompletedAt: state.CompletedAt, NextRunAt: state.NextRunAt, LastRunAt: state.LastRunAt, Summary: state.Summary, ResultsTruncated: state.ResultsTruncated, Error: state.Error}
}

type AccountQualityMonitoringService struct {
	accountRepo      AccountRepository
	settingRepo      SettingRepository
	accountTestSvc   AccountQualityProbeRunner
	qualityProcessor AccountQualityArtifactProcessor
	qualityArtifacts AccountQualityArtifactRepository
	groupRepo        GroupRepository
	lockCache        LeaderLockCache
	db               *sql.DB
	instanceID       string

	parentCtx    context.Context
	parentCancel context.CancelFunc
	startOnce    sync.Once
	stopOnce     sync.Once
	wg           sync.WaitGroup
	running      atomic.Bool
}

func NewAccountQualityMonitoringService(accountRepo AccountRepository, settingRepo SettingRepository, probe AccountQualityProbeRunner, processor AccountQualityArtifactProcessor, artifacts AccountQualityArtifactRepository, groups GroupRepository) *AccountQualityMonitoringService {
	ctx, cancel := context.WithCancel(context.Background())
	return &AccountQualityMonitoringService{accountRepo: accountRepo, settingRepo: settingRepo, accountTestSvc: probe, qualityProcessor: processor, qualityArtifacts: artifacts, groupRepo: groups, instanceID: uuid.NewString(), parentCtx: ctx, parentCancel: cancel}
}

func (s *AccountQualityMonitoringService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s != nil {
		s.lockCache, s.db = lockCache, db
	}
}

func (s *AccountQualityMonitoringService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() { s.wg.Add(1); go s.runLoop() })
}

func (s *AccountQualityMonitoringService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() { s.parentCancel(); s.wg.Wait() })
}

func (s *AccountQualityMonitoringService) runLoop() {
	defer s.wg.Done()
	ticker := time.NewTicker(accountInspectionTickInterval)
	defer ticker.Stop()
	s.runDue()
	for {
		select {
		case <-s.parentCtx.Done():
			return
		case <-ticker.C:
			s.runDue()
		}
	}
}

func (s *AccountQualityMonitoringService) runDue() {
	ctx, cancel := context.WithTimeout(s.parentCtx, accountInspectionRunTimeout)
	defer cancel()
	settings, err := s.GetSettings(ctx)
	if err != nil || !settings.Enabled {
		return
	}
	state, err := s.loadState(ctx)
	if err != nil || (state.Status == AccountInspectionStatusRunning && state.StartedAt != nil && time.Since(*state.StartedAt) < accountInspectionRunTimeout) {
		return
	}
	now := time.Now()
	if state.LastRunAt != nil && now.Before(state.LastRunAt.Add(time.Duration(settings.IntervalMinutes)*time.Minute)) {
		return
	}
	_, _ = s.RunNow(ctx, "scheduled")
}

func (s *AccountQualityMonitoringService) GetSettings(ctx context.Context) (AccountQualitySettings, error) {
	defaults := DefaultAccountQualitySettings()
	if s == nil || s.settingRepo == nil {
		return defaults, nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAccountQualitySettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			// Upgrade compatibility: import the former nested quality policy
			// once, while all future writes use the independent key.
			legacyRaw, legacyErr := s.settingRepo.GetValue(ctx, SettingKeyAccountInspectionSettings)
			if legacyErr == nil && strings.TrimSpace(legacyRaw) != "" {
				var legacy struct {
					Enabled           bool    `json:"quality_monitoring_enabled"`
					IntervalMinutes   int     `json:"quality_interval_minutes"`
					TimeoutSeconds    int     `json:"quality_timeout_seconds"`
					Model             string  `json:"quality_model"`
					Effort            string  `json:"quality_effort"`
					Prompt            string  `json:"quality_prompt"`
					FailureThreshold  int     `json:"quality_failure_threshold"`
					RecoveryThreshold int     `json:"quality_recovery_threshold"`
					DegradedGroupID   *int64  `json:"quality_degraded_group_id"`
					SourceGroupID     *int64  `json:"quality_source_group_id"`
					MaxConcurrent     int     `json:"quality_max_concurrent"`
					MinConfidence     float64 `json:"quality_min_confidence"`
					PublicEnabled     bool    `json:"quality_public_enabled"`
				}
				if json.Unmarshal([]byte(legacyRaw), &legacy) == nil {
					settings := AccountQualitySettings{Enabled: legacy.Enabled, IntervalMinutes: legacy.IntervalMinutes, TimeoutSeconds: legacy.TimeoutSeconds, Model: legacy.Model, Effort: legacy.Effort, Prompt: legacy.Prompt, FailureThreshold: legacy.FailureThreshold, RecoveryThreshold: legacy.RecoveryThreshold, DegradedGroupID: legacy.DegradedGroupID, SourceGroupID: legacy.SourceGroupID, MaxConcurrent: legacy.MaxConcurrent, MinConfidence: legacy.MinConfidence, PublicEnabled: legacy.PublicEnabled}
					settings.normalize()
					return settings, nil
				}
			}
			return defaults, nil
		}
		return defaults, fmt.Errorf("get account quality settings: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return defaults, nil
	}
	settings := defaults
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return defaults, fmt.Errorf("parse account quality settings: %w", err)
	}
	settings.normalize()
	return settings, nil
}

func (s *AccountQualityMonitoringService) UpdateSettings(ctx context.Context, settings *AccountQualitySettings) (AccountQualitySettings, error) {
	if s == nil || s.settingRepo == nil {
		return AccountQualitySettings{}, ErrAccountInspectionUnavailable
	}
	if settings == nil {
		return AccountQualitySettings{}, infraerrors.BadRequest("INVALID_ACCOUNT_QUALITY_SETTINGS", "settings cannot be nil")
	}
	normalized := *settings
	normalized.normalize()
	if err := normalized.validate(); err != nil {
		return AccountQualitySettings{}, err
	}
	if err := s.validateQualityGroups(ctx, normalized); err != nil {
		return AccountQualitySettings{}, err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return AccountQualitySettings{}, fmt.Errorf("marshal account quality settings: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyAccountQualitySettings, string(data)); err != nil {
		return AccountQualitySettings{}, fmt.Errorf("save account quality settings: %w", err)
	}
	return normalized, nil
}

func (s *AccountQualityMonitoringService) GetOverview(ctx context.Context, filter AccountInspectionListFilter) (*AccountQualityOverview, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	state, err := s.loadState(ctx)
	if err != nil {
		return nil, err
	}
	filtered := filterQualityResults(state.Results, filter)
	page, _ := paginateInspectionResults(filtered, filter)
	return &AccountQualityOverview{Settings: settings, Run: qualityRunView(state), Results: page}, nil
}

func (s *AccountQualityMonitoringService) RunNow(ctx context.Context, trigger string) (*AccountQualityRunState, error) {
	if s == nil || s.accountRepo == nil || s.accountTestSvc == nil {
		return nil, ErrAccountInspectionUnavailable
	}
	if !s.running.CompareAndSwap(false, true) {
		return nil, ErrAccountInspectionBusy
	}
	defer s.running.Store(false)
	release, acquired := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, accountQualityLeaderLockKey, s.instanceID, accountQualityLeaderLockTTL)
	if !acquired {
		return nil, ErrAccountInspectionBusy
	}
	defer release()
	return s.execute(ctx, trigger)
}

func (s *AccountQualityMonitoringService) execute(ctx context.Context, trigger string) (*AccountQualityRunState, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.validateQualityGroups(ctx, settings); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	previous, _ := s.loadState(ctx)
	state := &AccountQualityRunState{RunID: uuid.NewString(), Status: AccountInspectionStatusRunning, Trigger: trigger, StartedAt: &now, Results: []AccountInspectionAccountResult{}}
	if err := s.saveState(ctx, state); err != nil {
		return nil, err
	}
	accounts, err := s.accountRepo.ListAllWithFilters(ctx, "", "", "", "", 0, "")
	if err != nil {
		return s.failState(ctx, state, err)
	}
	eligible := make([]Account, 0, len(accounts))
	for i := range accounts {
		if qualityProbeSupported(&accounts[i]) {
			eligible = append(eligible, accounts[i])
		}
	}
	results := make([]AccountInspectionAccountResult, len(eligible))
	for i := range eligible {
		results[i] = neutralAccountInspectionResult(&eligible[i], now)
	}
	if err := s.runQualityMonitoring(ctx, eligible, results, previous, settings, now); err != nil {
		return s.failState(ctx, state, err)
	}
	state.Results = results
	state.Summary = summarizeQualityResults(results)
	state.Summary.Inspected = len(eligible)
	state.LastRunAt = &now
	completed := time.Now().UTC()
	state.Status, state.CompletedAt = AccountInspectionStatusSucceeded, &completed
	next := completed.Add(time.Duration(settings.IntervalMinutes) * time.Minute)
	state.NextRunAt = &next
	if len(results) > accountInspectionMaxStoredResults {
		state.Results = results[:accountInspectionMaxStoredResults]
		state.ResultsTruncated = true
	}
	if err := s.saveState(ctx, state); err != nil {
		return nil, err
	}
	return state, nil
}

func (s *AccountQualityMonitoringService) failState(ctx context.Context, state *AccountQualityRunState, runErr error) (*AccountQualityRunState, error) {
	now := time.Now().UTC()
	state.Status, state.CompletedAt = AccountInspectionStatusFailed, &now
	state.Error = truncateQualityError(strings.TrimSpace(runErr.Error()))
	_ = s.saveState(ctx, state)
	return nil, runErr
}

func (s *AccountQualityMonitoringService) loadState(ctx context.Context) (*AccountQualityRunState, error) {
	state := &AccountQualityRunState{Status: AccountInspectionStatusIdle, Results: []AccountInspectionAccountResult{}}
	if s == nil || s.settingRepo == nil {
		return state, nil
	}
	raw, err := s.settingRepo.GetValue(ctx, accountQualityStateKey)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return state, nil
		}
		return nil, fmt.Errorf("get account quality state: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return state, nil
	}
	if err := json.Unmarshal([]byte(raw), state); err != nil {
		return nil, fmt.Errorf("parse account quality state: %w", err)
	}
	if state.Results == nil {
		state.Results = []AccountInspectionAccountResult{}
	}
	for i := range state.Results {
		if state.Results[i].Reasons == nil {
			state.Results[i].Reasons = []string{}
		}
	}
	return state, nil
}

func (s *AccountQualityMonitoringService) saveState(ctx context.Context, state *AccountQualityRunState) error {
	if s == nil || s.settingRepo == nil {
		return nil
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal account quality state: %w", err)
	}
	return s.settingRepo.Set(ctx, accountQualityStateKey, string(data))
}

func filterQualityResults(results []AccountInspectionAccountResult, filter AccountInspectionListFilter) []AccountInspectionAccountResult {
	search := strings.ToLower(strings.TrimSpace(filter.Search))
	out := make([]AccountInspectionAccountResult, 0, len(results))
	for _, result := range results {
		if filter.Type != "" && filter.Type != "all" && result.Type != filter.Type {
			continue
		}
		if filter.Status == "degraded" && result.QualityStatus != "degraded" {
			continue
		}
		if filter.Status == "healthy" && result.QualityStatus != "healthy" {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(result.Name), search) && !strings.Contains(fmt.Sprint(result.AccountID), search) {
			continue
		}
		out = append(out, result)
	}
	return out
}

func paginateInspectionResults(results []AccountInspectionAccountResult, filter AccountInspectionListFilter) (AccountInspectionPage, int) {
	page, pageSize := filter.Page, filter.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}
	pages := (len(results) + pageSize - 1) / pageSize
	if pages < 1 {
		pages = 1
	}
	start := (page - 1) * pageSize
	items := []AccountInspectionAccountResult{}
	if start < len(results) {
		end := start + pageSize
		if end > len(results) {
			end = len(results)
		}
		items = results[start:end]
	}
	return AccountInspectionPage{Items: items, Total: len(results), Page: page, PageSize: pageSize, Pages: pages}, pageSize
}

func summarizeQualityResults(results []AccountInspectionAccountResult) AccountQualitySummary {
	var summary AccountQualitySummary
	for _, result := range results {
		switch result.QualityStatus {
		case "healthy":
			summary.Passed++
		case "degraded":
			summary.Degraded++
		case "uncertain":
			summary.Uncertain++
		case "error":
			summary.Errors++
		}
		if result.QualityAction == "switched_group" {
			summary.Switched++
		}
	}
	return summary
}

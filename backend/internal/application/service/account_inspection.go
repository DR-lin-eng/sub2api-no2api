package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/Wei-Shaw/sub2api/internal/shared/usagestats"
	"github.com/google/uuid"
)

const (
	AccountInspectionStatusIdle      = "idle"
	AccountInspectionStatusRunning   = "running"
	AccountInspectionStatusSucceeded = "succeeded"
	AccountInspectionStatusFailed    = "failed"

	AccountInspectionActionNone            = "none"
	AccountInspectionActionReported        = "reported"
	AccountInspectionActionDisabled        = "disabled"
	AccountInspectionActionAlreadyDisabled = "already_disabled"
	AccountInspectionActionError           = "error"

	AccountInspectionTypeOAuth   = "oauth"
	AccountInspectionTypeAPIKey  = "apikey"
	AccountInspectionTypeBedrock = "bedrock"

	accountInspectionDefaultIntervalMinutes = 60
	accountInspectionMinIntervalMinutes     = 5
	accountInspectionMaxIntervalMinutes     = 24 * 60
	accountInspectionDefaultLookbackMinutes = 60
	accountInspectionMinLookbackMinutes     = 15
	accountInspectionMaxLookbackMinutes     = 24 * 60
	accountInspectionDefaultTTFTMs          = 30_000
	accountInspectionDefaultSuccessRate     = 0.60
	accountInspectionDefaultMinRequests     = 1
	accountInspectionMaxStoredResults       = 5000
	accountInspectionRunTimeout             = 5 * time.Minute
	accountInspectionLeaderLockKey          = "account-inspection:run:leader"
	accountInspectionLeaderLockTTL          = 6 * time.Minute
	accountInspectionTickInterval           = time.Minute
)

var (
	ErrAccountInspectionUnavailable = infraerrors.ServiceUnavailable(
		"ACCOUNT_INSPECTION_UNAVAILABLE", "account inspection is unavailable",
	)
	ErrAccountInspectionBusy = infraerrors.Conflict(
		"ACCOUNT_INSPECTION_BUSY", "an account inspection is already running",
	)
)

// AccountInspectionSettings controls both the periodic runner and the manual run.
// A zero API-key cache/multiplier threshold means "display only".
type AccountInspectionSettings struct {
	Enabled                 bool    `json:"enabled"`
	IntervalMinutes         int     `json:"interval_minutes"`
	AutoDisable             bool    `json:"auto_disable"`
	LookbackMinutes         int     `json:"lookback_minutes"`
	MinRequests             int     `json:"min_requests"`
	TTFTThresholdMs         int     `json:"ttft_threshold_ms"`
	SuccessRateThreshold    float64 `json:"success_rate_threshold"`
	OAuthQuotaCheckEnabled  bool    `json:"oauth_quota_check_enabled"`
	APIKeyQuotaCheckEnabled bool    `json:"api_key_quota_check_enabled"`
	APIKeyMinCacheHitRate   float64 `json:"api_key_min_cache_hit_rate"`
	APIKeyMaxRateMultiplier float64 `json:"api_key_max_rate_multiplier"`
	APIKeyMinRemainingQuota float64 `json:"api_key_min_remaining_quota"`
}

func DefaultAccountInspectionSettings() AccountInspectionSettings {
	return AccountInspectionSettings{
		Enabled:                 false,
		IntervalMinutes:         accountInspectionDefaultIntervalMinutes,
		AutoDisable:             true,
		LookbackMinutes:         accountInspectionDefaultLookbackMinutes,
		MinRequests:             accountInspectionDefaultMinRequests,
		TTFTThresholdMs:         accountInspectionDefaultTTFTMs,
		SuccessRateThreshold:    accountInspectionDefaultSuccessRate,
		OAuthQuotaCheckEnabled:  true,
		APIKeyQuotaCheckEnabled: true,
	}
}

func (s *AccountInspectionSettings) normalize() {
	if s.IntervalMinutes < accountInspectionMinIntervalMinutes {
		s.IntervalMinutes = accountInspectionMinIntervalMinutes
	}
	if s.IntervalMinutes > accountInspectionMaxIntervalMinutes {
		s.IntervalMinutes = accountInspectionMaxIntervalMinutes
	}
	if s.LookbackMinutes < accountInspectionMinLookbackMinutes {
		s.LookbackMinutes = accountInspectionMinLookbackMinutes
	}
	if s.LookbackMinutes > accountInspectionMaxLookbackMinutes {
		s.LookbackMinutes = accountInspectionMaxLookbackMinutes
	}
	if s.MinRequests < 1 {
		s.MinRequests = 1
	}
	if s.TTFTThresholdMs < 0 {
		s.TTFTThresholdMs = 0
	}
	if s.SuccessRateThreshold < 0 {
		s.SuccessRateThreshold = 0
	}
	if s.SuccessRateThreshold > 1 {
		s.SuccessRateThreshold = 1
	}
	if s.APIKeyMinCacheHitRate < 0 {
		s.APIKeyMinCacheHitRate = 0
	}
	if s.APIKeyMinCacheHitRate > 1 {
		s.APIKeyMinCacheHitRate = 1
	}
	if s.APIKeyMaxRateMultiplier < 0 {
		s.APIKeyMaxRateMultiplier = 0
	}
	if s.APIKeyMinRemainingQuota < 0 {
		s.APIKeyMinRemainingQuota = 0
	}
}

func (s AccountInspectionSettings) validate() error {
	if s.IntervalMinutes < accountInspectionMinIntervalMinutes || s.IntervalMinutes > accountInspectionMaxIntervalMinutes {
		return infraerrors.BadRequest("INVALID_ACCOUNT_INSPECTION_INTERVAL", fmt.Sprintf("interval_minutes must be between %d and %d", accountInspectionMinIntervalMinutes, accountInspectionMaxIntervalMinutes))
	}
	if s.LookbackMinutes < accountInspectionMinLookbackMinutes || s.LookbackMinutes > accountInspectionMaxLookbackMinutes {
		return infraerrors.BadRequest("INVALID_ACCOUNT_INSPECTION_LOOKBACK", fmt.Sprintf("lookback_minutes must be between %d and %d", accountInspectionMinLookbackMinutes, accountInspectionMaxLookbackMinutes))
	}
	if s.MinRequests < 1 {
		return infraerrors.BadRequest("INVALID_ACCOUNT_INSPECTION_MIN_REQUESTS", "min_requests must be at least 1")
	}
	if s.TTFTThresholdMs < 0 || s.SuccessRateThreshold < 0 || s.SuccessRateThreshold > 1 {
		return infraerrors.BadRequest("INVALID_ACCOUNT_INSPECTION_THRESHOLDS", "TTFT and success-rate thresholds are invalid")
	}
	if s.APIKeyMinCacheHitRate < 0 || s.APIKeyMinCacheHitRate > 1 || s.APIKeyMaxRateMultiplier < 0 || s.APIKeyMinRemainingQuota < 0 {
		return infraerrors.BadRequest("INVALID_ACCOUNT_INSPECTION_API_KEY_THRESHOLDS", "API key inspection thresholds are invalid")
	}
	return nil
}

type AccountInspectionAccountResult struct {
	AccountID               int64     `json:"account_id"`
	Name                    string    `json:"name"`
	Platform                string    `json:"platform"`
	Type                    string    `json:"type"`
	Status                  string    `json:"status"`
	Schedulable             bool      `json:"schedulable"`
	Action                  string    `json:"action"`
	Reasons                 []string  `json:"reasons,omitempty"`
	TotalRequests           int64     `json:"total_requests"`
	SuccessfulRequests      int64     `json:"successful_requests"`
	SuccessRate             *float64  `json:"success_rate,omitempty"`
	AvgFirstTokenMs         *float64  `json:"avg_first_token_ms,omitempty"`
	CacheHitRate            *float64  `json:"cache_hit_rate,omitempty"`
	CacheReadTokens         int64     `json:"cache_read_tokens,omitempty"`
	CacheCreationTokens     int64     `json:"cache_creation_tokens,omitempty"`
	RateMultiplier          *float64  `json:"rate_multiplier,omitempty"`
	RemainingQuota          *float64  `json:"remaining_quota,omitempty"`
	RemainingQuotaDimension string    `json:"remaining_quota_dimension,omitempty"`
	QuotaUnlimited          bool      `json:"quota_unlimited,omitempty"`
	QuotaUsedPercent        *float64  `json:"quota_used_percent,omitempty"`
	QuotaUsageDimension     string    `json:"quota_usage_dimension,omitempty"`
	ObservedAt              time.Time `json:"observed_at"`
}

type AccountInspectionQuotaBucket struct {
	Key        string   `json:"key"`
	MinPercent float64  `json:"min_percent"`
	MaxPercent *float64 `json:"max_percent,omitempty"`
	Count      int      `json:"count"`
}

type AccountInspectionQuotaDistribution struct {
	AverageUsedPercent *float64                       `json:"average_used_percent,omitempty"`
	MeasuredAccounts   int                            `json:"measured_accounts"`
	UnknownAccounts    int                            `json:"unknown_accounts"`
	Buckets            []AccountInspectionQuotaBucket `json:"buckets"`
}

type AccountInspectionSummary struct {
	Inspected              int                                `json:"inspected"`
	Healthy                int                                `json:"healthy"`
	Flagged                int                                `json:"flagged"`
	Disabled               int                                `json:"disabled"`
	AlreadyDisabled        int                                `json:"already_disabled"`
	OAuthAccounts          int                                `json:"oauth_accounts"`
	APIKeyAccounts         int                                `json:"api_key_accounts"`
	QuotaUsageDistribution AccountInspectionQuotaDistribution `json:"quota_usage_distribution"`
}

type AccountInspectionRunState struct {
	RunID            string                           `json:"run_id,omitempty"`
	Status           string                           `json:"status"`
	Trigger          string                           `json:"trigger,omitempty"`
	StartedAt        *time.Time                       `json:"started_at,omitempty"`
	CompletedAt      *time.Time                       `json:"completed_at,omitempty"`
	NextRunAt        *time.Time                       `json:"next_run_at,omitempty"`
	Summary          AccountInspectionSummary         `json:"summary"`
	Results          []AccountInspectionAccountResult `json:"results,omitempty"`
	ResultsTruncated bool                             `json:"results_truncated,omitempty"`
	Error            string                           `json:"error,omitempty"`
}

type AccountInspectionListFilter struct {
	Page     int
	PageSize int
	Status   string
	Type     string
	Search   string
}

type AccountInspectionPage struct {
	Items    []AccountInspectionAccountResult `json:"items"`
	Total    int                              `json:"total"`
	Page     int                              `json:"page"`
	PageSize int                              `json:"page_size"`
	Pages    int                              `json:"pages"`
}

type AccountInspectionOverview struct {
	Settings AccountInspectionSettings `json:"settings"`
	Run      AccountInspectionRunView  `json:"run"`
	Results  AccountInspectionPage     `json:"results"`
}

type AccountInspectionRunView struct {
	RunID            string                   `json:"run_id,omitempty"`
	Status           string                   `json:"status"`
	Trigger          string                   `json:"trigger,omitempty"`
	StartedAt        *time.Time               `json:"started_at,omitempty"`
	CompletedAt      *time.Time               `json:"completed_at,omitempty"`
	NextRunAt        *time.Time               `json:"next_run_at,omitempty"`
	Summary          AccountInspectionSummary `json:"summary"`
	ResultsTruncated bool                     `json:"results_truncated,omitempty"`
	Error            string                   `json:"error,omitempty"`
}

func inspectionRunView(state AccountInspectionRunState) AccountInspectionRunView {
	return AccountInspectionRunView{
		RunID: state.RunID, Status: state.Status, Trigger: state.Trigger,
		StartedAt: state.StartedAt, CompletedAt: state.CompletedAt,
		NextRunAt: state.NextRunAt, Summary: state.Summary, Error: state.Error,
		ResultsTruncated: state.ResultsTruncated,
	}
}

type AccountInspectionService struct {
	accountRepo  AccountRepository
	usageService *AccountUsageService
	settingRepo  SettingRepository
	lockCache    LeaderLockCache
	db           *sql.DB
	instanceID   string

	parentCtx    context.Context
	parentCancel context.CancelFunc
	startOnce    sync.Once
	stopOnce     sync.Once
	wg           sync.WaitGroup
	running      atomic.Bool
}

func NewAccountInspectionService(accountRepo AccountRepository, usageService *AccountUsageService, settingRepo SettingRepository) *AccountInspectionService {
	ctx, cancel := context.WithCancel(context.Background())
	return &AccountInspectionService{
		accountRepo:  accountRepo,
		usageService: usageService,
		settingRepo:  settingRepo,
		instanceID:   uuid.NewString(),
		parentCtx:    ctx,
		parentCancel: cancel,
	}
}

func (s *AccountInspectionService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

func (s *AccountInspectionService) Start() {
	if s == nil {
		return
	}
	s.startOnce.Do(func() {
		s.wg.Add(1)
		go s.runLoop()
	})
}

func (s *AccountInspectionService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		s.parentCancel()
		s.wg.Wait()
	})
}

func (s *AccountInspectionService) runLoop() {
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

func (s *AccountInspectionService) runDue() {
	ctx, cancel := context.WithTimeout(s.parentCtx, accountInspectionRunTimeout)
	defer cancel()
	settings, err := s.GetSettings(ctx)
	if err != nil || !settings.Enabled {
		return
	}
	state, err := s.loadState(ctx)
	if err != nil {
		return
	}
	if state.Status == AccountInspectionStatusRunning && state.StartedAt != nil && time.Since(*state.StartedAt) < accountInspectionRunTimeout {
		return
	}
	if state.CompletedAt != nil && time.Now().Before(state.CompletedAt.Add(time.Duration(settings.IntervalMinutes)*time.Minute)) {
		return
	}
	if _, err := s.RunNow(ctx, "scheduled"); err != nil && !errors.Is(err, ErrAccountInspectionBusy) {
		// The next tick retries a failed run; the state keeps the visible error.
		return
	}
}

func (s *AccountInspectionService) GetSettings(ctx context.Context) (AccountInspectionSettings, error) {
	defaults := DefaultAccountInspectionSettings()
	if s == nil || s.settingRepo == nil {
		return defaults, nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAccountInspectionSettings)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return defaults, nil
		}
		return defaults, fmt.Errorf("get account inspection settings: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return defaults, nil
	}
	settings := defaults
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return defaults, fmt.Errorf("parse account inspection settings: %w", err)
	}
	settings.normalize()
	return settings, nil
}

func (s *AccountInspectionService) UpdateSettings(ctx context.Context, settings *AccountInspectionSettings) (AccountInspectionSettings, error) {
	if s == nil || s.settingRepo == nil {
		return AccountInspectionSettings{}, ErrAccountInspectionUnavailable
	}
	if settings == nil {
		return AccountInspectionSettings{}, infraerrors.BadRequest("INVALID_ACCOUNT_INSPECTION_SETTINGS", "settings cannot be nil")
	}
	normalized := *settings
	normalized.normalize()
	if err := normalized.validate(); err != nil {
		return AccountInspectionSettings{}, err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return AccountInspectionSettings{}, fmt.Errorf("marshal account inspection settings: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyAccountInspectionSettings, string(data)); err != nil {
		return AccountInspectionSettings{}, fmt.Errorf("save account inspection settings: %w", err)
	}
	return normalized, nil
}

func (s *AccountInspectionService) GetOverview(ctx context.Context, filter AccountInspectionListFilter) (*AccountInspectionOverview, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	state, err := s.loadState(ctx)
	if err != nil {
		return nil, err
	}
	if filter.Page < 1 {
		filter.Page = 1
	}
	if filter.PageSize < 1 {
		filter.PageSize = 50
	}
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}
	filtered := filterInspectionResults(state.Results, filter)
	pages := (len(filtered) + filter.PageSize - 1) / filter.PageSize
	if pages < 1 {
		pages = 1
	}
	start := (filter.Page - 1) * filter.PageSize
	items := []AccountInspectionAccountResult{}
	if start < len(filtered) {
		end := start + filter.PageSize
		if end > len(filtered) {
			end = len(filtered)
		}
		items = filtered[start:end]
	}
	return &AccountInspectionOverview{
		Settings: settings,
		Run:      inspectionRunView(*state),
		Results:  AccountInspectionPage{Items: items, Total: len(filtered), Page: filter.Page, PageSize: filter.PageSize, Pages: pages},
	}, nil
}

func filterInspectionResults(results []AccountInspectionAccountResult, filter AccountInspectionListFilter) []AccountInspectionAccountResult {
	search := strings.ToLower(strings.TrimSpace(filter.Search))
	out := make([]AccountInspectionAccountResult, 0, len(results))
	for _, result := range results {
		if filter.Type != "" && filter.Type != "all" && result.Type != filter.Type {
			continue
		}
		if filter.Status == "flagged" && len(result.Reasons) == 0 {
			continue
		}
		if filter.Status == "healthy" && len(result.Reasons) > 0 {
			continue
		}
		if filter.Status == "disabled" && result.Action != AccountInspectionActionDisabled && result.Action != AccountInspectionActionAlreadyDisabled {
			continue
		}
		if search != "" && !strings.Contains(strings.ToLower(result.Name), search) && !strings.Contains(strconv.FormatInt(result.AccountID, 10), search) {
			continue
		}
		out = append(out, result)
	}
	return out
}

func (s *AccountInspectionService) RunNow(ctx context.Context, trigger string) (*AccountInspectionRunState, error) {
	if s == nil || s.accountRepo == nil || s.usageService == nil {
		return nil, ErrAccountInspectionUnavailable
	}
	if !s.running.CompareAndSwap(false, true) {
		return nil, ErrAccountInspectionBusy
	}
	defer s.running.Store(false)
	release, acquired := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, accountInspectionLeaderLockKey, s.instanceID, accountInspectionLeaderLockTTL)
	if !acquired {
		return nil, ErrAccountInspectionBusy
	}
	defer release()
	return s.execute(ctx, trigger)
}

func (s *AccountInspectionService) execute(ctx context.Context, trigger string) (*AccountInspectionRunState, error) {
	settings, err := s.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	state := &AccountInspectionRunState{
		RunID: uuid.NewString(), Status: AccountInspectionStatusRunning, Trigger: trigger, StartedAt: &now,
		Summary: AccountInspectionSummary{QuotaUsageDistribution: newAccountInspectionQuotaDistribution()},
	}
	if err := s.saveState(ctx, state); err != nil {
		return nil, err
	}
	accounts, err := s.accountRepo.ListAllWithFilters(ctx, "", "", "", "", 0, "")
	if err != nil {
		return s.failState(ctx, state, err)
	}
	eligible := make([]Account, 0, len(accounts))
	for i := range accounts {
		account := accounts[i]
		if !account.IsActive() || !isAccountInspectionType(&account) {
			continue
		}
		eligible = append(eligible, account)
	}
	ids := make([]int64, 0, len(eligible))
	for i := range eligible {
		ids = append(ids, eligible[i].ID)
	}
	stats, err := s.usageService.GetAccountHourlyUsageStatsBatch(ctx, ids, now.Add(-time.Duration(settings.LookbackMinutes)*time.Minute), now)
	if err != nil {
		return s.failState(ctx, state, err)
	}
	results := make([]AccountInspectionAccountResult, 0, len(eligible))
	for i := range eligible {
		result := evaluateAccountInspection(&eligible[i], stats[eligible[i].ID], settings, now)
		results = append(results, result)
	}
	quotaDistribution := summarizeAccountInspectionQuotaDistribution(results)
	sort.SliceStable(results, func(i, j int) bool {
		leftFlagged, rightFlagged := len(results[i].Reasons) > 0, len(results[j].Reasons) > 0
		if leftFlagged != rightFlagged {
			return leftFlagged
		}
		return results[i].AccountID < results[j].AccountID
	})
	if len(results) > accountInspectionMaxStoredResults {
		results = results[:accountInspectionMaxStoredResults]
		state.ResultsTruncated = true
	}
	flaggedIDs := make([]int64, 0)
	for i := range results {
		if len(results[i].Reasons) > 0 && settings.AutoDisable && results[i].Schedulable {
			flaggedIDs = append(flaggedIDs, results[i].AccountID)
		}
	}
	if len(flaggedIDs) > 0 {
		falseValue := false
		if _, err := s.accountRepo.BulkUpdate(ctx, flaggedIDs, AccountBulkUpdate{Schedulable: &falseValue}); err != nil {
			return s.failState(ctx, state, err)
		}
		updated, getErr := s.accountRepo.GetByIDs(ctx, flaggedIDs)
		if getErr != nil {
			return s.failState(ctx, state, getErr)
		}
		updatedByID := make(map[int64]Account, len(updated))
		for _, account := range updated {
			if account != nil {
				updatedByID[account.ID] = *account
			}
		}
		selectedByID := make(map[int64]struct{}, len(flaggedIDs))
		for _, id := range flaggedIDs {
			selectedByID[id] = struct{}{}
		}
		for i := range results {
			if _, selected := selectedByID[results[i].AccountID]; !selected {
				continue
			}
			updatedAccount, found := updatedByID[results[i].AccountID]
			if !found || !updatedAccount.Schedulable {
				if !found {
					results[i].Action = AccountInspectionActionError
					results[i].Reasons = append(results[i].Reasons, "disable_update_not_confirmed")
					continue
				}
				results[i].Action = AccountInspectionActionDisabled
				results[i].Schedulable = false
			} else {
				results[i].Action = AccountInspectionActionError
				results[i].Reasons = append(results[i].Reasons, "disable_update_not_confirmed")
			}
		}
	}
	state.Results = results
	state.Summary = summarizeInspectionResults(results)
	state.Summary.Inspected = len(eligible)
	state.Summary.QuotaUsageDistribution = quotaDistribution
	completed := time.Now().UTC()
	state.Status = AccountInspectionStatusSucceeded
	state.CompletedAt = &completed
	next := completed.Add(time.Duration(settings.IntervalMinutes) * time.Minute)
	state.NextRunAt = &next
	if err := s.saveState(ctx, state); err != nil {
		return nil, err
	}
	return state, nil
}

func (s *AccountInspectionService) failState(ctx context.Context, state *AccountInspectionRunState, runErr error) (*AccountInspectionRunState, error) {
	now := time.Now().UTC()
	state.Status = AccountInspectionStatusFailed
	state.CompletedAt = &now
	state.Error = strings.TrimSpace(runErr.Error())
	if len(state.Error) > 500 {
		state.Error = state.Error[:500]
	}
	_ = s.saveState(ctx, state)
	return nil, runErr
}

func (s *AccountInspectionService) loadState(ctx context.Context) (*AccountInspectionRunState, error) {
	state := &AccountInspectionRunState{
		Status:  AccountInspectionStatusIdle,
		Summary: AccountInspectionSummary{QuotaUsageDistribution: newAccountInspectionQuotaDistribution()},
		Results: []AccountInspectionAccountResult{},
	}
	if s == nil || s.settingRepo == nil {
		return state, nil
	}
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyAccountInspectionState)
	if err != nil {
		if errors.Is(err, ErrSettingNotFound) {
			return state, nil
		}
		return nil, fmt.Errorf("get account inspection state: %w", err)
	}
	if strings.TrimSpace(raw) == "" {
		return state, nil
	}
	if err := json.Unmarshal([]byte(raw), state); err != nil {
		return nil, fmt.Errorf("parse account inspection state: %w", err)
	}
	if state.Results == nil {
		state.Results = []AccountInspectionAccountResult{}
	}
	if state.Summary.QuotaUsageDistribution.Buckets == nil {
		state.Summary.QuotaUsageDistribution = newAccountInspectionQuotaDistribution()
	}
	return state, nil
}

func (s *AccountInspectionService) saveState(ctx context.Context, state *AccountInspectionRunState) error {
	if s == nil || s.settingRepo == nil {
		return nil
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal account inspection state: %w", err)
	}
	return s.settingRepo.Set(ctx, SettingKeyAccountInspectionState, string(data))
}

func isAccountInspectionType(account *Account) bool {
	return account != nil && (account.Type == AccountTypeOAuth || account.IsAPIKeyOrBedrock())
}

func evaluateAccountInspection(account *Account, stats *usagestats.AccountHourlyUsageStats, settings AccountInspectionSettings, now time.Time) AccountInspectionAccountResult {
	result := AccountInspectionAccountResult{
		AccountID:   account.ID,
		Name:        account.Name,
		Platform:    account.Platform,
		Type:        account.Type,
		Status:      "healthy",
		Schedulable: account.Schedulable,
		Action:      AccountInspectionActionNone,
		ObservedAt:  now,
	}
	if usedPercent, dimension := accountInspectionQuotaUsage(account, now); usedPercent != nil {
		result.QuotaUsedPercent = usedPercent
		result.QuotaUsageDimension = dimension
	}
	if account.IsAPIKeyOrBedrock() {
		multiplier := account.BillingRateMultiplier()
		result.RateMultiplier = &multiplier
		remaining, dimension, unlimited := accountInspectionRemainingQuota(account)
		result.RemainingQuota = remaining
		result.RemainingQuotaDimension = dimension
		result.QuotaUnlimited = unlimited
	}
	if stats == nil {
		result.Status = "unknown"
		result.Reasons = []string{"metrics_unavailable"}
		result.Action = AccountInspectionActionReported
		return result
	}
	result.TotalRequests = stats.TotalRequests
	result.SuccessfulRequests = stats.SuccessfulRequests
	result.AvgFirstTokenMs = stats.AvgFirstTokenMs
	result.CacheHitRate = stats.CacheHitRate
	result.CacheReadTokens = stats.CacheReadTokens
	result.CacheCreationTokens = stats.CacheCreationTokens
	if stats.TotalRequests > 0 {
		rate := stats.SuccessRate
		result.SuccessRate = &rate
	}
	if stats.TotalRequests >= int64(settings.MinRequests) {
		if stats.AvgFirstTokenMs != nil && *stats.AvgFirstTokenMs > float64(settings.TTFTThresholdMs) {
			result.Reasons = append(result.Reasons, "first_token_over_threshold")
		}
		if stats.SuccessRate < settings.SuccessRateThreshold {
			result.Reasons = append(result.Reasons, "success_rate_below_threshold")
		}
	}
	if account.Type == AccountTypeOAuth && settings.OAuthQuotaCheckEnabled {
		if reason := accountInspectionOAuthQuotaReason(account, now); reason != "" {
			result.Reasons = append(result.Reasons, "oauth_quota_exhausted:"+reason)
		}
	}
	if account.IsAPIKeyOrBedrock() {
		if settings.APIKeyMinCacheHitRate > 0 && stats.CacheHitRate != nil && *stats.CacheHitRate < settings.APIKeyMinCacheHitRate {
			result.Reasons = append(result.Reasons, "cache_hit_rate_below_threshold")
		}
		if settings.APIKeyMaxRateMultiplier > 0 && result.RateMultiplier != nil && *result.RateMultiplier > settings.APIKeyMaxRateMultiplier {
			result.Reasons = append(result.Reasons, "rate_multiplier_over_threshold")
		}
		if settings.APIKeyQuotaCheckEnabled && result.RemainingQuota != nil && *result.RemainingQuota <= settings.APIKeyMinRemainingQuota {
			result.Reasons = append(result.Reasons, "remaining_quota_below_threshold")
		}
	}
	if len(result.Reasons) > 0 {
		result.Status = "flagged"
		if !account.Schedulable {
			result.Action = AccountInspectionActionAlreadyDisabled
		} else {
			result.Action = AccountInspectionActionReported
		}
	}
	return result
}

func summarizeInspectionResults(results []AccountInspectionAccountResult) AccountInspectionSummary {
	summary := AccountInspectionSummary{Inspected: len(results)}
	for _, result := range results {
		if result.Type == AccountTypeOAuth {
			summary.OAuthAccounts++
		} else {
			summary.APIKeyAccounts++
		}
		if len(result.Reasons) == 0 {
			summary.Healthy++
		} else {
			summary.Flagged++
		}
		switch result.Action {
		case AccountInspectionActionDisabled:
			summary.Disabled++
		case AccountInspectionActionAlreadyDisabled:
			summary.AlreadyDisabled++
		}
	}
	return summary
}

func summarizeAccountInspectionQuotaDistribution(results []AccountInspectionAccountResult) AccountInspectionQuotaDistribution {
	distribution := newAccountInspectionQuotaDistribution()
	var total float64
	for _, result := range results {
		if result.QuotaUsedPercent == nil || !isFiniteInspectionQuotaValue(*result.QuotaUsedPercent) {
			distribution.UnknownAccounts++
			continue
		}
		used := *result.QuotaUsedPercent
		if used < 0 {
			used = 0
		}
		distribution.MeasuredAccounts++
		total += used
		switch {
		case used < 20:
			distribution.Buckets[0].Count++
		case used < 40:
			distribution.Buckets[1].Count++
		case used < 70:
			distribution.Buckets[2].Count++
		case used < 90:
			distribution.Buckets[3].Count++
		case used <= 100:
			distribution.Buckets[4].Count++
		default:
			distribution.Buckets[5].Count++
		}
	}
	if distribution.MeasuredAccounts > 0 {
		average := total / float64(distribution.MeasuredAccounts)
		distribution.AverageUsedPercent = &average
	}
	return distribution
}

func newAccountInspectionQuotaDistribution() AccountInspectionQuotaDistribution {
	max20 := 20.0
	max40 := 40.0
	max70 := 70.0
	max90 := 90.0
	max100 := 100.0
	return AccountInspectionQuotaDistribution{Buckets: []AccountInspectionQuotaBucket{
		{Key: "0_20", MinPercent: 0, MaxPercent: &max20},
		{Key: "20_40", MinPercent: 20, MaxPercent: &max40},
		{Key: "40_70", MinPercent: 40, MaxPercent: &max70},
		{Key: "70_90", MinPercent: 70, MaxPercent: &max90},
		{Key: "90_100", MinPercent: 90, MaxPercent: &max100},
		{Key: "over_100", MinPercent: 100},
	}}
}

func accountInspectionQuotaUsage(account *Account, now time.Time) (*float64, string) {
	if account == nil {
		return nil, ""
	}
	if account.Type == AccountTypeOAuth {
		return accountInspectionOAuthQuotaUsage(account, now)
	}
	if account.IsAPIKeyOrBedrock() {
		return accountInspectionAPIKeyQuotaUsage(account)
	}
	return nil, ""
}

func accountInspectionAPIKeyQuotaUsage(account *Account) (*float64, string) {
	windows := []accountInspectionQuotaWindow{}
	if limit := account.GetQuotaLimit(); limit > 0 {
		windows = append(windows, newAccountInspectionQuotaUsage("total", account.GetQuotaUsed(), limit))
	}
	if limit := account.GetQuotaDailyLimit(); limit > 0 && !account.IsDailyQuotaPeriodExpired() {
		windows = append(windows, newAccountInspectionQuotaUsage("daily", account.GetQuotaDailyUsed(), limit))
	}
	if limit := account.GetQuotaWeeklyLimit(); limit > 0 && !account.IsWeeklyQuotaPeriodExpired() {
		windows = append(windows, newAccountInspectionQuotaUsage("weekly", account.GetQuotaWeeklyUsed(), limit))
	}
	return highestInspectionQuotaUsage(windows)
}

type accountInspectionQuotaWindow struct {
	dimension string
	percent   float64
}

func newAccountInspectionQuotaUsage(dimension string, used, limit float64) accountInspectionQuotaWindow {
	if !isFiniteInspectionQuotaValue(used) || !isFiniteInspectionQuotaValue(limit) || limit <= 0 {
		return accountInspectionQuotaWindow{dimension: dimension, percent: math.NaN()}
	}
	percent := used / limit * 100
	if percent < 0 {
		percent = 0
	}
	return accountInspectionQuotaWindow{dimension: dimension, percent: percent}
}

func highestInspectionQuotaUsage(windows []accountInspectionQuotaWindow) (*float64, string) {
	var highest *float64
	dimension := ""
	for _, window := range windows {
		if !isFiniteInspectionQuotaValue(window.percent) {
			continue
		}
		if highest == nil || window.percent > *highest {
			value := window.percent
			highest = &value
			dimension = window.dimension
		}
	}
	return highest, dimension
}

func accountInspectionOAuthQuotaUsage(account *Account, now time.Time) (*float64, string) {
	return highestInspectionQuotaUsage(accountInspectionOAuthQuotaWindows(account, now))
}

func accountInspectionOAuthQuotaWindows(account *Account, now time.Time) []accountInspectionQuotaWindow {
	if account == nil || account.Extra == nil {
		return nil
	}
	extra := account.Extra
	windows := make([]accountInspectionQuotaWindow, 0, 8)
	percentWindows := []struct{ usage, reset, name string }{
		{"codex_5h_used_percent", "codex_5h_reset_at", "5h"},
		{"codex_7d_used_percent", "codex_7d_reset_at", "7d"},
		{"codex_primary_used_percent", "codex_primary_reset_at", "primary"},
		{"codex_secondary_used_percent", "codex_secondary_reset_at", "secondary"},
	}
	for _, window := range percentWindows {
		usage, ok := resolveAccountExtraNumber(extra, window.usage)
		if !ok || !inspectionResetActive(extra[window.reset], false, now) {
			continue
		}
		if usage < 0 {
			usage = 0
		}
		windows = append(windows, accountInspectionQuotaWindow{dimension: window.name, percent: usage})
	}
	ratioWindows := []struct {
		usage, reset, name string
		unix               bool
	}{
		{"session_window_utilization", "", "session", false},
		{"passive_usage_7d_utilization", "passive_usage_7d_reset", "passive_7d", true},
		{"passive_usage_7d_oi_utilization", "passive_usage_7d_oi_reset", "passive_7d_oi", true},
	}
	for _, window := range ratioWindows {
		usage, ok := resolveAccountExtraNumber(extra, window.usage)
		if !ok {
			continue
		}
		reset := any(nil)
		if window.reset == "" {
			if account.SessionWindowEnd != nil {
				reset = account.SessionWindowEnd.Format(time.RFC3339Nano)
			}
		} else {
			reset = extra[window.reset]
		}
		if !inspectionResetActive(reset, window.unix, now) {
			continue
		}
		if usage < 0 {
			usage = 0
		}
		windows = append(windows, accountInspectionQuotaWindow{dimension: window.name, percent: usage * 100})
	}
	if billing, ok := extra["grok_billing_snapshot"].(map[string]any); ok &&
		inspectionResetActive(billing["period_end"], false, now) &&
		inspectionResetActive(billing["billing_period_end"], false, now) {
		for _, key := range []string{"usage_percent", "used_percent"} {
			if usage, ok := resolveAccountExtraNumber(billing, key); ok {
				if usage < 0 {
					usage = 0
				}
				windows = append(windows, accountInspectionQuotaWindow{dimension: "grok", percent: usage})
			}
		}
	}
	return windows
}

func accountInspectionRemainingQuota(account *Account) (remaining *float64, dimension string, unlimited bool) {
	type quotaWindow struct {
		name  string
		limit float64
		used  float64
		valid bool
	}
	windows := []quotaWindow{
		{name: "total", limit: account.GetQuotaLimit(), used: account.GetQuotaUsed(), valid: account.GetQuotaLimit() > 0},
	}
	if limit := account.GetQuotaDailyLimit(); limit > 0 && !account.IsDailyQuotaPeriodExpired() {
		windows = append(windows, quotaWindow{name: "daily", limit: limit, used: account.GetQuotaDailyUsed(), valid: true})
	}
	if limit := account.GetQuotaWeeklyLimit(); limit > 0 && !account.IsWeeklyQuotaPeriodExpired() {
		windows = append(windows, quotaWindow{name: "weekly", limit: limit, used: account.GetQuotaWeeklyUsed(), valid: true})
	}
	for _, window := range windows {
		if !window.valid {
			continue
		}
		value := window.limit - window.used
		if value < 0 {
			value = 0
		}
		if remaining == nil || value < *remaining {
			v := value
			remaining = &v
			dimension = window.name
		}
	}
	return remaining, dimension, remaining == nil
}

func accountInspectionOAuthQuotaReason(account *Account, now time.Time) string {
	for _, window := range accountInspectionOAuthQuotaWindows(account, now) {
		if isFiniteInspectionQuotaValue(window.percent) && window.percent >= 100 {
			return window.dimension
		}
	}
	return ""
}

func isFiniteInspectionQuotaValue(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func inspectionResetActive(value any, unixSeconds bool, now time.Time) bool {
	if value == nil || fmt.Sprint(value) == "" {
		return true
	}
	if unixSeconds {
		seconds, ok := resolveAccountExtraNumber(map[string]any{"value": value}, "value")
		return !ok || now.Before(time.Unix(int64(seconds), 0))
	}
	timestamp, err := time.Parse(time.RFC3339Nano, fmt.Sprint(value))
	if err != nil {
		timestamp, err = time.Parse(time.RFC3339, fmt.Sprint(value))
	}
	if err != nil {
		return true
	}
	return now.Before(timestamp)
}

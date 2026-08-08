package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"golang.org/x/sync/singleflight"
)

const (
	CPAModeCredentialKey                       = "cpa_mode"
	CPAManagementURLCredentialKey              = "cpa_management_url"
	CPAManagementKeyCredentialKey              = "cpa_management_key"
	CPAConcurrencyPerCredentialCredentialKey   = "cpa_concurrency_per_credential"
	CPAExcludeAbnormalCredentialsCredentialKey = "cpa_exclude_abnormal_credentials"

	DefaultCPAConcurrencyPerCredential = 10
	maxCPAConcurrencyPerCredential     = 10000
	defaultCPASnapshotTTL              = 90 * time.Second
	defaultCPAStaleSnapshotTTL         = 180 * time.Second
	defaultCPARequestTimeout           = 2 * time.Second
	maxCPAAuthFilesResponseBytes       = 2 << 20
)

var errCPAPoolCapacityUnavailable = errors.New("CPA pool capacity is unavailable")

type cpaPoolConfig struct {
	managementURL              string
	managementKey              string
	concurrencyPerCredential   int
	excludeAbnormalCredentials bool
}

const (
	CPACapacityStateFresh       = "fresh"
	CPACapacityStateStale       = "stale"
	CPACapacityStateUnavailable = "unavailable"
)

// CPACapacityStatus is safe to expose through admin APIs. It never includes
// the management URL or administrator password.
type CPACapacityStatus struct {
	TotalCredentials           int       `json:"total_credentials"`
	EnabledCredentials         int       `json:"enabled_credentials"`
	AbnormalCredentials        int       `json:"abnormal_credentials"`
	AvailableCredentials       int       `json:"available_credentials"`
	CapacityCredentials        int       `json:"capacity_credentials"`
	EffectiveConcurrency       int       `json:"effective_concurrency"`
	ConcurrencyPerCredential   int       `json:"concurrency_per_credential"`
	ExcludeAbnormalCredentials bool      `json:"exclude_abnormal_credentials"`
	FetchedAt                  time.Time `json:"fetched_at"`
	State                      string    `json:"state"`
}

type CPATestInput struct {
	UseAccountBaseURL          bool
	BaseURL                    string
	ManagementURL              string
	ManagementPassword         string
	ConcurrencyPerCredential   *int
	ExcludeAbnormalCredentials *bool
}

type CPATestResult struct {
	*CPACapacityStatus
	LatencyMS int64 `json:"latency_ms"`
}

func cpaBool(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(typed))
		return parsed, err == nil
	default:
		return false, false
	}
}

func cpaPositiveInt(value any) (int, bool) {
	var number int64
	switch typed := value.(type) {
	case int:
		number = int64(typed)
	case int32:
		number = int64(typed)
	case int64:
		number = typed
	case float64:
		if math.Trunc(typed) != typed || typed > math.MaxInt64 || typed < math.MinInt64 {
			return 0, false
		}
		number = int64(typed)
	case json.Number:
		parsed, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		number = parsed
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 32)
		if err != nil {
			return 0, false
		}
		number = parsed
	default:
		return 0, false
	}
	if number <= 0 || number > maxCPAConcurrencyPerCredential {
		return 0, false
	}
	return int(number), true
}

func normalizeCPAManagementURL(raw string) (string, error) {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("cpa_management_url must be an absolute HTTP(S) URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("cpa_management_url must use HTTP or HTTPS")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("cpa_management_url must not contain credentials, query parameters, or fragments")
	}
	return raw, nil
}

// NormalizeCPACredentials validates and canonicalizes the optional read-only
// CLIProxyAPI capacity integration stored in an API-key account's credentials.
func NormalizeCPACredentials(accountType string, credentials map[string]any) error {
	if credentials == nil {
		return nil
	}
	rawEnabled, exists := credentials[CPAModeCredentialKey]
	if !exists {
		delete(credentials, CPAExcludeAbnormalCredentialsCredentialKey)
		return nil
	}
	enabled, ok := cpaBool(rawEnabled)
	if !ok {
		return infraerrors.BadRequest("INVALID_CPA_MODE", "cpa_mode must be a boolean")
	}
	if !enabled {
		delete(credentials, CPAModeCredentialKey)
		delete(credentials, CPAManagementURLCredentialKey)
		delete(credentials, CPAManagementKeyCredentialKey)
		delete(credentials, CPAConcurrencyPerCredentialCredentialKey)
		delete(credentials, CPAExcludeAbnormalCredentialsCredentialKey)
		return nil
	}
	if accountType != AccountTypeAPIKey {
		return infraerrors.BadRequest("CPA_MODE_REQUIRES_API_KEY_ACCOUNT", "CPA mode is only supported for API-key accounts")
	}
	if raw, provided := credentials[CPAExcludeAbnormalCredentialsCredentialKey]; provided {
		excludeAbnormal, valid := cpaBool(raw)
		if !valid {
			return infraerrors.BadRequest(
				"INVALID_CPA_EXCLUDE_ABNORMAL_CREDENTIALS",
				"cpa_exclude_abnormal_credentials must be a boolean",
			)
		}
		credentials[CPAExcludeAbnormalCredentialsCredentialKey] = excludeAbnormal
	}

	managementURL := ""
	if rawManagementURL, provided := credentials[CPAManagementURLCredentialKey]; provided {
		if rawManagementURL != nil {
			var ok bool
			managementURL, ok = rawManagementURL.(string)
			if !ok {
				return infraerrors.BadRequest("INVALID_CPA_MANAGEMENT_URL", "cpa_management_url must be a string")
			}
		}
	}
	managementURL = strings.TrimSpace(managementURL)
	if managementURL == "" {
		delete(credentials, CPAManagementURLCredentialKey)
		baseURL, _ := credentials["base_url"].(string)
		managementURL = strings.TrimSpace(baseURL)
	}
	if managementURL == "" {
		return infraerrors.BadRequest("CPA_MANAGEMENT_URL_REQUIRED", "account base_url or cpa_management_url is required when CPA mode is enabled")
	}
	normalizedURL, err := normalizeCPAManagementURL(managementURL)
	if err != nil {
		return infraerrors.BadRequest("INVALID_CPA_MANAGEMENT_URL", err.Error())
	}
	managementKey, ok := credentials[CPAManagementKeyCredentialKey].(string)
	if !ok || strings.TrimSpace(managementKey) == "" {
		return infraerrors.BadRequest("CPA_MANAGEMENT_KEY_REQUIRED", "cpa_management_key is required when CPA mode is enabled")
	}

	perCredential := DefaultCPAConcurrencyPerCredential
	if raw, provided := credentials[CPAConcurrencyPerCredentialCredentialKey]; provided {
		parsed, valid := cpaPositiveInt(raw)
		if !valid {
			return infraerrors.BadRequest(
				"INVALID_CPA_CONCURRENCY_PER_CREDENTIAL",
				fmt.Sprintf("cpa_concurrency_per_credential must be between 1 and %d", maxCPAConcurrencyPerCredential),
			)
		}
		perCredential = parsed
	}

	credentials[CPAModeCredentialKey] = true
	if _, overridden := credentials[CPAManagementURLCredentialKey]; overridden {
		credentials[CPAManagementURLCredentialKey] = normalizedURL
	}
	credentials[CPAManagementKeyCredentialKey] = strings.TrimSpace(managementKey)
	credentials[CPAConcurrencyPerCredentialCredentialKey] = perCredential
	return nil
}

func cpaModeEnabled(account *Account) bool {
	if account == nil || account.Type != AccountTypeAPIKey || account.Credentials == nil {
		return false
	}
	enabled, ok := cpaBool(account.Credentials[CPAModeCredentialKey])
	return ok && enabled
}

// IsCPAModeEnabled reports whether an account has the optional CPA integration enabled.
func IsCPAModeEnabled(account *Account) bool {
	return cpaModeEnabled(account)
}

func cpaPoolConfigFromAccount(account *Account) (cpaPoolConfig, bool) {
	if !cpaModeEnabled(account) {
		return cpaPoolConfig{}, false
	}
	managementURL, _ := account.Credentials[CPAManagementURLCredentialKey].(string)
	if strings.TrimSpace(managementURL) == "" {
		managementURL, _ = account.Credentials["base_url"].(string)
	}
	managementKey, okKey := account.Credentials[CPAManagementKeyCredentialKey].(string)
	managementURL = strings.TrimSpace(managementURL)
	managementKey = strings.TrimSpace(managementKey)
	if !okKey || managementURL == "" || managementKey == "" {
		return cpaPoolConfig{}, true
	}
	perCredential := DefaultCPAConcurrencyPerCredential
	if parsed, ok := cpaPositiveInt(account.Credentials[CPAConcurrencyPerCredentialCredentialKey]); ok {
		perCredential = parsed
	}
	excludeAbnormalCredentials := false
	if parsed, ok := cpaBool(account.Credentials[CPAExcludeAbnormalCredentialsCredentialKey]); ok {
		excludeAbnormalCredentials = parsed
	}
	return cpaPoolConfig{
		managementURL:              managementURL,
		managementKey:              managementKey,
		concurrencyPerCredential:   perCredential,
		excludeAbnormalCredentials: excludeAbnormalCredentials,
	}, true
}

func cpaAuthFilesURL(managementURL string) string {
	base := strings.TrimRight(strings.TrimSpace(managementURL), "/")
	switch {
	case strings.HasSuffix(base, "/v0/management/auth-files"):
		return base
	case strings.HasSuffix(base, "/v0/management"):
		return base + "/auth-files"
	case strings.HasSuffix(base, "/v1"):
		return strings.TrimSuffix(base, "/v1") + "/v0/management/auth-files"
	default:
		return base + "/v0/management/auth-files"
	}
}

type cpaAuthFile struct {
	Status         string     `json:"status"`
	Disabled       bool       `json:"disabled"`
	Unavailable    bool       `json:"unavailable"`
	NextRetryAfter *time.Time `json:"next_retry_after"`
}

type cpaAuthFilesResponse struct {
	Files []cpaAuthFile `json:"files"`
}

type cpaCapacitySnapshot struct {
	totalCredentials     int
	enabledCredentials   int
	abnormalCredentials  int
	availableCredentials int
	fetchedAt            time.Time
}

type cpaCapacityCacheKey struct {
	managementURL     string
	managementKeyHash [sha256.Size]byte
}

func (k cpaCapacityCacheKey) singleflightKey() string {
	return k.managementURL + "\x00" + string(k.managementKeyHash[:])
}

type cpaCapacityCacheEntry struct {
	snapshot      *cpaCapacitySnapshot
	lastAttemptAt time.Time
	lastErr       error
}

type cpaPoolCapacityService struct {
	client   *http.Client
	now      func() time.Time
	cacheTTL time.Duration
	staleTTL time.Duration
	cache    sync.Map
	group    singleflight.Group
}

func newCPAPoolCapacityService() *cpaPoolCapacityService {
	return &cpaPoolCapacityService{
		client: &http.Client{
			Timeout: defaultCPARequestTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		now:      time.Now,
		cacheTTL: defaultCPASnapshotTTL,
		staleTTL: defaultCPAStaleSnapshotTTL,
	}
}

func (s *cpaPoolCapacityService) cacheKey(config cpaPoolConfig) cpaCapacityCacheKey {
	return cpaCapacityCacheKey{
		managementURL:     config.managementURL,
		managementKeyHash: sha256.Sum256([]byte(config.managementKey)),
	}
}

func (s *cpaPoolCapacityService) cachedSnapshot(key cpaCapacityCacheKey, now time.Time) (*cpaCapacitySnapshot, string, error, bool) {
	raw, ok := s.cache.Load(key)
	if !ok {
		return nil, "", nil, false
	}
	entry, ok := raw.(cpaCapacityCacheEntry)
	if !ok {
		return nil, "", nil, false
	}
	if entry.lastErr != nil && now.Sub(entry.lastAttemptAt) < s.cacheTTL {
		if entry.snapshot != nil && now.Sub(entry.snapshot.fetchedAt) <= s.staleTTL {
			return entry.snapshot, CPACapacityStateStale, nil, true
		}
		return nil, CPACapacityStateUnavailable, entry.lastErr, true
	}
	if entry.snapshot != nil && now.Sub(entry.snapshot.fetchedAt) < s.cacheTTL {
		return entry.snapshot, CPACapacityStateFresh, nil, true
	}
	return nil, "", nil, false
}

func (s *cpaPoolCapacityService) snapshot(ctx context.Context, config cpaPoolConfig) (*cpaCapacitySnapshot, string, error) {
	if s == nil {
		return nil, CPACapacityStateUnavailable, errCPAPoolCapacityUnavailable
	}
	now := s.now()
	key := s.cacheKey(config)
	if snapshot, state, err, ok := s.cachedSnapshot(key, now); ok {
		return snapshot, state, err
	}

	value, err, _ := s.group.Do(key.singleflightKey(), func() (any, error) {
		checkNow := s.now()
		if snapshot, _, cachedErr, ok := s.cachedSnapshot(key, checkNow); ok {
			return snapshot, cachedErr
		}
		previousRaw, _ := s.cache.Load(key)
		previous, _ := previousRaw.(cpaCapacityCacheEntry)
		snapshot, fetchErr := s.fetch(ctx, config, checkNow)
		if fetchErr != nil {
			s.cache.Store(key, cpaCapacityCacheEntry{
				snapshot:      previous.snapshot,
				lastAttemptAt: checkNow,
				lastErr:       fetchErr,
			})
			if previous.snapshot != nil && checkNow.Sub(previous.snapshot.fetchedAt) <= s.staleTTL {
				return previous.snapshot, nil
			}
			return nil, fetchErr
		}
		s.cache.Store(key, cpaCapacityCacheEntry{snapshot: snapshot, lastAttemptAt: checkNow})
		return snapshot, nil
	})
	if err != nil {
		return nil, CPACapacityStateUnavailable, err
	}
	snapshot, ok := value.(*cpaCapacitySnapshot)
	if !ok || snapshot == nil {
		return nil, CPACapacityStateUnavailable, errCPAPoolCapacityUnavailable
	}
	if _, state, _, ok := s.cachedSnapshot(key, s.now()); ok {
		return snapshot, state, nil
	}
	return snapshot, CPACapacityStateFresh, nil
}

func (s *cpaPoolCapacityService) forceSnapshot(ctx context.Context, config cpaPoolConfig) (*cpaCapacitySnapshot, error) {
	if s == nil {
		return nil, errCPAPoolCapacityUnavailable
	}
	key := s.cacheKey(config)
	value, err, _ := s.group.Do(key.singleflightKey()+"\x00force", func() (any, error) {
		now := s.now()
		previousRaw, _ := s.cache.Load(key)
		previous, _ := previousRaw.(cpaCapacityCacheEntry)
		snapshot, fetchErr := s.fetch(ctx, config, now)
		if fetchErr != nil {
			s.cache.Store(key, cpaCapacityCacheEntry{
				snapshot:      previous.snapshot,
				lastAttemptAt: now,
				lastErr:       fetchErr,
			})
			return nil, fetchErr
		}
		s.cache.Store(key, cpaCapacityCacheEntry{snapshot: snapshot, lastAttemptAt: now})
		return snapshot, nil
	})
	if err != nil {
		return nil, err
	}
	snapshot, ok := value.(*cpaCapacitySnapshot)
	if !ok || snapshot == nil {
		return nil, errCPAPoolCapacityUnavailable
	}
	return snapshot, nil
}

func (s *cpaPoolCapacityService) fetch(ctx context.Context, config cpaPoolConfig, now time.Time) (*cpaCapacitySnapshot, error) {
	baseCtx := context.Background()
	if ctx != nil {
		baseCtx = context.WithoutCancel(ctx)
	}
	requestCtx, cancel := context.WithTimeout(baseCtx, defaultCPARequestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, cpaAuthFilesURL(config.managementURL), nil)
	if err != nil {
		return nil, fmt.Errorf("build CPA auth-files request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+config.managementKey)
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("query CPA auth-files: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("query CPA auth-files: unexpected HTTP status %d", resp.StatusCode)
	}
	var payload cpaAuthFilesResponse
	decoder := json.NewDecoder(io.LimitReader(resp.Body, maxCPAAuthFilesResponseBytes))
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode CPA auth-files response: %w", err)
	}

	enabled := 0
	abnormal := 0
	for _, file := range payload.Files {
		if file.Disabled || strings.EqualFold(strings.TrimSpace(file.Status), "disabled") {
			continue
		}
		enabled++
		if file.Unavailable || strings.EqualFold(strings.TrimSpace(file.Status), "error") {
			abnormal++
		}
	}
	available := enabled - abnormal
	if available < 0 {
		available = 0
	}
	return &cpaCapacitySnapshot{
		totalCredentials:     len(payload.Files),
		enabledCredentials:   enabled,
		abnormalCredentials:  abnormal,
		availableCredentials: available,
		fetchedAt:            now,
	}, nil
}

func cpaCapacityCredentialCount(snapshot *cpaCapacitySnapshot, config cpaPoolConfig) int {
	if snapshot == nil {
		return 0
	}
	if config.excludeAbnormalCredentials {
		return snapshot.availableCredentials
	}
	return snapshot.enabledCredentials
}

func cpaCapacityFromSnapshot(snapshot *cpaCapacitySnapshot, config cpaPoolConfig, state string) *CPACapacityStatus {
	status := &CPACapacityStatus{
		ConcurrencyPerCredential:   config.concurrencyPerCredential,
		ExcludeAbnormalCredentials: config.excludeAbnormalCredentials,
		State:                      state,
	}
	if snapshot == nil {
		return status
	}
	capacityCredentials := cpaCapacityCredentialCount(snapshot, config)
	capacity64 := int64(capacityCredentials) * int64(config.concurrencyPerCredential)
	maxInt := int64(^uint(0) >> 1)
	if capacity64 > maxInt {
		capacity64 = maxInt
	}
	status.TotalCredentials = snapshot.totalCredentials
	status.EnabledCredentials = snapshot.enabledCredentials
	status.AbnormalCredentials = snapshot.abnormalCredentials
	status.AvailableCredentials = snapshot.availableCredentials
	status.CapacityCredentials = capacityCredentials
	status.EffectiveConcurrency = int(capacity64)
	status.FetchedAt = snapshot.fetchedAt
	return status
}

func (s *cpaPoolCapacityService) capacityStatus(ctx context.Context, account *Account) (*CPACapacityStatus, error) {
	config, enabled := cpaPoolConfigFromAccount(account)
	if !enabled {
		return nil, nil
	}
	if config.managementURL == "" || config.managementKey == "" {
		return cpaCapacityFromSnapshot(nil, config, CPACapacityStateUnavailable), errCPAPoolCapacityUnavailable
	}
	snapshot, state, err := s.snapshot(ctx, config)
	if err != nil {
		return cpaCapacityFromSnapshot(nil, config, CPACapacityStateUnavailable), err
	}
	return cpaCapacityFromSnapshot(snapshot, config, state), nil
}

func (s *cpaPoolCapacityService) effectiveConcurrency(ctx context.Context, account *Account) (int, bool) {
	config, enabled := cpaPoolConfigFromAccount(account)
	if !enabled {
		return account.Concurrency, true
	}
	if config.managementURL == "" || config.managementKey == "" {
		return 0, false
	}
	snapshot, _, err := s.snapshot(ctx, config)
	if err != nil {
		slog.Warn("cpa_pool_capacity_unavailable", "account_id", account.ID, "management_url", config.managementURL, "error", err)
		return 0, false
	}
	capacityCredentials := cpaCapacityCredentialCount(snapshot, config)
	if capacityCredentials <= 0 {
		return 0, false
	}
	capacity64 := int64(capacityCredentials) * int64(config.concurrencyPerCredential)
	maxInt := int64(^uint(0) >> 1)
	if capacity64 > maxInt {
		capacity64 = maxInt
	}
	capacity := int(capacity64)
	return capacity, capacity > 0
}

func (s *ConcurrencyService) GetCPACapacityStatus(ctx context.Context, account *Account) (*CPACapacityStatus, error) {
	if s == nil || s.cpaPoolCapacity == nil {
		return nil, errCPAPoolCapacityUnavailable
	}
	return s.cpaPoolCapacity.capacityStatus(ctx, account)
}

func (s *ConcurrencyService) ForceRefreshCPACapacity(ctx context.Context, account *Account) (*CPACapacityStatus, error) {
	if s == nil || s.cpaPoolCapacity == nil {
		return nil, errCPAPoolCapacityUnavailable
	}
	config, enabled := cpaPoolConfigFromAccount(account)
	if !enabled || config.managementURL == "" || config.managementKey == "" {
		return nil, infraerrors.BadRequest("CPA_MODE_NOT_CONFIGURED", "CPA mode is not enabled or is incomplete")
	}
	snapshot, err := s.cpaPoolCapacity.forceSnapshot(ctx, config)
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CPA_CONNECTION_FAILED", "CPA connection failed: %v", err)
	}
	return cpaCapacityFromSnapshot(snapshot, config, CPACapacityStateFresh), nil
}

func (s *ConcurrencyService) TestCPACapacity(ctx context.Context, account *Account, input CPATestInput) (*CPATestResult, error) {
	if s == nil || s.cpaPoolCapacity == nil || account == nil {
		return nil, errCPAPoolCapacityUnavailable
	}
	credentials := make(map[string]any, len(account.Credentials)+4)
	for key, value := range account.Credentials {
		credentials[key] = value
	}
	credentials[CPAModeCredentialKey] = true
	if input.UseAccountBaseURL {
		if strings.TrimSpace(input.BaseURL) != "" {
			credentials["base_url"] = strings.TrimSpace(input.BaseURL)
		}
		delete(credentials, CPAManagementURLCredentialKey)
	} else if strings.TrimSpace(input.ManagementURL) != "" {
		credentials[CPAManagementURLCredentialKey] = input.ManagementURL
	}
	if strings.TrimSpace(input.ManagementPassword) != "" {
		credentials[CPAManagementKeyCredentialKey] = input.ManagementPassword
	}
	if input.ConcurrencyPerCredential != nil {
		credentials[CPAConcurrencyPerCredentialCredentialKey] = *input.ConcurrencyPerCredential
	}
	if input.ExcludeAbnormalCredentials != nil {
		credentials[CPAExcludeAbnormalCredentialsCredentialKey] = *input.ExcludeAbnormalCredentials
	}
	if err := NormalizeCPACredentials(account.Type, credentials); err != nil {
		return nil, err
	}
	testAccount := *account
	testAccount.Credentials = credentials
	config, enabled := cpaPoolConfigFromAccount(&testAccount)
	if !enabled || config.managementURL == "" || config.managementKey == "" {
		return nil, infraerrors.BadRequest("CPA_MODE_NOT_CONFIGURED", "CPA mode is not configured")
	}
	startedAt := time.Now()
	snapshot, err := s.cpaPoolCapacity.fetch(ctx, config, s.cpaPoolCapacity.now())
	latency := time.Since(startedAt).Milliseconds()
	if err != nil {
		return nil, infraerrors.Newf(http.StatusBadGateway, "CPA_CONNECTION_FAILED", "CPA connection failed: %v", err)
	}
	return &CPATestResult{
		CPACapacityStatus: cpaCapacityFromSnapshot(snapshot, config, CPACapacityStateFresh),
		LatencyMS:         latency,
	}, nil
}

func (s *ConcurrencyService) applyCPAPoolCapacity(ctx context.Context, account *Account) (*Account, bool) {
	if account == nil {
		return nil, false
	}
	if s == nil || s.cpaPoolCapacity == nil {
		return account, true
	}
	effective, available := s.cpaPoolCapacity.effectiveConcurrency(ctx, account)
	if !available {
		return nil, false
	}
	if effective == account.Concurrency {
		return account, true
	}
	copy := *account
	copy.Concurrency = effective
	return &copy, true
}

func (s *ConcurrencyService) applyCPAPoolCapacityBatch(ctx context.Context, accounts []Account) []Account {
	if len(accounts) == 0 || s == nil || s.cpaPoolCapacity == nil {
		return accounts
	}
	var filtered []Account
	for index := range accounts {
		if !cpaModeEnabled(&accounts[index]) {
			if filtered != nil {
				filtered = append(filtered, accounts[index])
			}
			continue
		}
		account, available := s.applyCPAPoolCapacity(ctx, &accounts[index])
		unchanged := available && account == &accounts[index]
		if filtered == nil && unchanged {
			continue
		}
		if filtered == nil {
			filtered = make([]Account, 0, len(accounts))
			filtered = append(filtered, accounts[:index]...)
		}
		if available && account != nil {
			filtered = append(filtered, *account)
		}
	}
	if filtered == nil {
		return accounts
	}
	return filtered
}

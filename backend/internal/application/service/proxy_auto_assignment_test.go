package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type proxyAutoAssignmentSettingRepo struct {
	values map[string]string
}

func (r *proxyAutoAssignmentSettingRepo) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}

func (r *proxyAutoAssignmentSettingRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (r *proxyAutoAssignmentSettingRepo) Set(_ context.Context, key, value string) error {
	if r.values == nil {
		r.values = make(map[string]string)
	}
	r.values[key] = value
	return nil
}

func (r *proxyAutoAssignmentSettingRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}

func (r *proxyAutoAssignmentSettingRepo) SetMultiple(ctx context.Context, values map[string]string) error {
	for key, value := range values {
		if err := r.Set(ctx, key, value); err != nil {
			return err
		}
	}
	return nil
}

func (r *proxyAutoAssignmentSettingRepo) GetAll(context.Context) (map[string]string, error) {
	result := make(map[string]string, len(r.values))
	for key, value := range r.values {
		result[key] = value
	}
	return result, nil
}

func (r *proxyAutoAssignmentSettingRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func TestProxyAutoAssignmentSettingsDefaultsAndValidation(t *testing.T) {
	repo := &proxyAutoAssignmentSettingRepo{}
	settingsService := NewSettingService(repo, &config.Config{})

	defaults, err := settingsService.GetProxyAutoAssignmentSettings(context.Background())
	require.NoError(t, err)
	require.False(t, defaults.Enabled)
	require.False(t, defaults.HealthCheckEnabled)
	require.Equal(t, defaultProxyHealthCheckIntervalMinutes, defaults.HealthCheckIntervalMinutes)
	require.Equal(t, defaultProxyHealthFailureThreshold, defaults.FailureThreshold)

	wanted := &ProxyAutoAssignmentSettings{
		Enabled:                    true,
		HealthCheckEnabled:         true,
		HealthCheckIntervalMinutes: 7,
		FailureThreshold:           2,
	}
	require.NoError(t, settingsService.SetProxyAutoAssignmentSettings(context.Background(), wanted))
	loaded, err := settingsService.GetProxyAutoAssignmentSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, wanted, loaded)

	invalid := *wanted
	invalid.HealthCheckIntervalMinutes = 0
	invalid.FailureThreshold = maxProxyHealthFailureThreshold + 1
	require.Error(t, settingsService.SetProxyAutoAssignmentSettings(context.Background(), &invalid))
}

type proxyAutoAssignmentAdminRepo struct {
	ProxyRepository
	selectedID   int64
	selectErr    error
	rebalanceErr error
	rebalances   int
}

func (r *proxyAutoAssignmentAdminRepo) SelectLeastLoadedActiveProxy(context.Context, time.Time) (*int64, error) {
	if r.selectErr != nil {
		return nil, r.selectErr
	}
	id := r.selectedID
	return &id, nil
}

func (r *proxyAutoAssignmentAdminRepo) RebalanceActiveProxyAccounts(context.Context, time.Time) (int64, error) {
	r.rebalances++
	if r.rebalanceErr != nil {
		return 0, r.rebalanceErr
	}
	return 4, nil
}

func TestUpdateProxyAutoAssignmentSettingsEnablesAndRebalances(t *testing.T) {
	settingRepo := &proxyAutoAssignmentSettingRepo{}
	settingsService := NewSettingService(settingRepo, &config.Config{})
	proxyRepo := &proxyAutoAssignmentAdminRepo{selectedID: 42}
	svc := &adminServiceImpl{settingService: settingsService, proxyRepo: proxyRepo}
	wanted := &ProxyAutoAssignmentSettings{
		Enabled:                    true,
		HealthCheckEnabled:         true,
		HealthCheckIntervalMinutes: 5,
		FailureThreshold:           2,
	}

	got, err := svc.UpdateProxyAutoAssignmentSettings(context.Background(), wanted)
	require.NoError(t, err)
	require.Equal(t, wanted, got)
	require.Equal(t, 1, proxyRepo.rebalances)
	persisted, err := settingsService.GetProxyAutoAssignmentSettings(context.Background())
	require.NoError(t, err)
	require.True(t, persisted.Enabled)
}

func TestUpdateProxyAutoAssignmentSettingsRollsBackOnRebalanceFailure(t *testing.T) {
	settingRepo := &proxyAutoAssignmentSettingRepo{}
	settingsService := NewSettingService(settingRepo, &config.Config{})
	proxyRepo := &proxyAutoAssignmentAdminRepo{selectedID: 42, rebalanceErr: errors.New("rebalance failed")}
	svc := &adminServiceImpl{settingService: settingsService, proxyRepo: proxyRepo}

	_, err := svc.UpdateProxyAutoAssignmentSettings(context.Background(), &ProxyAutoAssignmentSettings{
		Enabled:                    true,
		HealthCheckEnabled:         true,
		HealthCheckIntervalMinutes: 5,
		FailureThreshold:           2,
	})
	require.ErrorContains(t, err, "rebalance failed")
	persisted, loadErr := settingsService.GetProxyAutoAssignmentSettings(context.Background())
	require.NoError(t, loadErr)
	require.False(t, persisted.Enabled)
}

type proxyAutoAssignmentAccountRepo struct {
	AccountRepository
	created  *Account
	existing *Account
}

func (r *proxyAutoAssignmentAccountRepo) Create(_ context.Context, account *Account) error {
	clone := *account
	clone.ID = 99
	r.created = &clone
	account.ID = clone.ID
	return nil
}

func (r *proxyAutoAssignmentAccountRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	if r.existing == nil || r.existing.ID != id {
		return nil, ErrAccountNotFound
	}
	clone := *r.existing
	return &clone, nil
}

func (r *proxyAutoAssignmentAccountRepo) Update(_ context.Context, account *Account) error {
	clone := *account
	r.existing = &clone
	return nil
}

func (r *proxyAutoAssignmentAccountRepo) ListShadowsByParent(context.Context, int64) ([]*Account, error) {
	return []*Account{}, nil
}

func TestCreateAccountForcesLeastLoadedProxy(t *testing.T) {
	settingRepo := &proxyAutoAssignmentSettingRepo{}
	settingsService := NewSettingService(settingRepo, &config.Config{})
	require.NoError(t, settingsService.SetProxyAutoAssignmentSettings(context.Background(), &ProxyAutoAssignmentSettings{
		Enabled:                    true,
		HealthCheckEnabled:         false,
		HealthCheckIntervalMinutes: 15,
		FailureThreshold:           3,
	}))
	proxyRepo := &proxyAutoAssignmentAdminRepo{selectedID: 42}
	accountRepo := &proxyAutoAssignmentAccountRepo{}
	svc := &adminServiceImpl{
		settingService: settingsService,
		proxyRepo:      proxyRepo,
		accountRepo:    accountRepo,
	}
	manualProxyID := int64(7)

	created, err := svc.CreateAccount(context.Background(), &CreateAccountInput{
		Name:                  "forced-account",
		Platform:              PlatformAnthropic,
		Type:                  AccountTypeAPIKey,
		Credentials:           map[string]any{"api_key": "test-key"},
		Extra:                 map[string]any{},
		ProxyID:               &manualProxyID,
		Concurrency:           1,
		Priority:              50,
		SkipDefaultGroupBind:  true,
		SkipMixedChannelCheck: true,
	})
	require.NoError(t, err)
	require.NotNil(t, created.ProxyID)
	require.EqualValues(t, 42, *created.ProxyID)
	require.NotNil(t, accountRepo.created.ProxyID)
	require.EqualValues(t, 42, *accountRepo.created.ProxyID)
}

func TestUpdateAccountIgnoresManualProxyWhileAutoAssignmentEnabled(t *testing.T) {
	settingRepo := &proxyAutoAssignmentSettingRepo{}
	settingsService := NewSettingService(settingRepo, &config.Config{})
	require.NoError(t, settingsService.SetProxyAutoAssignmentSettings(context.Background(), &ProxyAutoAssignmentSettings{
		Enabled:                    true,
		HealthCheckEnabled:         false,
		HealthCheckIntervalMinutes: 15,
		FailureThreshold:           3,
	}))
	currentProxyID := int64(42)
	accountRepo := &proxyAutoAssignmentAccountRepo{existing: &Account{
		ID:          99,
		Name:        "existing",
		Platform:    PlatformAnthropic,
		Type:        AccountTypeAPIKey,
		Credentials: map[string]any{"api_key": "test-key"},
		Extra:       map[string]any{},
		ProxyID:     &currentProxyID,
		Status:      StatusActive,
		Schedulable: true,
	}}
	svc := &adminServiceImpl{
		settingService: settingsService,
		proxyRepo:      &proxyAutoAssignmentAdminRepo{selectedID: 7},
		accountRepo:    accountRepo,
	}
	manualProxyID := int64(7)

	updated, err := svc.UpdateAccount(context.Background(), 99, &UpdateAccountInput{
		Name:    "updated",
		ProxyID: &manualProxyID,
	})
	require.NoError(t, err)
	require.NotNil(t, updated.ProxyID)
	require.EqualValues(t, 42, *updated.ProxyID)
	require.NotNil(t, accountRepo.existing.ProxyID)
	require.EqualValues(t, 42, *accountRepo.existing.ProxyID)
}

type proxyHealthRepositoryStub struct {
	mu             sync.Mutex
	due            []Proxy
	deactivateIDs  map[int64]bool
	recorded       []proxyHealthProbeResult
	thresholds     []int
	rebalanceCalls int
}

func (r *proxyHealthRepositoryStub) ListDueProxyHealthChecks(context.Context, time.Time, time.Time, int) ([]Proxy, error) {
	return append([]Proxy(nil), r.due...), nil
}

func (r *proxyHealthRepositoryStub) RecordProxyHealthCheck(_ context.Context, proxyID int64, checkedAt time.Time, success bool, message string, threshold int) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recorded = append(r.recorded, proxyHealthProbeResult{proxyID: proxyID, checkedAt: checkedAt, success: success, message: message})
	r.thresholds = append(r.thresholds, threshold)
	return r.deactivateIDs[proxyID], nil
}

func (r *proxyHealthRepositoryStub) RebalanceActiveProxyAccounts(context.Context, time.Time) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rebalanceCalls++
	return 3, nil
}

type proxyHealthProberStub struct {
	failHost string
}

func (p *proxyHealthProberStub) ProbeProxy(_ context.Context, proxyURL string) (*ProxyExitInfo, int64, error) {
	if p.failHost != "" && strings.Contains(proxyURL, p.failHost) {
		return nil, 0, errors.New("connection failed with secret details")
	}
	return &ProxyExitInfo{IP: "203.0.113.1"}, 12, nil
}

func TestProxyHealthServiceDeactivatesAndRebalances(t *testing.T) {
	settingRepo := &proxyAutoAssignmentSettingRepo{}
	settingsService := NewSettingService(settingRepo, &config.Config{})
	require.NoError(t, settingsService.SetProxyAutoAssignmentSettings(context.Background(), &ProxyAutoAssignmentSettings{
		Enabled:                    true,
		HealthCheckEnabled:         true,
		HealthCheckIntervalMinutes: 5,
		FailureThreshold:           2,
	}))
	repo := &proxyHealthRepositoryStub{
		due: []Proxy{
			{ID: 1, Protocol: "http", Host: "healthy.example", Port: 8080},
			{ID: 2, Protocol: "http", Host: "failed.example", Port: 8080},
		},
		deactivateIDs: map[int64]bool{2: true},
	}
	svc := NewProxyHealthService(repo, settingsService, &proxyHealthProberStub{failHost: "failed.example"})
	fixedNow := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return fixedNow }

	require.NoError(t, svc.RunDue(context.Background()))
	require.Len(t, repo.recorded, 2)
	require.Equal(t, []int{2, 2}, repo.thresholds)
	require.True(t, repo.recorded[0].success)
	require.False(t, repo.recorded[1].success)
	require.Equal(t, "probe_failed", repo.recorded[1].message)
	require.NotContains(t, repo.recorded[1].message, "secret")
	require.Equal(t, 1, repo.rebalanceCalls)
}

func TestProxyHealthServiceDoesNothingWhenDisabled(t *testing.T) {
	settingsService := NewSettingService(&proxyAutoAssignmentSettingRepo{}, &config.Config{})
	repo := &proxyHealthRepositoryStub{due: []Proxy{{ID: 1, Protocol: "http", Host: "unused", Port: 8080}}}
	svc := NewProxyHealthService(repo, settingsService, &proxyHealthProberStub{})

	require.NoError(t, svc.RunDue(context.Background()))
	require.Empty(t, repo.recorded)
	require.Zero(t, repo.rebalanceCalls)
}

type proxyExpiryAutoAssignmentRepo struct {
	ProxyRepository
	expiredCounts []int64
	countCalls    int
	rebalances    int
}

func (r *proxyExpiryAutoAssignmentRepo) CountExpired(context.Context) (int64, error) {
	value := r.expiredCounts[r.countCalls]
	r.countCalls++
	return value, nil
}

func (r *proxyExpiryAutoAssignmentRepo) SweepExpiredProxies(context.Context, time.Time) (int64, error) {
	return 0, nil
}

func (r *proxyExpiryAutoAssignmentRepo) SelectLeastLoadedActiveProxy(context.Context, time.Time) (*int64, error) {
	id := int64(1)
	return &id, nil
}

func (r *proxyExpiryAutoAssignmentRepo) RebalanceActiveProxyAccounts(context.Context, time.Time) (int64, error) {
	r.rebalances++
	return 2, nil
}

func TestProxyExpiryServiceRebalancesNewlyExpiredProxyInAutoMode(t *testing.T) {
	settingRepo := &proxyAutoAssignmentSettingRepo{}
	settingsService := NewSettingService(settingRepo, &config.Config{})
	require.NoError(t, settingsService.SetProxyAutoAssignmentSettings(context.Background(), &ProxyAutoAssignmentSettings{
		Enabled:                    true,
		HealthCheckEnabled:         false,
		HealthCheckIntervalMinutes: 15,
		FailureThreshold:           3,
	}))
	repo := &proxyExpiryAutoAssignmentRepo{expiredCounts: []int64{0, 1}}
	svc := NewProxyExpiryService(repo, time.Minute)
	svc.SetAutoAssignment(settingsService, repo)

	svc.runOnce()

	require.Equal(t, 2, repo.countCalls)
	require.Equal(t, 1, repo.rebalances)
}

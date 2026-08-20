package egress

import (
	"context"
	"errors"
	"net/netip"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
)

type serviceTestStore struct {
	mu           sync.Mutex
	defaultPool  *Pool
	pools        map[int64]Pool
	bindings     map[int64]Binding
	inheritedIDs []int64
	modes        map[int64]platformegress.Mode
	upserted     chan int64
	inheritedErr error
}

type runtimeSettingsStub struct {
	values map[string]string
}

func (s *runtimeSettingsStub) GetValue(_ context.Context, key string) (string, error) {
	if value, ok := s.values[key]; ok {
		return value, nil
	}
	return "", errors.New("setting not found")
}

func (s *runtimeSettingsStub) Set(_ context.Context, key, value string) error {
	if s.values == nil {
		s.values = make(map[string]string)
	}
	s.values[key] = value
	return nil
}

func newServiceTestStore(pool Pool, accountIDs ...int64) *serviceTestStore {
	return &serviceTestStore{
		defaultPool:  &pool,
		pools:        map[int64]Pool{pool.ID: pool},
		bindings:     make(map[int64]Binding),
		inheritedIDs: accountIDs,
		modes:        make(map[int64]platformegress.Mode),
		upserted:     make(chan int64, len(accountIDs)+1),
	}
}

func (s *serviceTestStore) CreatePool(_ context.Context, input CreatePoolInput) (*Pool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var id int64 = 1
	for existingID := range s.pools {
		if existingID >= id {
			id = existingID + 1
		}
	}
	capacity, err := PoolCapacity(input.CIDR)
	if err != nil {
		return nil, err
	}
	pool := Pool{ID: id, Name: input.Name, CIDR: input.CIDR, Status: PoolStatusActive, IsDefault: input.IsDefault, AllocationVersion: 1, Capacity: capacity, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	s.pools[id] = pool
	if input.IsDefault {
		copy := pool
		s.defaultPool = &copy
	}
	copy := pool
	return &copy, nil
}

func (s *serviceTestStore) UpdatePool(_ context.Context, id int64, input UpdatePoolInput) (*Pool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pool, ok := s.pools[id]
	if !ok {
		return nil, ErrPoolNotFound
	}
	if input.Name != nil {
		pool.Name = *input.Name
	}
	if input.NodeID != nil {
		if *input.NodeID == "" {
			pool.NodeID = nil
		} else {
			nodeID := *input.NodeID
			pool.NodeID = &nodeID
		}
	}
	if input.Status != nil {
		pool.Status = *input.Status
	}
	if input.IsDefault != nil {
		pool.IsDefault = *input.IsDefault
		if *input.IsDefault {
			for poolID, other := range s.pools {
				other.IsDefault = poolID == id
				s.pools[poolID] = other
			}
		}
	}
	s.pools[id] = pool
	if pool.IsDefault {
		copy := pool
		s.defaultPool = &copy
	} else if s.defaultPool != nil && s.defaultPool.ID == id {
		s.defaultPool = nil
	}
	copy := pool
	return &copy, nil
}

func (s *serviceTestStore) DeletePool(context.Context, int64) error {
	return errors.New("not implemented")
}

func (s *serviceTestStore) GetPool(_ context.Context, id int64) (*Pool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pool, ok := s.pools[id]
	if !ok {
		return nil, ErrPoolNotFound
	}
	copy := pool
	return &copy, nil
}

func (s *serviceTestStore) GetDefaultPool(context.Context) (*Pool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.defaultPool == nil {
		return nil, ErrPoolNotFound
	}
	copy := *s.defaultPool
	return &copy, nil
}

func (s *serviceTestStore) ListPools(context.Context) ([]Pool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pools := make([]Pool, 0, len(s.pools))
	for _, pool := range s.pools {
		pools = append(pools, pool)
	}
	return pools, nil
}

func (s *serviceTestStore) GetBinding(_ context.Context, accountID int64) (*Binding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	binding, ok := s.bindings[accountID]
	if !ok {
		return nil, ErrBindingNotFound
	}
	copy := binding
	return &copy, nil
}

func (s *serviceTestStore) GetAnyBindingForPool(_ context.Context, poolID int64) (*Binding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, binding := range s.bindings {
		if binding.PoolID == poolID {
			copy := binding
			return &copy, nil
		}
	}
	return nil, ErrBindingNotFound
}

func (s *serviceTestStore) ListBindings(context.Context, int, int, string) (*BindingPage, error) {
	return nil, errors.New("not implemented")
}

func (s *serviceTestStore) UpsertBinding(_ context.Context, binding Binding, expectedVersion *int64) (*Binding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, exists := s.bindings[binding.AccountID]
	if exists && (expectedVersion == nil || current.Version != *expectedVersion) {
		return nil, ErrBindingChanged
	}
	if !exists && expectedVersion != nil {
		return nil, ErrBindingChanged
	}
	binding.ID = binding.AccountID
	binding.PoolStatus = PoolStatusActive
	binding.CreatedAt = time.Now()
	binding.UpdatedAt = binding.CreatedAt
	s.bindings[binding.AccountID] = binding
	select {
	case s.upserted <- binding.AccountID:
	default:
	}
	copy := binding
	return &copy, nil
}

func (s *serviceTestStore) DeleteBinding(_ context.Context, accountID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.bindings[accountID]; !ok {
		return ErrBindingNotFound
	}
	delete(s.bindings, accountID)
	return nil
}

func (s *serviceTestStore) SetAccountMode(_ context.Context, accountID int64, mode platformegress.Mode) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modes[accountID] = mode
	return nil
}

func (s *serviceTestStore) ListInheritedAccountIDsWithoutBinding(_ context.Context, limit int) ([]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inheritedErr != nil {
		return nil, s.inheritedErr
	}
	ids := make([]int64, 0, limit)
	for _, id := range s.inheritedIDs {
		if _, exists := s.bindings[id]; exists {
			continue
		}
		ids = append(ids, id)
		if len(ids) == limit {
			break
		}
	}
	return ids, nil
}

func TestServiceReconcileDefaultPersistsStableBindings(t *testing.T) {
	pool := Pool{ID: 2, Name: "default", CIDR: "2001:db8:10::/64", Status: PoolStatusActive, IsDefault: true, AllocationVersion: 1}
	store := newServiceTestStore(pool, 11, 12)
	svc := NewService(store, &config.Config{IPv6Egress: config.IPv6EgressConfig{AllocationSecret: "0123456789abcdef0123456789abcdef"}})

	completed, err := svc.ReconcileDefault(context.Background(), 100)
	if err != nil || completed != 2 {
		t.Fatalf("ReconcileDefault() = %d, %v", completed, err)
	}
	first := store.bindings[11]
	if first.SourceIPv6 == "" || first.SourceIPv6 == store.bindings[12].SourceIPv6 {
		t.Fatalf("bindings = %#v", store.bindings)
	}
	completed, err = svc.ReconcileDefault(context.Background(), 100)
	if err != nil || completed != 0 {
		t.Fatalf("second ReconcileDefault() = %d, %v", completed, err)
	}
	if store.bindings[11].SourceIPv6 != first.SourceIPv6 || store.bindings[11].Version != first.Version {
		t.Fatalf("binding changed across reconciliation: before=%#v after=%#v", first, store.bindings[11])
	}
}

func TestServiceStartReconcilesEnabledWorkerImmediately(t *testing.T) {
	pool := Pool{ID: 3, Name: "default", CIDR: "2001:db8:20::/64", Status: PoolStatusActive, IsDefault: true, AllocationVersion: 1}
	store := newServiceTestStore(pool, 21)
	svc := NewService(store, &config.Config{
		Deployment: config.DeploymentConfig{Mode: config.DeploymentModeStandalone},
		IPv6Egress: config.IPv6EgressConfig{
			Enabled:                  true,
			AllocationSecret:         "0123456789abcdef0123456789abcdef",
			ReconcileIntervalSeconds: 60,
		},
	})
	svc.Start()
	defer svc.Stop()

	select {
	case accountID := <-store.upserted:
		if accountID != 21 {
			t.Fatalf("reconciled account = %d", accountID)
		}
	case <-time.After(time.Second):
		t.Fatal("startup reconciliation did not allocate the inherited account")
	}
}

func TestServiceSetAccountRouteDirectRemovesBinding(t *testing.T) {
	pool := Pool{ID: 4, Name: "default", CIDR: "2001:db8:30::/64", Status: PoolStatusActive, IsDefault: true, AllocationVersion: 1}
	store := newServiceTestStore(pool)
	store.bindings[31] = Binding{AccountID: 31, PoolID: 4, SourceIPv6: "2001:db8:30::31", Status: BindingStatusActive, PoolStatus: PoolStatusActive, Version: 1}
	svc := NewService(store, &config.Config{IPv6Egress: config.IPv6EgressConfig{AllocationSecret: "0123456789abcdef0123456789abcdef"}})

	binding, err := svc.SetAccountRoute(context.Background(), 31, SetAccountRouteInput{Mode: platformegress.ModeDirect})
	if err != nil || binding != nil {
		t.Fatalf("SetAccountRoute() = %#v, %v", binding, err)
	}
	if store.modes[31] != platformegress.ModeDirect {
		t.Fatalf("mode = %q", store.modes[31])
	}
	if _, exists := store.bindings[31]; exists {
		t.Fatal("direct route retained an obsolete IPv6 binding")
	}
}

func TestServiceRequiresSuccessfulProbeBeforeDefault(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("IPv6 egress runtime is Linux-only")
	}
	pool := Pool{ID: 5, Name: "candidate", CIDR: "2001:db8:40::/64", Status: PoolStatusActive, AllocationVersion: 1}
	store := newServiceTestStore(pool)
	store.bindings[41] = Binding{
		AccountID:  41,
		PoolID:     pool.ID,
		SourceIPv6: "2001:db8:40::41",
		Status:     BindingStatusActive,
		PoolStatus: PoolStatusActive,
		Version:    1,
	}
	svc := NewService(store, &config.Config{IPv6Egress: config.IPv6EgressConfig{
		Enabled:          true,
		AllocationSecret: "0123456789abcdef0123456789abcdef",
		ProbeURL:         "https://probe.example",
	}})

	probeErr := errors.New("route unavailable")
	svc.probe = func(context.Context, platformegress.Route, platformegress.Policy, string, time.Duration) (*platformegress.ProbeResult, error) {
		if probeErr != nil {
			return nil, probeErr
		}
		return &platformegress.ProbeResult{ObservedIP: "2001:db8:40::41"}, nil
	}
	if _, err := svc.ProbeAccount(context.Background(), 41); !errors.Is(err, probeErr) {
		t.Fatalf("failed ProbeAccount() error = %v", err)
	}
	makeDefault := true
	if _, err := svc.UpdatePool(context.Background(), pool.ID, UpdatePoolInput{IsDefault: &makeDefault}); !errors.Is(err, ErrPoolUnhealthy) {
		t.Fatalf("unhealthy UpdatePool() error = %v", err)
	}
	pools, err := svc.ListPools(context.Background())
	if err != nil || len(pools) != 1 || pools[0].RouteHealthy == nil || *pools[0].RouteHealthy {
		t.Fatalf("failed pool health = %#v, %v", pools, err)
	}

	probeErr = nil
	if _, err := svc.ProbeAccount(context.Background(), 41); err != nil {
		t.Fatalf("successful ProbeAccount() error = %v", err)
	}
	updated, err := svc.UpdatePool(context.Background(), pool.ID, UpdatePoolInput{IsDefault: &makeDefault})
	if err != nil || !updated.IsDefault || updated.RouteHealthy == nil || !*updated.RouteHealthy {
		t.Fatalf("healthy UpdatePool() = %#v, %v", updated, err)
	}

	disabled := PoolStatusDisabled
	updated, err = svc.UpdatePool(context.Background(), pool.ID, UpdatePoolInput{Status: &disabled})
	if err != nil || updated.RouteHealthy != nil {
		t.Fatalf("disabled UpdatePool() retained health = %#v, %v", updated, err)
	}
}

func TestServicePreflightsWhenWorkerIsDisabled(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("IPv6 egress runtime is Linux-only")
	}
	pool := Pool{ID: 6, Name: "default", CIDR: "2001:db8:50::/64", Status: PoolStatusActive, IsDefault: true, AllocationVersion: 1}
	store := newServiceTestStore(pool, 52)
	store.bindings[51] = Binding{
		AccountID: 51, PoolID: pool.ID, SourceIPv6: "2001:db8:50::51",
		Status: BindingStatusActive, PoolStatus: PoolStatusActive, Version: 1,
	}
	svc := NewService(store, &config.Config{
		Deployment: config.DeploymentConfig{WorkerEnabled: config.WorkerModeDisabled},
		IPv6Egress: config.IPv6EgressConfig{
			Enabled: true, AllocationSecret: "0123456789abcdef0123456789abcdef",
			ProbeURL: "https://probe.example", ReconcileIntervalSeconds: 60,
		},
	})
	probed := make(chan struct{}, 1)
	svc.probe = func(context.Context, platformegress.Route, platformegress.Policy, string, time.Duration) (*platformegress.ProbeResult, error) {
		probed <- struct{}{}
		return &platformegress.ProbeResult{ObservedIP: "2001:db8:50::51"}, nil
	}
	svc.Start()
	defer svc.Stop()

	select {
	case <-probed:
	case <-time.After(time.Second):
		t.Fatal("worker-disabled node did not run its local route preflight")
	}
	select {
	case accountID := <-store.upserted:
		t.Fatalf("worker-disabled node reconciled account %d", accountID)
	default:
	}
}

func TestServicePreflightsAfterReconciliationError(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("IPv6 egress runtime is Linux-only")
	}
	pool := Pool{ID: 7, Name: "default", CIDR: "2001:db8:60::/64", Status: PoolStatusActive, IsDefault: true, AllocationVersion: 1}
	store := newServiceTestStore(pool)
	store.inheritedErr = errors.New("list inherited accounts")
	store.bindings[61] = Binding{
		AccountID: 61, PoolID: pool.ID, SourceIPv6: "2001:db8:60::61",
		Status: BindingStatusActive, PoolStatus: PoolStatusActive, Version: 1,
	}
	svc := NewService(store, &config.Config{IPv6Egress: config.IPv6EgressConfig{
		Enabled: true, AllocationSecret: "0123456789abcdef0123456789abcdef",
		ProbeURL: "https://probe.example", ReconcileIntervalSeconds: 60,
	}})
	probed := make(chan struct{}, 1)
	svc.probe = func(context.Context, platformegress.Route, platformegress.Policy, string, time.Duration) (*platformegress.ProbeResult, error) {
		probed <- struct{}{}
		return &platformegress.ProbeResult{ObservedIP: "2001:db8:60::61"}, nil
	}
	svc.Start()
	defer svc.Stop()

	select {
	case <-probed:
	case <-time.After(time.Second):
		t.Fatal("reconciliation error prevented the independent route preflight")
	}
}

func TestServiceAutoConfigurePersistsSwitchSecretAndDefaultPool(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("IPv6 auto-configuration is Linux-only")
	}
	platformegress.ClearRuntimeEnabledOverride()
	t.Cleanup(platformegress.ClearRuntimeEnabledOverride)
	initial := Pool{ID: 9, Name: "existing", CIDR: "2001:db8:90::/64", Status: PoolStatusDisabled, AllocationVersion: 1}
	store := newServiceTestStore(initial)
	settings := &runtimeSettingsStub{values: map[string]string{}}
	svc := NewService(store, &config.Config{
		Deployment: config.DeploymentConfig{Mode: config.DeploymentModeStandalone, WorkerEnabled: config.WorkerModeDisabled},
		IPv6Egress: config.IPv6EgressConfig{ProbeURL: "https://probe.example", FreeBind: true},
	})
	svc.SetRuntimeSettings(settings)
	svc.detect = func() (*platformegress.DetectedIPv6Network, error) {
		return &platformegress.DetectedIPv6Network{
			Address:   netip.MustParseAddr("2001:4860:abcd:1::42"),
			Prefix:    netip.MustParsePrefix("2001:4860:abcd:1::/64"),
			Interface: "eth0",
		}, nil
	}
	svc.probeSource = func(context.Context, string, platformegress.Policy, string, time.Duration) (*platformegress.ProbeResult, error) {
		return &platformegress.ProbeResult{ObservedIP: "2001:4860:abcd:1::42"}, nil
	}
	result, err := svc.AutoConfigure(context.Background())
	if err != nil {
		t.Fatalf("AutoConfigure() error = %v", err)
	}
	if result == nil || result.Pool == nil || !result.Pool.IsDefault || result.Pool.CIDR != "2001:4860:abcd:1::/64" {
		t.Fatalf("unexpected auto-configure result: %#v", result)
	}
	if !svc.IsEnabled() || settings.values[RuntimeSettingKey] != "true" || len(settings.values[allocationSecretSettingKey]) != 64 {
		t.Fatalf("runtime settings were not persisted: enabled=%v values=%v", svc.IsEnabled(), settings.values)
	}
	svc.Stop()
}

func TestServiceAutoConfigureRollsBackWhenNoGlobalAddress(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("IPv6 auto-configuration is Linux-only")
	}
	platformegress.ClearRuntimeEnabledOverride()
	t.Cleanup(platformegress.ClearRuntimeEnabledOverride)
	store := newServiceTestStore(Pool{ID: 10, Name: "existing", CIDR: "2001:db8:100::/64", Status: PoolStatusActive, AllocationVersion: 1})
	settings := &runtimeSettingsStub{values: map[string]string{}}
	svc := NewService(store, &config.Config{Deployment: config.DeploymentConfig{Mode: config.DeploymentModeStandalone}, IPv6Egress: config.IPv6EgressConfig{}})
	svc.SetRuntimeSettings(settings)
	svc.detect = func() (*platformegress.DetectedIPv6Network, error) { return nil, platformegress.ErrIPv6AutoDetect }
	if _, err := svc.AutoConfigure(context.Background()); !errors.Is(err, ErrAutoConfigure) {
		t.Fatalf("AutoConfigure() error = %v", err)
	}
	if svc.IsEnabled() || settings.values[RuntimeSettingKey] != "false" {
		t.Fatalf("auto-configure did not roll back switch: enabled=%v values=%v", svc.IsEnabled(), settings.values)
	}
}

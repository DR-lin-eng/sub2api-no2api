package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/lenovoattribution"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type lenovoAttributionRepo struct {
	AccountRepository
	mu        sync.Mutex
	accounts  map[int64]*Account
	failWrite bool
}

func cloneLenovoAccount(a *Account) *Account {
	if a == nil {
		return nil
	}
	out := *a
	out.Credentials = maps.Clone(a.Credentials)
	raw, _ := json.Marshal(a.Extra)
	out.Extra = nil
	_ = json.Unmarshal(raw, &out.Extra)
	return &out
}
func (r *lenovoAttributionRepo) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if a := r.accounts[id]; a != nil {
		return cloneLenovoAccount(a), nil
	}
	return nil, ErrAccountNotFound
}
func (r *lenovoAttributionRepo) CompareAndSwapOpenAILenovoAttribution(_ context.Context, a *Account, expected any, next *lenovoattribution.State) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failWrite {
		return false, errors.New("fixture store failed")
	}
	current := r.accounts[a.ID]
	if current == nil {
		return false, ErrAccountNotFound
	}
	before, _ := json.Marshal(expected)
	stored, _ := json.Marshal(current.Extra[lenovoattribution.ExtraKey])
	creds, _ := json.Marshal(a.Credentials)
	currentCreds, _ := json.Marshal(current.Credentials)
	var beforeValue, storedValue any
	_ = json.Unmarshal(before, &beforeValue)
	_ = json.Unmarshal(stored, &storedValue)
	if !reflect.DeepEqual(beforeValue, storedValue) || string(creds) != string(currentCreds) {
		return false, nil
	}
	if current.Extra == nil {
		current.Extra = make(map[string]any)
	}
	copy := *next
	current.Extra[lenovoattribution.ExtraKey] = &copy
	return true, nil
}

type lenovoAttributionHTTPUpstream struct {
	mu       sync.Mutex
	requests []*http.Request
	bodies   [][]byte
	status   int
	header   string
	block    <-chan struct{}
}

func (u *lenovoAttributionHTTPUpstream) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	body, _ := io.ReadAll(req.Body)
	u.mu.Lock()
	u.requests = append(u.requests, req.Clone(req.Context()))
	u.bodies = append(u.bodies, body)
	u.mu.Unlock()
	if u.block != nil {
		select {
		case <-u.block:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
	h := http.Header{}
	if u.header != "" {
		h.Set(lenovoattribution.RecordedHeader, u.header)
	}
	return &http.Response{StatusCode: u.status, Header: h, Body: io.NopCloser(strings.NewReader(""))}, nil
}
func (u *lenovoAttributionHTTPUpstream) DoWithTLS(r *http.Request, p string, id int64, c int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(r, p, id, c)
}
func (u *lenovoAttributionHTTPUpstream) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.requests)
}

func lenovoAccount(id int64) *Account {
	return &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive,
		Credentials: map[string]any{"access_token": "synthetic-token", "chatgpt_account_id": "shared-workspace", "chatgpt_user_id": "fixture-user"},
		Extra:       map[string]any{"openai_device_id": "shared-untrusted-device-value"}}
}
func lenovoFixture(accounts ...*Account) (*OpenAIGatewayService, *lenovoAttributionRepo, *lenovoAttributionHTTPUpstream) {
	repo := &lenovoAttributionRepo{accounts: make(map[int64]*Account)}
	for _, a := range accounts {
		repo.accounts[a.ID] = cloneLenovoAccount(a)
	}
	settings := NewSettingService(nil, &config.Config{})
	settings.refreshOpenAIAuxiliarySettings(&SystemSettings{OpenAILenovoAttributionEnabled: true})
	upstream := &lenovoAttributionHTTPUpstream{status: 204, header: "1"}
	return &OpenAIGatewayService{accountRepo: repo, settingService: settings, httpUpstream: upstream}, repo, upstream
}
func fetchLenovo(t *testing.T, s *OpenAIGatewayService, a *Account) *lenovoattribution.State {
	t.Helper()
	binding, _ := openAILenovoAttributionBinding(a)
	result, err := s.fetchOpenAILenovoAttribution(t.Context(), a.ID, binding)
	require.NoError(t, err)
	return result
}
func TestOpenAILenovoAttributionIndependentPersistentInstallations(t *testing.T) {
	a, b := lenovoAccount(301), lenovoAccount(302)
	b.Credentials["access_token"] = "synthetic-token-b"
	svc, repo, upstream := lenovoFixture(a, b)
	first, second := fetchLenovo(t, svc, a), fetchLenovo(t, svc, b)
	require.NotEqual(t, first.InstallationID, second.InstallationID)
	_, err := uuid.Parse(first.InstallationID)
	require.NoError(t, err)
	require.True(t, first.Terminal)
	require.False(t, first.DeviceRecorded, "installation-only 204 cannot prove serial recording")
	require.Len(t, upstream.requests, 2)
	for i, result := range []*lenovoattribution.State{first, second} {
		require.Equal(t, lenovoattribution.Endpoint, upstream.requests[i].URL.String())
		require.Equal(t, "application/json", upstream.requests[i].Header.Get("Content-Type"))
		require.Empty(t, upstream.requests[i].Header.Get(lenovoattribution.RecordedHeader))
		require.Empty(t, upstream.requests[i].Header.Get("x-oai-attestation"))
		require.True(t, HTTPUpstreamRedirectsDisabled(upstream.requests[i].Context()))
		require.JSONEq(t, `{"installation_id":"`+result.InstallationID+`"}`, string(upstream.bodies[i]))
	}
	require.Equal(t, "Bearer synthetic-token-b", upstream.requests[1].Header.Get("Authorization"))
	// New service: no process cache survives. It must respect the durable result.
	restarted := &OpenAIGatewayService{accountRepo: repo, settingService: svc.settingService, httpUpstream: upstream}
	restored := fetchLenovo(t, restarted, a)
	require.Equal(t, first.InstallationID, restored.InstallationID)
	require.Equal(t, 2, upstream.count())
	// Changing principal invalidates the old result, keeping this installation.
	repo.mu.Lock()
	repo.accounts[a.ID].Credentials["chatgpt_user_id"] = "new-user"
	repo.mu.Unlock()
	changed, _ := repo.GetByID(t.Context(), a.ID)
	current := fetchLenovo(t, restarted, changed)
	require.NotEqual(t, first.Binding, current.Binding)
	require.Equal(t, first.InstallationID, current.InstallationID)
	require.Equal(t, 3, upstream.count())
}
func TestOpenAILenovoAttributionRespectsSwitchAndPersistenceFailure(t *testing.T) {
	a := lenovoAccount(303)
	svc, repo, upstream := lenovoFixture(a)
	repo.failWrite = true
	binding, _ := openAILenovoAttributionBinding(a)
	_, err := svc.fetchOpenAILenovoAttribution(t.Context(), a.ID, binding)
	require.Error(t, err)
	require.Zero(t, upstream.count(), "never send if installation/lease cannot be stored")
	svc.settingService.refreshOpenAIAuxiliarySettings(&SystemSettings{})
	result, err := svc.fetchOpenAILenovoAttribution(t.Context(), a.ID, binding)
	require.NoError(t, err)
	require.Nil(t, result)
	require.Zero(t, upstream.count())
}
func TestOpenAILenovoAttributionPersistsRetryAndExact204State(t *testing.T) {
	a := lenovoAccount(304)
	// Synthetic digest is test input, never generated by the production code.
	a.Extra[lenovoattribution.DigestExtraKey] = strings.Repeat("ab", 32)
	svc, repo, upstream := lenovoFixture(a)
	first := fetchLenovo(t, svc, a)
	require.True(t, first.DeviceRecorded)
	require.WithinDuration(t, time.Now().Add(7*24*time.Hour), first.NextRetryAt, time.Second)
	require.Contains(t, string(upstream.bodies[0]), "device_serial_digest")
	stored, _ := repo.GetByID(t.Context(), a.ID)
	state, err := lenovoattribution.Read(stored.Extra)
	require.NoError(t, err)
	require.Equal(t, first.NextRetryAt, state.NextRetryAt)
	// Expire the fixture's durable TTL to exercise the next real attempt.
	repo.mu.Lock()
	next := *state
	next.NextRetryAt = time.Now().Add(-time.Second)
	repo.accounts[a.ID].Extra[lenovoattribution.ExtraKey] = &next
	repo.mu.Unlock()
	upstream.header = " 1"
	second := fetchLenovo(t, svc, a)
	require.False(t, second.DeviceRecorded)
	require.WithinDuration(t, time.Now().Add(6*time.Hour), second.NextRetryAt, time.Second)
}
func TestOpenAILenovoAttributionBackgroundBoundsBeforeSpawning(t *testing.T) {
	var accounts []*Account
	for i := int64(1); i <= 20; i++ {
		accounts = append(accounts, lenovoAccount(400+i))
	}
	svc, _, upstream := lenovoFixture(accounts...)
	release := make(chan struct{})
	upstream.block = release
	defer close(release)
	for range 10 {
		for _, a := range accounts {
			svc.scheduleOpenAILenovoAttribution(a)
		}
	}
	require.Eventually(t, func() bool { return upstream.count() == openAILenovoAttributionWorkers }, time.Second, 5*time.Millisecond)
	svc.openAILenovoAttributionMu.Lock()
	active := svc.openAILenovoAttributionActive
	svc.openAILenovoAttributionMu.Unlock()
	require.Equal(t, openAILenovoAttributionWorkers, active)
}

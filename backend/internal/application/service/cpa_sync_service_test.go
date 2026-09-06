//go:build unit

package service

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/modules/cpasync"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type cpaSyncTestRepo struct {
	AccountRepository
	accounts  map[int64]*Account
	lookupErr error
}

func (r *cpaSyncTestRepo) FindByExtraField(_ context.Context, key string, value any) ([]Account, error) {
	if r.lookupErr != nil {
		return nil, r.lookupErr
	}
	result := []Account{}
	for _, a := range r.accounts {
		if a.Extra[key] == value {
			result = append(result, *a)
		}
	}
	return result, nil
}

type cpaSyncTestAdmin struct {
	AdminService
	repo             *cpaSyncTestRepo
	groups           map[int64]*Group
	created, updated int
	lastUpdate       *UpdateAccountInput
	saveErr          error
}

func (a *cpaSyncTestAdmin) GetGroup(_ context.Context, id int64) (*Group, error) {
	return a.groups[id], nil
}
func (a *cpaSyncTestAdmin) GetAccount(_ context.Context, id int64) (*Account, error) {
	return a.repo.accounts[id], nil
}
func (a *cpaSyncTestAdmin) CreateAccount(_ context.Context, input *CreateAccountInput) (*Account, error) {
	if a.saveErr != nil {
		return nil, a.saveErr
	}
	account, err := buildAccountForCreate(input, input.Extra)
	if err != nil {
		return nil, err
	}
	a.created++
	account.ID = int64(len(a.repo.accounts) + 1)
	account.GroupIDs = append([]int64{}, input.GroupIDs...)
	a.repo.accounts[account.ID] = account
	return account, nil
}
func (a *cpaSyncTestAdmin) UpdateAccount(_ context.Context, id int64, input *UpdateAccountInput) (*Account, error) {
	a.lastUpdate = input
	if a.saveErr != nil {
		return nil, a.saveErr
	}
	a.updated++
	account := a.repo.accounts[id]
	account.Credentials = input.Credentials
	account.Extra = input.Extra
	if input.GroupIDs != nil {
		account.GroupIDs = append([]int64{}, (*input.GroupIDs)...)
	}
	return account, nil
}

type cpaSyncInvalidator struct{ calls int }

func (i *cpaSyncInvalidator) InvalidateToken(context.Context, *Account) error { i.calls++; return nil }

type cpaRemoteFixture struct {
	mu                sync.Mutex
	files             []cpasync.File
	credentials       map[string]map[string]any
	downloads         []string
	listCalls         int
	listStatus        int
	downloadStatus    map[string]int
	failListAt        int
	afterDownload     func(*cpaRemoteFixture, string)
	active, maxActive int
	delay             time.Duration
}

func (f *cpaRemoteFixture) serve(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer fixture-password" {
		w.WriteHeader(401)
		return
	}
	f.mu.Lock()
	if !strings.HasSuffix(r.URL.Path, "/download") {
		defer f.mu.Unlock()
		f.listCalls++
		if f.listStatus != 0 || (f.failListAt > 0 && f.listCalls >= f.failListAt) {
			w.WriteHeader(503)
			_, _ = w.Write([]byte("fixture-password access_token secret"))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"files": f.files})
		return
	}
	name := r.URL.Query().Get("name")
	f.downloads = append(f.downloads, name)
	f.active++
	if f.active > f.maxActive {
		f.maxActive = f.active
	}
	data, status, delay := f.credentials[name], f.downloadStatus[name], f.delay
	f.mu.Unlock()
	time.Sleep(delay)
	f.mu.Lock()
	f.active--
	if f.afterDownload != nil {
		f.afterDownload(f, name)
	}
	f.mu.Unlock()
	if status != 0 {
		w.WriteHeader(status)
		_, _ = w.Write([]byte("fixture-password access_token secret"))
		return
	}
	_ = json.NewEncoder(w).Encode(data)
}
func newCPASyncFixture(t *testing.T) (*CPASyncService, *cpaSyncTestAdmin, *cpaRemoteFixture, CPASyncInput, *cpaSyncInvalidator) {
	t.Helper()
	remote := &cpaRemoteFixture{files: []cpasync.File{}, credentials: map[string]map[string]any{}, downloadStatus: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(remote.serve))
	t.Cleanup(server.Close)
	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowPrivateHosts = true
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	repo := &cpaSyncTestRepo{accounts: map[int64]*Account{}}
	admin := &cpaSyncTestAdmin{repo: repo, groups: map[int64]*Group{10: {ID: 10, Platform: PlatformOpenAI, Status: StatusActive}, 11: {ID: 11, Platform: PlatformOpenAI, Status: StatusActive}, 20: {ID: 20, Platform: PlatformAnthropic, Status: StatusActive}, 30: {ID: 30, Platform: PlatformOpenAI, Status: StatusDisabled}}}
	invalidator := &cpaSyncInvalidator{}
	svc := NewCPASyncService(repo, admin, cfg, nil, invalidator)
	input := CPASyncInput{CPAConnectionInput: CPAConnectionInput{BaseURL: server.URL, ManagementPassword: "fixture-password", Platform: PlatformOpenAI}, GroupIDs: []int64{10}}
	return svc, admin, remote, input, invalidator
}
func addCPAFixtureFile(remote *cpaRemoteFixture, name string) {
	remote.files = append(remote.files, cpasync.File{Name: name, Provider: "codex", Status: "active"})
	remote.credentials[name] = map[string]any{"type": "codex", "access_token": "old-access", "refresh_token": "refresh", "email": name + "@example.test", "account_id": "team", "chatgpt_user_id": name}
}
func cpaBoolPtr(v bool) *bool       { return &v }
func cpaStringPtr(v string) *string { return &v }

func TestCPASyncPreviewAndHealthyOnly(t *testing.T) {
	svc, admin, remote, input, _ := newCPASyncFixture(t)
	addCPAFixtureFile(remote, "healthy.json")
	addCPAFixtureFile(remote, "disabled.json")
	addCPAFixtureFile(remote, "error.json")
	addCPAFixtureFile(remote, "unavailable.json")
	remote.files[1].Disabled = true
	remote.files[2].Status = "error"
	remote.files[3].Unavailable = true
	remote.files = append(remote.files, cpasync.File{Name: "unsupported.json", Provider: "qwen", Status: "active"})
	preview, err := svc.Preview(context.Background(), input.CPAConnectionInput)
	require.NoError(t, err)
	require.Len(t, preview.Accounts, 1)
	require.Equal(t, 5, preview.Total)
	require.Equal(t, 4, preview.Skipped)
	require.Empty(t, remote.downloads)
	require.Empty(t, admin.repo.accounts)
	raw, err := json.Marshal(preview)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "old-access")
	require.NotContains(t, string(raw), "fixture-password")
	input.SelectedFiles = []string{"healthy.json", "disabled.json", "error.json", "unavailable.json", "missing.json"}
	result, err := svc.Sync(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 1, result.Created)
	require.Equal(t, 4, result.Skipped)
	require.Zero(t, result.Failed)
	require.Equal(t, []string{"healthy.json"}, remote.downloads)
	require.Len(t, admin.repo.accounts, 1)
	account := admin.repo.accounts[1]
	require.Equal(t, StatusActive, account.Status)
	require.True(t, account.Schedulable)
	require.Equal(t, []int64{10}, account.GroupIDs)
	require.NotContains(t, account.Credentials, "management_password")
	require.NotContains(t, account.Extra, "management_password")
}

func TestCPASyncRepeatedUpdatesAndExplicitSettings(t *testing.T) {
	svc, admin, remote, input, invalidator := newCPASyncFixture(t)
	addCPAFixtureFile(remote, "one.json")
	input.SelectedFiles = []string{"one.json"}
	input.OAuthOptions = CPAOAuthOptions{TLSFingerprint: cpaBoolPtr(true), Passthrough: cpaBoolPtr(true), WSMode: cpaStringPtr("ctx_pool"), FingerprintMode: cpaStringPtr("session"), CompactMode: cpaStringPtr("force_on"), LongContextBilling: cpaBoolPtr(true)}
	first, err := svc.Sync(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 1, first.Created)
	account := admin.repo.accounts[1]
	require.True(t, account.IsTLSFingerprintEnabled())
	require.Equal(t, "ctx_pool", account.Extra["openai_oauth_responses_websockets_v2_mode"])
	account.Name = "local-name"
	account.Status = StatusDisabled
	account.Schedulable = false
	account.Concurrency = 17
	account.Extra["unrelated"] = true
	account.Credentials["model_mapping"] = map[string]any{"a": "b"}
	remote.credentials["one.json"]["access_token"] = "new-access"
	delete(remote.credentials["one.json"], "refresh_token")
	input.BaseURL += "/v1/"
	input.GroupIDs = []int64{11}
	input.OAuthOptions = CPAOAuthOptions{TLSFingerprint: cpaBoolPtr(false), Passthrough: cpaBoolPtr(false), WSMode: cpaStringPtr("off"), FingerprintMode: cpaStringPtr("off"), CompactMode: cpaStringPtr("auto"), LongContextBilling: cpaBoolPtr(false)}
	second, err := svc.Sync(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 1, second.Updated)
	require.Zero(t, second.Created)
	require.Len(t, admin.repo.accounts, 1)
	require.Equal(t, "new-access", account.GetCredential("access_token"))
	require.Equal(t, "refresh", account.GetCredential("refresh_token"))
	require.Contains(t, account.Credentials, "model_mapping")
	require.Equal(t, []int64{10}, account.GroupIDs)
	require.True(t, account.IsTLSFingerprintEnabled())
	require.True(t, account.Extra["unrelated"].(bool))
	require.Equal(t, "local-name", account.Name)
	require.Equal(t, 17, account.Concurrency)
	require.Equal(t, StatusDisabled, account.Status)
	require.False(t, account.Schedulable)
	require.Empty(t, admin.lastUpdate.Status)
	require.Empty(t, admin.lastUpdate.Name)
	require.Nil(t, admin.lastUpdate.Concurrency)
	input.ApplySettingsToExisting = true
	third, err := svc.Sync(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 1, third.Updated)
	require.Equal(t, []int64{11}, account.GroupIDs)
	require.False(t, account.IsTLSFingerprintEnabled())
	require.Equal(t, false, account.Extra["openai_passthrough"])
	require.Equal(t, "off", account.Extra["codex_fingerprint_mode"])
	require.Equal(t, false, account.Extra["openai_long_context_billing_enabled"])
	require.Equal(t, 3, invalidator.calls)
	input.GroupIDs = []int64{}
	_, err = svc.Sync(context.Background(), input)
	require.NoError(t, err)
	require.Empty(t, account.GroupIDs)
	preview, err := svc.Preview(context.Background(), input.CPAConnectionInput)
	require.NoError(t, err)
	require.True(t, preview.Accounts[0].Existing)
	require.Equal(t, int64(1), preview.Accounts[0].AccountID)
}

func TestCPASyncRechecksHealthAfterDownload(t *testing.T) {
	for _, change := range []string{"disabled", "error", "removed"} {
		t.Run(change, func(t *testing.T) {
			svc, admin, remote, input, _ := newCPASyncFixture(t)
			addCPAFixtureFile(remote, "one.json")
			input.SelectedFiles = []string{"one.json"}
			remote.afterDownload = func(f *cpaRemoteFixture, _ string) {
				switch change {
				case "disabled":
					f.files[0].Disabled = true
				case "error":
					f.files[0].Status = "error"
				case "removed":
					f.files = []cpasync.File{}
				}
			}
			result, err := svc.Sync(context.Background(), input)
			require.NoError(t, err)
			require.Equal(t, 1, result.Skipped)
			require.Empty(t, admin.repo.accounts)
		})
	}
}

func TestCPASyncFailuresArePartialAndRedacted(t *testing.T) {
	svc, admin, remote, input, _ := newCPASyncFixture(t)
	for _, name := range []string{"good.json", "download.json", "invalid.json"} {
		addCPAFixtureFile(remote, name)
	}
	remote.downloadStatus["download.json"] = 401
	remote.credentials["invalid.json"]["disabled"] = true
	input.SelectedFiles = []string{"good.json", "download.json", "invalid.json"}
	result, err := svc.Sync(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 1, result.Created)
	require.Equal(t, 1, result.Failed)
	require.Equal(t, 1, result.Skipped)
	require.Len(t, admin.repo.accounts, 1)
	raw, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "fixture-password")
	require.NotContains(t, string(raw), "old-access")
	require.NotContains(t, string(raw), "refresh")
	admin.saveErr = errors.New("database password=secret")
	input.SelectedFiles = []string{"good.json"}
	result, err = svc.Sync(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 1, result.Failed)
	require.Empty(t, result.Items[0].Message)
}

func TestCPASyncPreflightStopsAllWrites(t *testing.T) {
	cases := []struct {
		name   string
		modify func(*CPASyncService, *CPASyncInput, *cpaRemoteFixture)
	}{
		{"empty selection", func(_ *CPASyncService, i *CPASyncInput, _ *cpaRemoteFixture) { i.SelectedFiles = []string{} }},
		{"duplicate selection", func(_ *CPASyncService, i *CPASyncInput, _ *cpaRemoteFixture) {
			i.SelectedFiles = []string{"one.json", "one.json"}
		}},
		{"wrong group", func(_ *CPASyncService, i *CPASyncInput, _ *cpaRemoteFixture) { i.GroupIDs = []int64{20} }},
		{"inactive group", func(_ *CPASyncService, i *CPASyncInput, _ *cpaRemoteFixture) { i.GroupIDs = []int64{30} }},
		{"missing group", func(_ *CPASyncService, i *CPASyncInput, _ *cpaRemoteFixture) { i.GroupIDs = []int64{999} }},
		{"invalid options", func(_ *CPASyncService, i *CPASyncInput, _ *cpaRemoteFixture) {
			i.OAuthOptions.WSMode = cpaStringPtr("magic")
		}},
		{"wrong platform options", func(_ *CPASyncService, i *CPASyncInput, _ *cpaRemoteFixture) {
			i.OAuthOptions.InterceptWarmup = cpaBoolPtr(true)
		}},
		{"app server dependency", func(_ *CPASyncService, i *CPASyncInput, _ *cpaRemoteFixture) {
			i.OAuthOptions.AllowAppServer = cpaBoolPtr(true)
		}},
		{"forbidden private url", func(s *CPASyncService, _ *CPASyncInput, _ *cpaRemoteFixture) {
			s.cfg.Security.URLAllowlist.AllowPrivateHosts = false
		}},
		{"URL userinfo", func(_ *CPASyncService, i *CPASyncInput, _ *cpaRemoteFixture) {
			i.BaseURL = "https://user:secret@cpa.example"
		}},
		{"allowlist", func(s *CPASyncService, _ *CPASyncInput, _ *cpaRemoteFixture) {
			s.cfg.Security.URLAllowlist.Enabled = true
			s.cfg.Security.URLAllowlist.CRSHosts = []string{"cpa.example"}
		}},
		{"remote list failure", func(_ *CPASyncService, _ *CPASyncInput, f *cpaRemoteFixture) { f.listStatus = 503 }},
		{"recheck failure", func(_ *CPASyncService, _ *CPASyncInput, f *cpaRemoteFixture) { f.failListAt = 2 }},
		{"local lookup failure", func(s *CPASyncService, _ *CPASyncInput, _ *cpaRemoteFixture) {
			s.repo.(*cpaSyncTestRepo).lookupErr = errors.New("db error")
		}},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			svc, admin, remote, input, _ := newCPASyncFixture(t)
			addCPAFixtureFile(remote, "one.json")
			input.SelectedFiles = []string{"one.json"}
			tt.modify(svc, &input, remote)
			result, err := svc.Sync(context.Background(), input)
			require.Error(t, err)
			require.Nil(t, result)
			require.Empty(t, admin.repo.accounts)
			require.NotContains(t, err.Error(), "secret")
			require.NotContains(t, err.Error(), "fixture-password")
		})
	}
}

func TestCPASyncAllowsActiveCompositeTargetGroup(t *testing.T) {
	svc, admin, remote, input, _ := newCPASyncFixture(t)
	admin.groups[99] = &Group{ID: 99, Platform: PlatformComposite, Status: StatusActive}
	addCPAFixtureFile(remote, "one.json")
	input.SelectedFiles = []string{"one.json"}
	input.GroupIDs = []int64{99}

	result, err := svc.Sync(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 1, result.Created)
	require.Equal(t, []int64{99}, admin.repo.accounts[1].GroupIDs)
}

func TestCPASyncBoundedWorkersAndConcurrency(t *testing.T) {
	svc, admin, remote, input, _ := newCPASyncFixture(t)
	remote.delay = 10 * time.Millisecond
	for n := 0; n < 12; n++ {
		name := fmt.Sprintf("%02d.json", n)
		addCPAFixtureFile(remote, name)
		input.SelectedFiles = append(input.SelectedFiles, name)
	}
	svc.mu.Lock()
	_, err := svc.Sync(context.Background(), input)
	svc.mu.Unlock()
	require.Error(t, err)
	require.Empty(t, remote.downloads)
	result, err := svc.Sync(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 12, result.Created)
	require.LessOrEqual(t, remote.maxActive, 4)
	require.Greater(t, remote.maxActive, 1)
	require.Len(t, admin.repo.accounts, 12)
	got := []string{}
	for _, item := range result.Items {
		got = append(got, item.FileName)
	}
	require.True(t, sort.StringsAreSorted(got))
}

func TestCPASyncIdentityConflictAndDuplicateCopyMetadata(t *testing.T) {
	svc, admin, remote, input, _ := newCPASyncFixture(t)
	addCPAFixtureFile(remote, "one.json")
	input.SelectedFiles = []string{"one.json"}
	_, err := svc.Sync(context.Background(), input)
	require.NoError(t, err)
	remote.credentials["one.json"]["chatgpt_user_id"] = "different-member"
	result, err := svc.Sync(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, 1, result.Failed)
	require.Equal(t, "identity_changed", result.Items[0].Reason)
	require.Equal(t, "one.json", admin.repo.accounts[1].GetCredential("chatgpt_user_id"))
	for _, key := range []string{cpaSyncSourceKey, cpaSyncFileKey, cpaSyncedAtKey} {
		require.Contains(t, duplicateAccountDiscardedExtraKeys, key)
	}
	source := admin.repo.accounts[1].Extra[cpaSyncSourceKey]
	admin.repo.accounts[2] = &Account{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{cpaSyncSourceKey: source, cpaSyncFileKey: "one.json"}}
	_, err = svc.Preview(context.Background(), input.CPAConnectionInput)
	require.Error(t, err)
}

func TestCPASyncDatabaseLockFailsClosed(t *testing.T) {
	for _, mode := range []string{"busy", "error", "acquired"} {
		t.Run(mode, func(t *testing.T) {
			svc, admin, remote, input, _ := newCPASyncFixture(t)
			addCPAFixtureFile(remote, "one.json")
			input.SelectedFiles = []string{"one.json"}
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			defer func() { _ = db.Close() }()
			svc.db = db
			endpoint, err := cpasync.Endpoint(input.BaseURL)
			require.NoError(t, err)
			source := fmt.Sprintf("%x", sha256.Sum256([]byte(endpoint)))
			lockID := hashAdvisoryLockID("cpa-sync:" + source)
			expect := mock.ExpectQuery("SELECT pg_try_advisory_lock").WithArgs(driver.Value(lockID))
			if mode == "error" {
				expect.WillReturnError(errors.New("db failed"))
			} else {
				expect.WillReturnRows(sqlmock.NewRows([]string{"acquired"}).AddRow(mode == "acquired"))
			}
			if mode == "acquired" {
				mock.ExpectExec("SELECT pg_advisory_unlock").WithArgs(driver.Value(lockID)).WillReturnResult(sqlmock.NewResult(0, 1))
			}
			result, err := svc.Sync(context.Background(), input)
			if mode == "acquired" {
				require.NoError(t, err)
				require.Equal(t, 1, result.Created)
			} else {
				require.Error(t, err)
				require.Empty(t, admin.repo.accounts)
				require.Empty(t, remote.downloads)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

package service

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type batchModelsRepoStub struct {
	AccountRepository
	mu       sync.Mutex
	accounts map[int64]*Account
	writes   map[int64][]string
	writeErr error
}

func (r *batchModelsRepoStub) GetByID(_ context.Context, id int64) (*Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if account := r.accounts[id]; account != nil {
		copy := *account
		return &copy, nil
	}
	return nil, ErrAccountNotFound
}

func (r *batchModelsRepoStub) MergeUpstreamModelMapping(_ context.Context, account *Account, models []string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.writeErr != nil {
		return 0, r.writeErr
	}
	r.writes[account.ID] = models
	return len(models), nil
}

type batchModelsHTTPStub struct {
	active atomic.Int32
	peak   atomic.Int32
	delay  time.Duration
}

func (u *batchModelsHTTPStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	active := u.active.Add(1)
	defer u.active.Add(-1)
	for peak := u.peak.Load(); active > peak; peak = u.peak.Load() {
		if u.peak.CompareAndSwap(peak, active) {
			break
		}
	}
	select {
	case <-req.Context().Done():
		return nil, req.Context().Err()
	case <-time.After(u.delay):
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"new-model"},{"id":"new-model"}]}`))}, nil
}

func (u *batchModelsHTTPStub) DoWithTLS(req *http.Request, proxyURL string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, id, concurrency)
}

func TestBatchSyncUpstreamModelsMixedAccounts(t *testing.T) {
	parent := int64(1)
	repo := &batchModelsRepoStub{accounts: map[int64]*Account{
		1: {ID: 1, Name: "oauth", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"access_token": "secret"}},
		2: {ID: 2, Name: "key", Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "key"}},
		3: {ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey},
		4: {ID: 4, Platform: PlatformAnthropic, Type: AccountTypeServiceAccount},
		5: {ID: 5, Platform: PlatformOpenAI, Type: AccountTypeOAuth, ParentAccountID: &parent},
	}, writes: make(map[int64][]string)}
	svc := &AccountTestService{
		accountRepo: repo, httpUpstream: &batchModelsHTTPStub{}, cfg: upstreamModelSyncTestConfig(),
		openAIModelsManifest: &openAIModelsManifestFetcherStub{manifest: &CodexModelsManifest{Body: []byte(`{"models":[{"slug":"oauth-model"}]}`)}},
	}
	result, err := svc.BatchSyncUpstreamModels(context.Background(), []int64{1, 2, 3, 4, 5, 99, 1})
	require.NoError(t, err)
	require.Equal(t, 6, result.Total)
	require.Equal(t, 2, result.Success)
	require.Equal(t, 2, result.Failed)
	require.Equal(t, 2, result.Skipped)
	require.Equal(t, []string{"oauth-model"}, repo.writes[1])
	require.Equal(t, []string{"new-model"}, repo.writes[2])
	require.Len(t, repo.writes, 2)
	for i, id := range []int64{1, 2, 3, 4, 5, 99} {
		require.Equal(t, id, result.Results[i].AccountID)
	}
}

func TestBatchSyncUpstreamModelsBoundsConcurrency(t *testing.T) {
	repo := &batchModelsRepoStub{accounts: make(map[int64]*Account), writes: make(map[int64][]string)}
	ids := make([]int64, 12)
	for i := range ids {
		id := int64(i + 1)
		ids[i] = id
		repo.accounts[id] = &Account{ID: id, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "key"}}
	}
	upstream := &batchModelsHTTPStub{delay: 10 * time.Millisecond}
	svc := &AccountTestService{accountRepo: repo, httpUpstream: upstream, cfg: upstreamModelSyncTestConfig()}
	result, err := svc.BatchSyncUpstreamModels(context.Background(), ids)
	require.NoError(t, err)
	require.Equal(t, 12, result.Success)
	require.LessOrEqual(t, upstream.peak.Load(), int32(upstreamModelSyncConcurrency))
	require.Greater(t, upstream.peak.Load(), int32(1))
}

func TestBatchSyncUpstreamModelsCanceledDoesNotWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := &batchModelsRepoStub{writes: make(map[int64][]string)}
	svc := &AccountTestService{accountRepo: repo}
	result, err := svc.BatchSyncUpstreamModels(ctx, []int64{1, 2})
	require.NoError(t, err)
	require.Equal(t, 2, result.Failed)
	require.Empty(t, repo.writes)
	for _, item := range result.Results {
		require.Equal(t, "canceled", item.ErrorKind)
	}
}

func TestBatchSyncUpstreamModelsPersistenceErrorsAreSafe(t *testing.T) {
	for _, test := range []struct {
		err  error
		kind string
	}{
		{errors.New("database SECRET"), "persistence"},
		{ErrUpstreamModelSyncAccountChanged, "account_changed"},
	} {
		t.Run(test.kind, func(t *testing.T) {
			repo := &batchModelsRepoStub{accounts: map[int64]*Account{1: {ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "key"}}}, writeErr: test.err}
			svc := &AccountTestService{accountRepo: repo, httpUpstream: &batchModelsHTTPStub{}, cfg: upstreamModelSyncTestConfig()}
			result, err := svc.BatchSyncUpstreamModels(context.Background(), []int64{1})
			require.NoError(t, err)
			require.Equal(t, 1, result.Failed)
			require.Equal(t, test.kind, result.Results[0].ErrorKind)
			require.NotContains(t, result.Results[0].Error, "SECRET")
		})
	}
}

func TestBatchSyncUpstreamModelsRejectsInvalidIDs(t *testing.T) {
	svc := &AccountTestService{}
	for _, ids := range [][]int64{nil, {0}, {-1}, make([]int64, 101)} {
		_, err := svc.BatchSyncUpstreamModels(context.Background(), ids)
		require.Error(t, err)
	}
}

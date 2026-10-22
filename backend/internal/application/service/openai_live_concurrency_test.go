package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

type liveConcurrencyRecordingCache struct {
	schedulerTestConcurrencyCache

	mu            sync.Mutex
	acquireResult bool
	acquireErr    error
	acquireCalls  int
	releaseCalls  int
}

func (c *liveConcurrencyRecordingCache) GetUsersLoadBatch(
	context.Context,
	[]UserWithConcurrency,
) (map[int64]*UserLoadInfo, error) {
	return map[int64]*UserLoadInfo{}, nil
}

func (c *liveConcurrencyRecordingCache) AcquireLiveLease(
	context.Context,
	int64,
	int,
	int64,
	int,
	int64,
	int,
	string,
	LiveConcurrencyReplacements,
) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.acquireCalls++
	return c.acquireResult, c.acquireErr
}

func (c *liveConcurrencyRecordingCache) RefreshLiveLease(
	context.Context,
	int64,
	int64,
	int64,
	string,
) (bool, error) {
	return true, nil
}

func (c *liveConcurrencyRecordingCache) ReleaseLiveLease(
	context.Context,
	int64,
	int64,
	int64,
	string,
) error {
	c.mu.Lock()
	c.releaseCalls++
	c.mu.Unlock()
	return nil
}

func (c *liveConcurrencyRecordingCache) callCounts() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.acquireCalls, c.releaseCalls
}

func requireStandaloneLiveCounts(
	t *testing.T,
	svc *ConcurrencyService,
	userID, apiKeyID int64,
	want int,
) {
	t.Helper()
	users, err := svc.GetUsersLoadBatch(context.Background(), []UserWithConcurrency{{
		ID:             userID,
		MaxConcurrency: 1,
	}})
	require.NoError(t, err)
	require.Equal(t, want, users[userID].CurrentConcurrency)
	apiKeys, err := svc.GetAPIKeyConcurrencyBatch(context.Background(), []int64{apiKeyID})
	require.NoError(t, err)
	require.Equal(t, want, apiKeys[apiKeyID])
}

func TestStandaloneLiveLeaseSharesUserAndAPIKeyLimits(t *testing.T) {
	cache := &liveConcurrencyRecordingCache{acquireResult: true}
	svc := NewConcurrencyService(cache)
	svc.SetStandaloneRequestSlots(true)
	ctx := context.Background()

	ordinaryUser, err := svc.AcquireUserSlot(ctx, 20, 1)
	require.NoError(t, err)
	require.True(t, ordinaryUser.Acquired)
	ordinaryAPIKey, err := svc.AcquireAPIKeySlot(ctx, 30, 1)
	require.NoError(t, err)
	require.True(t, ordinaryAPIKey.Acquired)

	acquired, err := svc.AcquireLiveLease(
		ctx, 10, 1, 20, 1, 30, 1, "blocked-live", LiveConcurrencyReplacements{},
	)
	require.NoError(t, err)
	require.False(t, acquired)
	acquireCalls, _ := cache.callCounts()
	require.Zero(t, acquireCalls, "a local limit rejection must not reach Redis")

	ordinaryUser.ReleaseFunc()
	ordinaryAPIKey.ReleaseFunc()
	acquired, err = svc.AcquireLiveLease(
		ctx, 10, 1, 20, 1, 30, 1, "active-live", LiveConcurrencyReplacements{},
	)
	require.NoError(t, err)
	require.True(t, acquired)
	requireStandaloneLiveCounts(t, svc, 20, 30, 1)

	blockedUser, err := svc.AcquireUserSlot(ctx, 20, 1)
	require.NoError(t, err)
	require.False(t, blockedUser.Acquired)
	blockedAPIKey, err := svc.AcquireAPIKeySlot(ctx, 30, 1)
	require.NoError(t, err)
	require.False(t, blockedAPIKey.Acquired)

	require.NoError(t, svc.ReleaseLiveLease(ctx, 10, 20, 30, "active-live"))
	requireStandaloneLiveCounts(t, svc, 20, 30, 0)
}

func TestStandaloneLiveConversionLeavesOneLongLivedSlot(t *testing.T) {
	cache := &liveConcurrencyRecordingCache{acquireResult: true}
	svc := NewConcurrencyService(cache)
	svc.SetStandaloneRequestSlots(true)
	ctx := context.Background()

	temporaryUser, err := svc.AcquireUserSlot(ctx, 20, 1)
	require.NoError(t, err)
	require.True(t, temporaryUser.Acquired)
	temporaryAPIKey, err := svc.AcquireAPIKeySlot(ctx, 30, 1)
	require.NoError(t, err)
	require.True(t, temporaryAPIKey.Acquired)

	acquired, err := svc.AcquireLiveLease(ctx, 10, 1, 20, 1, 30, 1, "converted-live", LiveConcurrencyReplacements{
		Account: true,
		User:    true,
		APIKey:  true,
	})
	require.NoError(t, err)
	require.True(t, acquired)
	requireStandaloneLiveCounts(t, svc, 20, 30, 2)

	temporaryUser.ReleaseFunc()
	temporaryAPIKey.ReleaseFunc()
	requireStandaloneLiveCounts(t, svc, 20, 30, 1)
	require.NoError(t, svc.ReleaseLiveLease(ctx, 10, 20, 30, "converted-live"))
	requireStandaloneLiveCounts(t, svc, 20, 30, 0)
}

func TestStandaloneLiveAcquireFailureRollsBackLocalReservation(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		result bool
		err    error
	}{
		{name: "redis rejection"},
		{name: "redis error", err: errors.New("redis unavailable")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cache := &liveConcurrencyRecordingCache{
				acquireResult: testCase.result,
				acquireErr:    testCase.err,
			}
			svc := NewConcurrencyService(cache)
			svc.SetStandaloneRequestSlots(true)

			acquired, err := svc.AcquireLiveLease(
				context.Background(), 10, 1, 20, 1, 30, 1, "failed-live", LiveConcurrencyReplacements{},
			)
			if testCase.err != nil {
				require.ErrorIs(t, err, testCase.err)
			} else {
				require.NoError(t, err)
			}
			require.False(t, acquired)
			requireStandaloneLiveCounts(t, svc, 20, 30, 0)
			_, found := svc.localLiveLeases.Load("failed-live")
			require.False(t, found)
		})
	}
}

func TestCreateLiveCallUpstreamFailureRollsBackStandaloneLease(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	defer resetOpenAIAdvancedSchedulerSettingCacheForTest()

	var acquiredAccounts []int64
	var releasedAccounts []int64
	cache := &liveConcurrencyRecordingCache{
		schedulerTestConcurrencyCache: schedulerTestConcurrencyCache{
			acquiredIDs: &acquiredAccounts,
			releasedIDs: &releasedAccounts,
		},
		acquireResult: true,
	}
	concurrency := NewConcurrencyService(cache)
	concurrency.SetStandaloneRequestSlots(true)
	temporaryUser, err := concurrency.AcquireUserSlot(context.Background(), 20, 1)
	require.NoError(t, err)
	temporaryAPIKey, err := concurrency.AcquireAPIKeySlot(context.Background(), 30, 1)
	require.NoError(t, err)

	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	cfg.JWT.Secret = "live-concurrency-test-secret"
	store := &liveTestStore{GatewayCache: &schedulerTestGatewayCache{}}
	account := Account{
		ID:          10,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeOAuth,
		Status:      StatusActive,
		Schedulable: true,
		Concurrency: 1,
		Credentials: map[string]any{
			"access_token":       "test-access-token",
			"chatgpt_account_id": "acct_test",
		},
	}
	upstreamErr := errors.New("upstream connection failed")
	proofs := &accountProofRecorder{}
	svc := &OpenAIGatewayService{
		accountRepo:           schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
		cache:                 store,
		cfg:                   cfg,
		concurrencyService:    concurrency,
		httpUpstream:          &liveHTTPUpstreamStub{err: upstreamErr},
		liveAttestation:       scopedProofFixture{recorder: proofs},
		liveAttestationCipher: newLiveAttestationCipher(cfg),
	}

	created, err := svc.CreateLiveCall(
		context.Background(),
		&LiveCallRequest{SDP: "v=0\r\n", Session: json.RawMessage(`{"model":"gpt-live-test"}`)},
		LiveCallIdentity{UserID: 20, APIKeyID: 30},
		1,
		1,
	)
	require.ErrorIs(t, err, upstreamErr)
	require.Nil(t, created)
	require.Equal(t, []int64{10}, acquiredAccounts)
	require.Equal(t, []int64{10}, releasedAccounts)
	acquireCalls, releaseCalls := cache.callCounts()
	require.Equal(t, 1, acquireCalls)
	require.Equal(t, 1, releaseCalls)
	require.Len(t, proofs.calls, 1)
	require.Equal(t, "live", proofs.calls[0].kind)
	require.Equal(t, resolveConvergedInstallationID(&account), proofs.calls[0].scope.InstallationID)
	require.Equal(t, openAIIntegrityKey(&account, &account, "https://chatgpt.com"), proofs.calls[0].scope.Key)
	requireStandaloneLiveCounts(t, concurrency, 20, 30, 1)

	temporaryUser.ReleaseFunc()
	temporaryAPIKey.ReleaseFunc()
	requireStandaloneLiveCounts(t, concurrency, 20, 30, 0)
}

package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestCodexVoiceMinuteCost(t *testing.T) {
	require.True(t, BillingModeLive.IsValidUsageFilter())
	require.False(t, BillingModeLive.IsValid(), "voice billing must not become token/channel pricing")
	for _, tc := range []struct {
		ms                        int64
		multiplier, total, actual float64
	}{
		{60000, 1, .05, .05}, {30000, 1, .025, .025}, {90000, 2, .075, .15},
		{1, 1, .00000083, .00000083}, {60000, 0, .05, 0},
		{-1, 1, 0, 0}, {60000, -1, .05, 0}, {60000, math.NaN(), .05, 0},
	} {
		cost := liveMinuteCost(tc.ms, tc.multiplier)
		require.Equal(t, tc.total, cost.TotalCost)
		require.Equal(t, tc.actual, cost.ActualCost)
		require.Equal(t, "live", cost.BillingMode)
	}
}

func TestCodexVoiceUpstreamUsesCLIShapeAndStableSidebandIdentity(t *testing.T) {
	upstream := &liveHTTPUpstreamStub{}
	svc := &OpenAIGatewayService{cfg: &config.Config{}, httpUpstream: upstream}
	account := &Account{ID: 7, Type: AccountTypeOAuth, Platform: PlatformOpenAI,
		Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "acct_test"}}
	session := json.RawMessage(`{"model":"gpt-live-1-codex","instructions":"你好","audio":{"output":{"voice":"blueberry"}},"delegation":{"type":"client"},"initial_items":[{"type":"message","role":"user","content":[{"type":"input_text","text":"continue"}]}]}`)
	require.True(t, IsCodexVoiceSession(session))
	require.False(t, IsCodexVoiceSession(json.RawMessage(`{"model":"gpt-live-1-codex-untrusted"}`)))
	created, err := svc.createUpstreamLiveCall(context.Background(), account, &LiveCallRequest{SDP: "v=0\r\n", Session: session}, "")
	require.NoError(t, err)
	var body struct {
		Session json.RawMessage `json:"session"`
	}
	require.NoError(t, json.Unmarshal(upstream.body, &body))
	require.JSONEq(t, string(session), string(body.Session))
	require.Empty(t, upstream.request.Header.Get(liveAttestationHeader))
	require.Equal(t, "quicksilver=v2", upstream.request.Header.Get("OpenAI-Alpha"))
	require.NotEmpty(t, created.SessionID)
	record := &LiveCallRecord{CodexVoice: true, SessionID: created.SessionID, ThreadID: created.ThreadID}
	headers, err := svc.liveSidebandHeaders(context.Background(), account, record)
	require.NoError(t, err)
	require.Equal(t, upstream.request.Header.Get("session-id"), headers.Get("session-id"))
	require.Equal(t, created.SessionID, headers.Get("X-Session-Id"))
	require.Equal(t, created.ThreadID, headers.Get("thread-id"))
	require.Empty(t, headers.Get(liveAttestationHeader))
	_, err = svc.liveSidebandHeaders(context.Background(), account, &LiveCallRecord{})
	require.Error(t, err, "legacy Desktop calls must still require their own device proof")
}

func TestCodexVoiceCreationReachesUpstreamWithoutDesktopProof(t *testing.T) {
	cache := &liveConcurrencyRecordingCache{acquireResult: true}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	account := Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 1,
		Credentials: map[string]any{"access_token": "test-token", "chatgpt_account_id": "acct_test"}}
	upstreamErr := errors.New("mock upstream reached")
	svc := &OpenAIGatewayService{
		accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
		cache:       &liveTestStore{GatewayCache: &schedulerTestGatewayCache{}}, cfg: cfg,
		concurrencyService: NewConcurrencyService(cache), httpUpstream: &liveHTTPUpstreamStub{err: upstreamErr},
		liveAttestation: liveAttestationStub{err: errors.New("Desktop proof unavailable on this host")},
	}
	_, err := svc.CreateLiveCall(context.Background(), &LiveCallRequest{SDP: "v=0\r\n", Session: json.RawMessage(`{"model":"gpt-live-1-codex"}`)}, LiveCallIdentity{UserID: 20, APIKeyID: 30}, 1, 1)
	require.ErrorIs(t, err, upstreamErr)
	acquired, released := cache.callCounts()
	require.Equal(t, 1, acquired)
	require.Equal(t, 1, released)
	_, err = svc.CreateLiveCall(context.Background(), &LiveCallRequest{SDP: "v=0\r\n", Session: json.RawMessage(`{"model":"gpt-live-test"}`)}, LiveCallIdentity{UserID: 20, APIKeyID: 30}, 1, 1)
	var proofErr *LiveAttestationUnavailableError
	require.ErrorAs(t, err, &proofErr)
}

type liveBillingTestStore struct {
	*liveTestStore
	billed bool
	err    error
}

func (s *liveBillingTestStore) PrepareLiveCallSettlement(context.Context, string, time.Time) (*LiveCallRecord, error) {
	if s.billed {
		return nil, ErrLiveCallNotFound
	}
	return s.record, nil
}
func (s *liveBillingTestStore) ListDueLiveCalls(context.Context, time.Time, int) ([]*LiveCallRecord, error) {
	if s.billed {
		return nil, nil
	}
	return []*LiveCallRecord{s.record}, nil
}
func (s *liveBillingTestStore) MarkLiveCallBilled(context.Context, string, time.Duration) error {
	if s.err != nil {
		return s.err
	}
	s.billed = true
	return nil
}

func newBillableLiveTestRecord() *LiveCallRecord {
	return &LiveCallRecord{
		CallID: "rtc_voice", CallHash: hashLiveCallID("rtc_voice"), AccountID: 11, APIKeyID: 22,
		UserID: 33, GroupID: 44, LeaseID: "voice-lease", Model: CodexVoiceModel,
		CreatedAt: time.Unix(1000, 0), ExpiresAt: time.Unix(1060, 0), Controller: LiveControllerPending,
		BillingDurationMs: 60000, Billing: &LiveBillingSnapshot{RateMultiplier: 2, AccountRateMultiplier: 1,
			APIKeyQuota: true, APIKeyRateLimit: true, UserPlatformQuota: true, APIKeyAuthCacheKey: "hashed-api-key"},
	}
}

func TestCodexVoiceSettlementRecoversWithoutDoubleCharge(t *testing.T) {
	for _, subscription := range []bool{false, true} {
		t.Run(map[bool]string{false: "balance", true: "subscription"}[subscription], func(t *testing.T) {
			record := newBillableLiveTestRecord()
			if subscription {
				record.SubscriptionID = 55
			}
			store := &liveBillingTestStore{liveTestStore: &liveTestStore{}}
			require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
			usage := &openAIRecordUsageLogRepoStub{inserted: true}
			billing := &openAIRecordUsageBillingRepoStub{err: errors.New("database temporarily unavailable"), result: &UsageBillingApplyResult{Applied: false}}
			svc := &OpenAIGatewayService{cache: store, concurrencyService: NewConcurrencyService(&liveTestConcurrencyCache{}), usageLogRepo: usage, usageBillingRepo: billing}
			svc.finalizeLiveCall(record)
			require.False(t, store.billed)
			require.Zero(t, usage.calls)
			first := *billing.lastCmd
			billing.err = nil
			svc.recoverLiveBilling(context.Background())
			require.True(t, store.billed)
			require.Equal(t, first.RequestFingerprint, billing.lastCmd.RequestFingerprint)
			require.Equal(t, .05, usage.lastLog.TotalCost)
			require.Equal(t, .1, usage.lastLog.ActualCost)
			require.Equal(t, int64(60000), int64(*usage.lastLog.DurationMs))
			require.Equal(t, .1, billing.lastCmd.APIKeyQuotaCost)
			require.Equal(t, .1, billing.lastCmd.APIKeyRateLimitCost)
			require.Equal(t, "hashed-api-key", billing.lastCmd.APIKeyAuthCacheKey)
			if subscription {
				require.Equal(t, .1, billing.lastCmd.SubscriptionCost)
				require.Zero(t, billing.lastCmd.BalanceCost)
				require.Zero(t, billing.lastCmd.UserPlatformQuotaCost)
			} else {
				require.Equal(t, .1, billing.lastCmd.BalanceCost)
				require.Equal(t, .1, billing.lastCmd.UserPlatformQuotaCost)
			}
			svc.finalizeLiveCall(record)
			svc.recoverLiveBilling(context.Background())
			require.Equal(t, 2, billing.calls, "one failed attempt and one retry")
			require.Equal(t, 1, usage.calls)
		})
	}
}

func TestCodexVoiceSettlementRetriesUsagePersistenceAfterBilling(t *testing.T) {
	record := newBillableLiveTestRecord()
	store := &liveBillingTestStore{liveTestStore: &liveTestStore{}}
	require.NoError(t, store.SaveLiveCall(context.Background(), record, time.Hour))
	usage := &openAIRecordUsageLogRepoStub{err: errors.New("usage write temporarily unavailable")}
	billing := &openAIRecordUsageBillingRepoStub{result: &UsageBillingApplyResult{Applied: false}}
	svc := &OpenAIGatewayService{cache: store, concurrencyService: NewConcurrencyService(&liveTestConcurrencyCache{}), usageLogRepo: usage, usageBillingRepo: billing}
	svc.finalizeLiveCall(record)
	require.False(t, store.billed)
	first := *billing.lastCmd
	usage.err = nil
	svc.recoverLiveBilling(context.Background())
	require.True(t, store.billed)
	require.Equal(t, first.RequestFingerprint, billing.lastCmd.RequestFingerprint)
}

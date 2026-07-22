package service

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type openAIStreamProbeRaceConcurrencyCache struct {
	ConcurrencyCache
	onAcquire func(accountID int64)
}

func (c *openAIStreamProbeRaceConcurrencyCache) AcquireAccountSlot(_ context.Context, accountID int64, _ int, _ string) (bool, error) {
	if c.onAcquire != nil {
		c.onAcquire(accountID)
	}
	return true, nil
}

func (c *openAIStreamProbeRaceConcurrencyCache) ReleaseAccountSlot(context.Context, int64, string) error {
	return nil
}

func TestOpenAIStreamDegradationBackoffAndSingleProbeClaim(t *testing.T) {
	state := newOpenAIStreamDegradationState()
	accountID := int64(43)
	now := time.Date(2026, 7, 23, 10, 0, 0, 0, time.UTC)

	first := state.recordTimeout(accountID, now)
	require.Equal(t, 1, first.Level)
	require.Equal(t, now.Add(20*time.Second), first.NextProbeAt)
	_, degraded, due := state.snapshot(accountID, now.Add(19*time.Second))
	require.True(t, degraded)
	require.False(t, due)

	probeAt := now.Add(20 * time.Second)
	var claims atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if state.claimProbe(accountID, probeAt, 25*time.Second) {
				claims.Add(1)
			}
		}()
	}
	wg.Wait()
	require.Equal(t, int32(1), claims.Load())
	_, _, due = state.snapshot(accountID, probeAt)
	require.False(t, due, "an in-flight probe must not remain top-ranked")

	second := state.recordTimeout(accountID, probeAt.Add(time.Second))
	require.Equal(t, 2, second.Level)
	require.Equal(t, probeAt.Add(41*time.Second), second.NextProbeAt)
}

func TestOpenAIStreamDegradationHealthTierOverridesConfiguredPriority(t *testing.T) {
	healthy := openAIAccountCandidateScore{
		account:  &Account{ID: 1, Priority: 100},
		loadInfo: &AccountLoadInfo{},
	}
	degraded := openAIAccountCandidateScore{
		account:        &Account{ID: 2, Priority: -100},
		loadInfo:       &AccountLoadInfo{},
		streamTier:     openAIStreamCandidateTierDegraded,
		streamDegraded: true,
	}

	require.True(t, isOpenAIAccountCandidateBetter(healthy, degraded))
	require.False(t, isOpenAIAccountCandidateBetter(degraded, healthy))
}

func TestOpenAIStreamRecoveryProbeThenHealthyTraffic(t *testing.T) {
	degraded := &Account{ID: 10, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	healthy := &Account{ID: 11, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	svc := &OpenAIGatewayService{openaiStreamDegradation: newOpenAIStreamDegradationState()}
	svc.recordOpenAIStreamResponseHeaderTimeout(degraded.ID, time.Now().Add(-21*time.Second))
	scheduler := &defaultOpenAIAccountScheduler{service: svc}
	req := OpenAIAccountScheduleRequest{Platform: PlatformOpenAI, IsStreaming: true}

	buildOrder := func() []openAIAccountCandidateScore {
		candidates := make([]openAIAccountCandidateScore, 0, 2)
		for _, account := range []*Account{degraded, healthy} {
			tier, isDegraded := svc.openAIStreamCandidateTier(account.ID, time.Now())
			candidates = append(candidates, openAIAccountCandidateScore{
				account:        account,
				loadInfo:       &AccountLoadInfo{AccountID: account.ID},
				streamTier:     tier,
				streamDegraded: isDegraded,
			})
		}
		return selectTopKOpenAICandidates(candidates, len(candidates))
	}

	probe, _, err := scheduler.tryAcquireOpenAISelectionOrder(context.Background(), req, buildOrder())
	require.NoError(t, err)
	require.NotNil(t, probe)
	require.Equal(t, degraded.ID, probe.Account.ID, "due recovery probe should get one request")
	probe.ReleaseFunc()

	next, _, err := scheduler.tryAcquireOpenAISelectionOrder(context.Background(), req, buildOrder())
	require.NoError(t, err)
	require.NotNil(t, next)
	require.Equal(t, healthy.ID, next.Account.ID, "traffic after the claimed probe should return to healthy accounts")
	next.ReleaseFunc()
}

func TestOpenAIStreamStaleProbeOrderFallsBackToHealthyAccount(t *testing.T) {
	degraded := &Account{ID: 12, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	healthy := &Account{ID: 13, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	svc := &OpenAIGatewayService{openaiStreamDegradation: newOpenAIStreamDegradationState()}
	svc.recordOpenAIStreamResponseHeaderTimeout(degraded.ID, time.Now().Add(-21*time.Second))

	tier, isDegraded := svc.openAIStreamCandidateTier(degraded.ID, time.Now())
	require.Equal(t, openAIStreamCandidateTierProbe, tier)
	staleOrder := []openAIAccountCandidateScore{
		{account: degraded, loadInfo: &AccountLoadInfo{AccountID: degraded.ID}, streamTier: tier, streamDegraded: isDegraded},
		{account: healthy, loadInfo: &AccountLoadInfo{AccountID: healthy.ID}, streamTier: openAIStreamCandidateTierHealthy},
	}
	require.True(t, svc.claimOpenAIStreamRecoveryProbe(degraded.ID, time.Now()), "another request claims the probe after ordering")

	scheduler := &defaultOpenAIAccountScheduler{service: svc}
	selection, _, err := scheduler.tryAcquireOpenAISelectionOrder(context.Background(), OpenAIAccountScheduleRequest{
		Platform: PlatformOpenAI, IsStreaming: true,
	}, staleOrder)
	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, healthy.ID, selection.Account.ID)
	selection.ReleaseFunc()
}

func TestOpenAIStreamLegacySelectionRechecksAfterProbeClaimRace(t *testing.T) {
	degraded := Account{ID: 14, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: -100}
	healthy := Account{ID: 15, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 100}
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false

	var svc *OpenAIGatewayService
	var claimed bool
	var claimOnce sync.Once
	concurrencyCache := &openAIStreamProbeRaceConcurrencyCache{}
	concurrencyCache.onAcquire = func(accountID int64) {
		if accountID != degraded.ID {
			return
		}
		claimOnce.Do(func() {
			claimed = svc.claimOpenAIStreamRecoveryProbe(accountID, time.Now())
		})
	}
	svc = &OpenAIGatewayService{
		accountRepo:             schedulerTestOpenAIAccountRepo{accounts: []Account{degraded, healthy}},
		cfg:                     cfg,
		concurrencyService:      NewConcurrencyService(concurrencyCache),
		openaiStreamDegradation: newOpenAIStreamDegradationState(),
	}
	svc.recordOpenAIStreamResponseHeaderTimeout(degraded.ID, time.Now().Add(-21*time.Second))

	selection, err := svc.selectAccountWithLoadAwareness(
		WithOpenAIStreamScheduling(context.Background(), true),
		nil,
		PlatformOpenAI,
		"",
		"gpt-test",
		nil,
		false,
		"",
		true,
	)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NotNil(t, selection)
	require.Equal(t, healthy.ID, selection.Account.ID)
	require.NotNil(t, selection.ReleaseFunc)
	selection.ReleaseFunc()
}

func TestOpenAIStreamSoleDegradedAccountStillSchedulesAndSuccessClears(t *testing.T) {
	account := &Account{ID: 20, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1}
	svc := &OpenAIGatewayService{openaiStreamDegradation: newOpenAIStreamDegradationState()}
	svc.recordOpenAIStreamResponseHeaderTimeout(account.ID, time.Now())
	scheduler := &defaultOpenAIAccountScheduler{service: svc}
	tier, degraded := svc.openAIStreamCandidateTier(account.ID, time.Now())

	selection, _, err := scheduler.tryAcquireOpenAISelectionOrder(context.Background(), OpenAIAccountScheduleRequest{
		Platform: PlatformOpenAI, IsStreaming: true,
	}, []openAIAccountCandidateScore{{
		account: account, loadInfo: &AccountLoadInfo{AccountID: account.ID}, streamTier: tier, streamDegraded: degraded,
	}})
	require.NoError(t, err)
	require.NotNil(t, selection, "a sole degraded account must not become a false 503")
	require.Equal(t, account.ID, selection.Account.ID)
	selection.ReleaseFunc()

	svc.ReportOpenAIAccountStreamScheduleResult(account.ID, "gpt-test", true, nil, true)
	_, stillDegraded := svc.SnapshotOpenAIStreamDegradation(account.ID)
	require.False(t, stillDegraded)
}

func TestOpenAIStreamImageSuccessDoesNotClearLLMDegradation(t *testing.T) {
	accountID := int64(21)
	svc := &OpenAIGatewayService{openaiStreamDegradation: newOpenAIStreamDegradationState()}
	svc.recordOpenAIStreamResponseHeaderTimeout(accountID, time.Now())

	svc.ReportOpenAIAccountStreamScheduleResult(accountID, "gpt-image-2", true, nil, false)

	_, stillDegraded := svc.SnapshotOpenAIStreamDegradation(accountID)
	require.True(t, stillDegraded)
}

func TestHandleOpenAIStreamHeaderTimeoutSoftDegradesWithoutRuntimeBlock(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	svc := &OpenAIGatewayService{}
	account := &Account{ID: 30, Name: "slow-stream", Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	timeoutErr := WrapOpenAIStreamResponseHeaderTimeout(errors.New("http2: timeout awaiting response headers"))

	err := svc.handleOpenAIUpstreamTransportError(context.Background(), c, account, timeoutErr, false)
	var failover *UpstreamFailoverError
	require.True(t, errors.As(err, &failover))
	require.False(t, svc.isOpenAIAccountRuntimeBlocked(account))
	snapshot, degraded := svc.SnapshotOpenAIStreamDegradation(account.ID)
	require.True(t, degraded)
	require.Equal(t, 1, snapshot.Level)
}

func TestStreamTimeoutSettingsDefaultUpgradeValidationAndImmediateRefresh(t *testing.T) {
	repo := &schedulerV2SettingRepo{values: map[string]string{
		SettingKeyStreamTimeoutSettings: `{"temp_unsched_minutes":5,"threshold_count":3,"threshold_window_minutes":10,"action":"none"}`,
	}}
	svc := NewSettingService(repo, nil)

	legacy, err := svc.GetStreamTimeoutSettings(context.Background())
	require.NoError(t, err)
	require.True(t, legacy.ResponseHeaderTimeoutDegradationEnabled, "legacy settings must enable the new feature by default")
	require.Equal(t, DefaultStreamResponseHeaderTimeoutSeconds, legacy.ResponseHeaderTimeoutSeconds)
	require.True(t, svc.IsStreamResponseHeaderTimeoutDegradationEnabled())

	invalid := *legacy
	invalid.ResponseHeaderTimeoutSeconds = 0
	require.Error(t, svc.SetStreamTimeoutSettings(context.Background(), &invalid))

	updated := *legacy
	updated.ResponseHeaderTimeoutSeconds = 37
	require.NoError(t, svc.SetStreamTimeoutSettings(context.Background(), &updated))
	require.Equal(t, 37, svc.GetStreamResponseHeaderTimeoutSeconds())

	updated.ResponseHeaderTimeoutDegradationEnabled = false
	require.NoError(t, svc.SetStreamTimeoutSettings(context.Background(), &updated))
	require.False(t, svc.IsStreamResponseHeaderTimeoutDegradationEnabled())
}

func TestOpenAIStreamDegradationSwitchClearsRuntimeState(t *testing.T) {
	repo := &schedulerV2SettingRepo{values: make(map[string]string)}
	settingService := NewSettingService(repo, nil)
	svc := &OpenAIGatewayService{
		settingService:          settingService,
		openaiStreamDegradation: newOpenAIStreamDegradationState(),
	}
	accountID := int64(44)
	svc.recordOpenAIStreamResponseHeaderTimeout(accountID, time.Now())
	_, degraded := svc.SnapshotOpenAIStreamDegradation(accountID)
	require.True(t, degraded)

	settings := DefaultStreamTimeoutSettings()
	settings.ResponseHeaderTimeoutDegradationEnabled = false
	require.NoError(t, settingService.SetStreamTimeoutSettings(context.Background(), settings))
	_, degraded = svc.SnapshotOpenAIStreamDegradation(accountID)
	require.False(t, degraded)
	_, stored, _ := svc.openaiStreamDegradation.snapshot(accountID, time.Now())
	require.False(t, stored, "disabling the feature must remove stale degradation badges")
}

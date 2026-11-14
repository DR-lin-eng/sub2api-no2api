package service

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/ctxkey"
	"github.com/stretchr/testify/require"
)

func imageSchedulingTestSettings(enabled bool) *SettingService {
	return NewSettingService(&openAIAdvancedSchedulerSettingRepoStub{values: map[string]string{
		SettingKeyImageIndependentSchedulingEnabled: strconv.FormatBool(enabled),
	}}, &config.Config{})
}

func TestImageIndependentSchedulingHealthAndRequestIsolation(t *testing.T) {
	ctx := WithImageIndependentScheduling(t.Context())
	future := time.Now().Add(time.Hour)
	past := time.Now().Add(-time.Hour)
	for _, tc := range []struct {
		name   string
		change func(*Account)
		want   bool
	}{
		{"normal", func(a *Account) { a.Schedulable = true }, true},
		{"scheduling off", func(a *Account) {}, true},
		{"error", func(a *Account) { a.Status = StatusError }, false},
		{"disabled", func(a *Account) { a.Status = StatusDisabled }, false},
		{"expired", func(a *Account) { a.ExpiresAt, a.AutoPauseOnExpired = &past, true }, false},
		{"overloaded", func(a *Account) { a.OverloadUntil = &future }, false},
		{"rate limited", func(a *Account) { a.RateLimitResetAt = &future }, false},
		{"error cooldown", func(a *Account) { a.TempUnschedulableUntil = &future }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := &Account{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive}
			tc.change(a)
			require.Equal(t, tc.want, a.IsSchedulableForModelWithContext(ctx, "gpt-image-2.6"))
			if !a.Schedulable {
				require.False(t, a.IsSchedulable(), "image selection must not change the stored switch")
				require.False(t, a.IsSchedulableForModel("gpt-6.1-sol"))
			}
		})
	}
	for _, model := range []string{"gpt-image-2.6", "gpt-image-1", "grok-imagine-image", "models/gemini-3-pro-image-preview"} {
		ctx := imageSchedulingContext(t.Context(), imageSchedulingTestSettings(true), model)
		require.True(t, ImageIndependentSchedulingFromContext(ctx), model)
	}
	require.False(t, ImageIndependentSchedulingFromContext(imageSchedulingContext(t.Context(), imageSchedulingTestSettings(false), "gpt-image-2")))
	require.False(t, ImageIndependentSchedulingFromContext(imageSchedulingContext(t.Context(), nil, "gpt-image-2")))
	require.False(t, ImageIndependentSchedulingFromContext(imageSchedulingContext(WithOpenAIImageGenerationIntent(t.Context()), imageSchedulingTestSettings(true), "gpt-6.1-sol")), "a text request advertising image tools must stay in the ordinary pool")
	imageCtx := imageSchedulingContext(t.Context(), imageSchedulingTestSettings(true), "gpt-image-2")
	textCtx := imageSchedulingContext(imageCtx, nil, "gpt-6.1-sol")
	require.False(t, ImageIndependentSchedulingFromContext(textCtx), "switching a WebSocket turn to text must restore ordinary eligibility")
	require.True(t, ImageIndependentSchedulingFromContext(imageSchedulingContext(textCtx, nil, "gpt-image-2")), "later image turns retain the connection's setting snapshot")
	limited := &Account{Status: StatusActive, Platform: PlatformOpenAI, Extra: map[string]any{
		modelRateLimitsKey: map[string]any{openAIImageGenerationRateLimitKey: map[string]any{
			"rate_limit_reset_at": future.Format(time.RFC3339),
		}},
	}}
	require.False(t, limited.IsSchedulableForModelWithContext(ctx, "gpt-image-2"), "the independent switch must not bypass image-specific rate limits")
}

func TestImageIndependentSchedulingLegacyAndAdvanced(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		for _, enabled := range []bool{false, true} {
			t.Run("advanced="+strconv.FormatBool(advanced)+"/independent="+strconv.FormatBool(enabled), func(t *testing.T) {
				resetOpenAIAdvancedSchedulerSettingCacheForTest()
				t.Cleanup(resetOpenAIAdvancedSchedulerSettingCacheForTest)
				groupID := int64(51)
				accounts := []Account{
					{ID: 1, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusError, Schedulable: true, Priority: -2},
					{ID: 2, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: false, Priority: -1},
					{ID: 3, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Priority: 1},
				}
				for i := range accounts {
					accounts[i].Concurrency = 1
					accounts[i].GroupIDs = []int64{groupID}
					accounts[i].AccountGroups = []AccountGroup{{GroupID: groupID}}
				}
				svc := &OpenAIGatewayService{
					accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
					settingService:     imageSchedulingTestSettings(enabled),
					cfg:                &config.Config{},
					cache:              &schedulerTestGatewayCache{},
					rateLimitService:   newOpenAIAdvancedSchedulerRateLimitService(strconv.FormatBool(advanced)),
					concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
				}
				selection, _, err := svc.SelectAccountWithSchedulerForImages(t.Context(), &groupID, "image-session", "gpt-image-2", nil, OpenAIImagesCapabilityBasic)
				require.NoError(t, err)
				want := int64(3)
				if enabled {
					want = 2
				}
				require.Equal(t, want, selection.Account.ID)
				selection.ReleaseFunc()
				selection, _, err = svc.SelectAccountWithSchedulerForImages(t.Context(), &groupID, "image-session", "gpt-image-2", nil, OpenAIImagesCapabilityBasic)
				require.NoError(t, err, "sticky image account remains eligible")
				require.Equal(t, want, selection.Account.ID)
				selection.ReleaseFunc()
				selection, _, err = svc.SelectAccountWithSchedulerForImages(t.Context(), &groupID, "", "gpt-image-2", map[int64]struct{}{2: {}}, OpenAIImagesCapabilityBasic)
				require.NoError(t, err)
				require.Equal(t, int64(3), selection.Account.ID, "failed image account must stay excluded")
				selection.ReleaseFunc()
				selection, _, err = svc.SelectAccountWithScheduler(t.Context(), &groupID, "", "", "gpt-6.1-sol", nil, OpenAIUpstreamTransportAny, false)
				require.NoError(t, err)
				require.Equal(t, int64(3), selection.Account.ID, "text scheduling still respects the account switch")
				selection.ReleaseFunc()
			})
		}
	}
}

func TestImageIndependentSchedulingBypassesBothSnapshotPools(t *testing.T) {
	for _, engine := range []string{SchedulerEngineLegacy, SchedulerEngineV2} {
		t.Run(engine, func(t *testing.T) {
			cache := newSchedulerV2CacheStub()
			cache.state = SchedulerEngineState{Engine: engine, Status: SchedulerEngineStatusActive}
			account := Account{ID: 8, Platform: PlatformOpenAI, Status: StatusActive}
			snapshot := &SchedulerSnapshotService{cache: cache, accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{account}}}
			ctx := WithImageIndependentScheduling(t.Context())
			accounts, _, err := snapshot.ListSchedulableAccounts(ctx, nil, PlatformOpenAI, false)
			require.NoError(t, err)
			require.Len(t, accounts, 1)
			require.Equal(t, account.ID, accounts[0].ID)
			require.Zero(t, cache.legacyReads)
			require.Zero(t, cache.candidateReads)
			require.Empty(t, cache.legacySnapshot, "independent candidates must not leak into the text pool")
			require.Empty(t, cache.candidates)
			loaded, err := snapshot.GetAccount(ctx, account.ID)
			require.NoError(t, err)
			require.NotNil(t, loaded, "hydration must not require membership in the ordinary snapshot")
		})
	}
}

func TestImageIndependentSchedulingGeminiGateway(t *testing.T) {
	ctx := context.WithValue(t.Context(), ctxkey.ForcePlatform, PlatformGemini)
	accounts := []Account{
		{ID: 1, Platform: PlatformGemini, Type: AccountTypeOAuth, Status: StatusError, Schedulable: true, Priority: -2},
		{ID: 2, Platform: PlatformGemini, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: false, Priority: -1},
		{ID: 3, Platform: PlatformGemini, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Priority: 1},
	}
	svc := &GatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		settingService:     imageSchedulingTestSettings(true),
		cfg:                &config.Config{RunMode: config.RunModeSimple},
		cache:              &schedulerTestGatewayCache{},
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	selection, err := svc.SelectAccountWithLoadAwareness(ctx, nil, "", "gemini-3-pro-image-preview", nil, "", 0)
	require.NoError(t, err)
	require.Equal(t, int64(2), selection.Account.ID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
	selection, err = svc.SelectAccountWithLoadAwareness(ctx, nil, "", "gemini-2.5-pro", nil, "", 0)
	require.NoError(t, err)
	require.Equal(t, int64(3), selection.Account.ID)
	if selection.ReleaseFunc != nil {
		selection.ReleaseFunc()
	}
}

func TestImageIndependentSchedulingWaitRechecksHealth(t *testing.T) {
	account := Account{ID: 81, Platform: PlatformOpenAI, Status: StatusActive, Schedulable: false}
	repo := schedulerTestOpenAIAccountRepo{accounts: []Account{account}}
	cache := &waitRevalidationStateCache{state: AccountSchedulingState{Exists: true, Schedulable: false}, found: true}
	snapshot := &SchedulerSnapshotService{cache: cache}
	ctx := WithImageIndependentScheduling(t.Context())
	for _, check := range []func(context.Context, *int64, string, int64) error{
		(&OpenAIGatewayService{accountRepo: repo, schedulerSnapshot: snapshot}).EnsureAccountSchedulableAfterWait,
		(&GatewayService{accountRepo: repo, schedulerSnapshot: snapshot}).EnsureAccountSchedulableAfterWait,
	} {
		repo.accounts[0].Status = StatusActive
		require.NoError(t, check(ctx, nil, "", account.ID))
		repo.accounts[0].Status = StatusError
		require.ErrorIs(t, check(ctx, nil, "", account.ID), ErrAccountSchedulingChanged)
	}
	require.Zero(t, cache.calls, "the ordinary cached boolean cannot represent independent image eligibility")
}

func TestImageIndependentSchedulingPreviousResponseBinding(t *testing.T) {
	groupID := int64(61)
	account := Account{ID: 81, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Extra: map[string]any{
		"openai_apikey_responses_websockets_v2_enabled": true,
	}}
	cfg := &config.Config{}
	cfg.Gateway.OpenAIWS.Enabled = true
	cfg.Gateway.OpenAIWS.APIKeyEnabled = true
	cfg.Gateway.OpenAIWS.ResponsesWebsocketsV2 = true
	svc := &OpenAIGatewayService{
		accountRepo: schedulerTestOpenAIAccountRepo{accounts: []Account{account}},
		cache:       &schedulerTestGatewayCache{}, cfg: cfg,
	}
	ctx := WithImageIndependentScheduling(t.Context())
	require.NoError(t, svc.getOpenAIWSStateStore().BindResponseAccount(ctx, groupID, "resp_image", account.ID, time.Hour))
	got := svc.ResolveAccountIDByPreviousResponseIDForScheduler(ctx, &groupID, "resp_image", "gpt-image-2", nil, "", false)
	require.Equal(t, account.ID, got, "healthy image continuation retains its account even when the ordinary scheduling switch is off")
	got = svc.ResolveAccountIDByPreviousResponseIDForScheduler(t.Context(), &groupID, "resp_image", "gpt-6.1-sol", nil, "", false)
	require.Zero(t, got, "text continuation still respects the ordinary switch")
}

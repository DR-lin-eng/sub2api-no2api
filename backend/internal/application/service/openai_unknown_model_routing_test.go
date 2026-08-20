//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIUnknownModelRoutingSkipsEmptyMappingOAuth(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(schedulerModeName(advanced), func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			groupID := int64(990101)
			oauth := Account{
				ID: 910001, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0,
			}
			apiKey := Account{
				ID: 910002, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 10,
			}
			svc := newUnknownModelRoutingService([]Account{oauth, apiKey}, advanced)

			selection, _, err := svc.SelectAccountWithScheduler(
				context.Background(), &groupID, "", "", "unknown-model-x", nil,
				OpenAIUpstreamTransportAny, false,
			)

			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, apiKey.ID, selection.Account.ID)
		})
	}
}

func TestOpenAIUnknownModelRoutingUsesChannelMappedModelForOAuth(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(schedulerModeName(advanced), func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			groupID := int64(990102)
			oauth := Account{
				ID: 910011, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
				Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0,
			}
			apiKey := Account{
				ID: 910012, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
				Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 10,
			}
			channel := Channel{
				ID:       9101,
				Status:   StatusActive,
				GroupIDs: []int64{groupID},
				ModelMapping: map[string]map[string]string{
					PlatformOpenAI: {"unknown-model-x": "gpt-5.4"},
				},
			}
			repo := makeStandardRepo(channel, map[int64]string{groupID: PlatformOpenAI})
			svc := newUnknownModelRoutingService([]Account{oauth, apiKey}, advanced)
			svc.channelService = newTestChannelService(repo)

			selection, _, err := svc.SelectAccountWithScheduler(
				context.Background(), &groupID, "", "", "unknown-model-x", nil,
				OpenAIUpstreamTransportAny, false,
			)

			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, oauth.ID, selection.Account.ID)
		})
	}
}

func TestOpenAIUnknownModelRoutingKeepsPassthroughOAuthEligible(t *testing.T) {
	account := &Account{
		ID:       910021,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"openai_passthrough": true},
	}

	require.True(t, account.IsModelSupportedForRequest("unknown-model-x", "unknown-model-x"))
}

func TestOpenAIUnknownModelRoutingClearsIncompatibleOAuthStickyBinding(t *testing.T) {
	resetOpenAIAdvancedSchedulerSettingCacheForTest()
	groupID := int64(990103)
	oauth := Account{
		ID: 910031, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
		Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0,
	}
	apiKey := Account{
		ID: 910032, Platform: PlatformOpenAI, Type: AccountTypeAPIKey,
		Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 10,
	}
	svc := newUnknownModelRoutingService([]Account{oauth, apiKey}, true)
	cache := &schedulerTestGatewayCache{sessionBindings: map[string]int64{"openai:unknown-sticky": oauth.ID}}
	svc.cache = cache

	selection, _, err := svc.SelectAccountWithScheduler(
		context.Background(), &groupID, "", "unknown-sticky", "unknown-model-x", nil,
		OpenAIUpstreamTransportAny, false,
	)

	require.NoError(t, err)
	require.NotNil(t, selection)
	require.Equal(t, apiKey.ID, selection.Account.ID)
	require.NotEqual(t, oauth.ID, cache.sessionBindings["openai:unknown-sticky"])
}

func TestOpenAIDiagnoseUnknownModelDoesNotCountEmptyMappingOAuth(t *testing.T) {
	groupID := int64(990104)
	repo := &mockAccountRepoForPlatform{accounts: []Account{
		{
			ID: 910041, Platform: PlatformOpenAI, Type: AccountTypeOAuth,
			Status: StatusActive, Schedulable: true, GroupIDs: []int64{groupID},
		},
	}}
	svc := &OpenAIGatewayService{accountRepo: repo, cfg: testConfig()}

	diagnosis := svc.DiagnoseModelAvailabilityForPlatform(
		context.Background(), &groupID, "unknown-model-x", PlatformOpenAI,
	)

	require.True(t, diagnosis.HasAccountsInPool)
	require.False(t, diagnosis.HasModelSupport)
}

func newUnknownModelRoutingService(accounts []Account, advanced bool) *OpenAIGatewayService {
	cfg := &config.Config{}
	cfg.Gateway.Scheduling.LoadBatchEnabled = false
	svc := &OpenAIGatewayService{
		accountRepo:        schedulerTestOpenAIAccountRepo{accounts: accounts},
		cache:              &schedulerTestGatewayCache{},
		cfg:                cfg,
		concurrencyService: NewConcurrencyService(schedulerTestConcurrencyCache{}),
	}
	if advanced {
		svc.rateLimitService = newOpenAIAdvancedSchedulerRateLimitService("true")
	}
	return svc
}

func schedulerModeName(advanced bool) string {
	if advanced {
		return "advanced_scheduler"
	}
	return "legacy_scheduler"
}

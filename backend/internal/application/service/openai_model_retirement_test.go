//go:build unit

package service

import (
	"context"
	"testing"
	"time"

	gocache "github.com/patrickmn/go-cache"
	"github.com/tidwall/gjson"

	"github.com/stretchr/testify/require"
)

func TestRetiredOpenAISubscriptionModelScopesUpstreamAccount(t *testing.T) {
	oauth := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	setup := &Account{Platform: PlatformOpenAI, Type: AccountTypeSetupToken}
	apiKey := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}

	require.True(t, isRetiredOpenAISubscriptionModel(oauth, "gpt5.5"))
	require.True(t, isRetiredOpenAISubscriptionModel(setup, "gpt-5.5-pro"))
	require.False(t, isRetiredOpenAISubscriptionModel(apiKey, "gpt-5.5"))
	require.False(t, isRetiredOpenAISubscriptionModel(oauth, "gpt-5.6"))
}

func TestRetiredOpenAIModelFilteringKeepsMixedAPIKeySupport(t *testing.T) {
	oauth := Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	apiKey := Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey}
	models := filterRetiredOpenAIModelIDs([]Account{oauth, apiKey}, []string{"gpt-5.5", "gpt-5.6"})
	require.Equal(t, []string{"gpt-5.5", "gpt-5.6"}, models)

	onlyOAuth := filterRetiredOpenAIModelIDs([]Account{oauth}, []string{"gpt-5.5", "gpt-5.6"})
	require.Equal(t, []string{"gpt-5.6"}, onlyOAuth)
}

func TestAccountModelSupportRejectsRetiredSubscriptionTarget(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth}
	require.False(t, account.IsModelSupportedForRequest("gpt-5.5", "gpt-5.5"))
	require.True(t, account.IsModelSupportedForRequest("gpt-5.6", "gpt-5.6"))
}

func TestRetirementMappingKeepsReplacementAndBlocksRetiredAlias(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{
		"model_mapping":         map[string]any{"gpt-5.5": "gpt-6.1-sol", "legacy": "gpt-5.5", "gpt-6.1-sol": "gpt-6.1-sol"},
		"compact_model_mapping": map[string]any{"gpt-6.1-sol": "gpt-5.5"},
	}}
	require.True(t, account.IsModelSupported("gpt-5.5"))
	require.False(t, account.IsModelSupported("legacy"))
	require.False(t, supportsOpenAIRequestModel(account, "gpt-6.1-sol", "", true))
	body := []byte(`{"revision":"keep","models":[{"slug":"gpt-5.5","future":true},{"slug":"gpt-6.1-sol","future":true}]}`)
	manifest := &CodexModelsManifest{Body: body, ETag: `"raw"`}
	projected, err := ProjectCodexModelsManifestForAccount(manifest, account, &Group{}, "")
	require.NoError(t, err)
	require.Equal(t, body, manifest.Body)
	require.Equal(t, "keep", gjson.GetBytes(projected.Body, "revision").String())
	require.NotContains(t, string(projected.Body), `"slug":"legacy"`)
	require.Contains(t, string(projected.Body), `"slug":"gpt-5.5"`)
	require.NotEqual(t, manifest.ETag, projected.ETag)
}

func TestRetirementRoutingSkipsSubscriptionInBothSchedulers(t *testing.T) {
	for _, advanced := range []bool{false, true} {
		t.Run(schedulerModeName(advanced), func(t *testing.T) {
			resetOpenAIAdvancedSchedulerSettingCacheForTest()
			groupID := int64(990901)
			oauth := Account{ID: 9901, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 0}
			apiKey := Account{ID: 9902, Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Status: StatusActive, Schedulable: true, Concurrency: 1, Priority: 10}
			svc := newUnknownModelRoutingService([]Account{oauth, apiKey}, advanced)
			selection, _, err := svc.SelectAccountWithScheduler(context.Background(), &groupID, "", "", "gpt-5.5", nil, OpenAIUpstreamTransportAny, false)
			require.NoError(t, err)
			require.NotNil(t, selection)
			require.Equal(t, apiKey.ID, selection.Account.ID)
		})
	}
}

func TestRetirementEmptyModelCatalogStaysAuthoritativeInCache(t *testing.T) {
	groupID := int64(990902)
	repo := &modelsListAccountRepoStub{byGroup: map[int64][]Account{groupID: {{ID: 9903, Platform: PlatformOpenAI, Type: AccountTypeOAuth, Credentials: map[string]any{"model_mapping": map[string]any{"legacy": "gpt-5.5"}}}}}}
	svc := &GatewayService{accountRepo: repo, modelsListCache: gocache.New(time.Minute, time.Minute), modelsListCacheTTL: time.Minute}
	for i := 0; i < 2; i++ {
		models := svc.GetAvailableModels(context.Background(), &groupID, PlatformOpenAI)
		require.NotNil(t, models, "empty retirement result must not resurrect platform defaults")
		require.Empty(t, models)
	}
}

func TestRetirementPassthroughManifestPreservesNonRetiredBytes(t *testing.T) {
	account := &Account{Platform: PlatformOpenAI, Type: AccountTypeOAuth, Extra: map[string]any{"openai_passthrough": true}}
	body := []byte("{\n \"models\":[{\"slug\":\"gpt-6.1-sol\",\"future\":\"keep\"}]\n}")
	manifest := &CodexModelsManifest{Body: body, ETag: `"raw"`}
	projected, err := ProjectCodexModelsManifestForAccount(manifest, account, &Group{}, "")
	require.NoError(t, err)
	require.Equal(t, body, projected.Body)
	require.Equal(t, manifest.ETag, projected.ETag)
}

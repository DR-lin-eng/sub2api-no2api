package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type declaredGeminiRepo struct {
	AccountRepository
	calls    int
	accounts []Account
}

func (r *declaredGeminiRepo) ListSchedulableByGroupIDAndPlatforms(context.Context, int64, []string) ([]Account, error) {
	r.calls++
	return append([]Account(nil), r.accounts...), nil
}

func TestGeminiDeclaredMetadataReusesLookupOnUnmappedFallback(t *testing.T) {
	repo := &declaredGeminiRepo{accounts: []Account{{ID: 1, Platform: PlatformGemini, Type: AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture"}}}}
	svc := &GeminiMessagesCompatService{accountRepo: repo}
	groupID := int64(1)
	ctx, ids, err := svc.PrepareGeminiModelsMetadata(context.Background(), &groupID)
	require.NoError(t, err)
	require.Nil(t, ids)
	account, err := svc.SelectAccountForAIStudioEndpoints(ctx, &groupID)
	require.NoError(t, err)
	require.Equal(t, int64(1), account.ID)
	require.Equal(t, 1, repo.calls, "declaration detection and fallback selection share one request lookup")
}

func TestGeminiDeclaredMetadataCombinesMappingsAndMixedGeminiModels(t *testing.T) {
	repo := &declaredGeminiRepo{accounts: []Account{
		{ID: 1, Platform: PlatformGemini, Credentials: map[string]any{"model_mapping": map[string]any{"gemini-3.8-*": "gemini-3.8-flash", "gemini-3.1-pro-preview": "gemini-3.1-pro-preview"}}},
		{ID: 2, Platform: PlatformAntigravity, Extra: map[string]any{"mixed_scheduling": true}, Credentials: map[string]any{"model_mapping": map[string]any{"gemini-custom": "gemini-3.8-flash", "claude-custom": "claude-opus-4-6"}}},
	}}
	svc := &GeminiMessagesCompatService{accountRepo: repo}
	groupID := int64(1)
	_, ids, err := svc.PrepareGeminiModelsMetadata(context.Background(), &groupID)
	require.NoError(t, err)
	require.Contains(t, ids, "gemini-custom")
	require.NotContains(t, ids, "claude-custom")
	require.True(t, DeclaredGeminiModelsCover(ids, "gemini-3.8-flash"))
	require.True(t, DeclaredGeminiModelsCover(ids, "gemini-3.1-pro-preview-customtools"))
	require.False(t, DeclaredGeminiModelsCover(ids, "gemini-unmapped"))
}

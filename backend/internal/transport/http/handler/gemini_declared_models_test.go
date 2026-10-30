package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type declaredMetadataRepo struct {
	service.AccountRepository
	calls int
}

func (r *declaredMetadataRepo) ListSchedulableByGroupIDAndPlatforms(context.Context, int64, []string) ([]service.Account, error) {
	r.calls++
	return []service.Account{
		{ID: 1, Platform: service.PlatformGemini, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture-a", "model_mapping": map[string]any{"gemini-alpha": "gemini-3.8-flash"}}},
		{ID: 2, Platform: service.PlatformGemini, Type: service.AccountTypeServiceAccount, Credentials: map[string]any{"model_mapping": map[string]any{"gemini-beta*": "gemini-3.8-flash"}}},
	}, nil
}
func TestGeminiNativeDeclaredMetadataAvoidsUpstreamAndFiltersAllowlist(t *testing.T) {
	repo := &declaredMetadataRepo{}
	svc := service.NewGeminiMessagesCompatService(repo, nil, nil, nil, nil, nil, nil, nil, nil)
	handler := &GatewayHandler{geminiCompatService: svc}
	groupID := int64(42)
	for _, model := range []string{"", "gemini-beta-variant", "gemini-missing"} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1beta/models/"+model, nil)
		c.Params = gin.Params{{Key: "model", Value: model}}
		group := &service.Group{ID: groupID, Platform: service.PlatformGemini}
		if model == "" {
			group.ModelAllowlist = service.GroupModelAllowlist{Enabled: true, Models: []string{"gemini-alpha"}}
		}
		c.Set(string(middleware.ContextKeyAPIKey), &service.APIKey{GroupID: &groupID, Group: group})
		if model == "" {
			handler.GeminiV1BetaListModels(c)
			require.JSONEq(t, `{"models":[{"name":"models/gemini-alpha","supportedGenerationMethods":["generateContent","streamGenerateContent"]}]}`, rec.Body.String())
		} else {
			handler.GeminiV1BetaGetModel(c)
		}
		if model == "gemini-missing" {
			require.Equal(t, http.StatusNotFound, rec.Code)
		} else {
			require.Equal(t, http.StatusOK, rec.Code)
		}
	}
	require.Equal(t, 3, repo.calls)
}

package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler/admin"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type modelSyncRouteRepo struct {
	service.AccountRepository
	mu       sync.Mutex
	accounts map[int64]*service.Account
}

func (r *modelSyncRouteRepo) GetByID(_ context.Context, id int64) (*service.Account, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if account := r.accounts[id]; account != nil {
		copy := *account
		return &copy, nil
	}
	return nil, service.ErrAccountNotFound
}

func (r *modelSyncRouteRepo) MergeUpstreamModelMapping(_ context.Context, account *service.Account, models []string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := r.accounts[account.ID]
	mapping := map[string]any{}
	if current, ok := stored.Credentials["model_mapping"].(map[string]any); ok {
		for key, value := range current {
			mapping[key] = value
		}
	}
	added := 0
	for _, model := range models {
		if _, exists := mapping[model]; !exists {
			mapping[model] = model
			added++
		}
	}
	stored.Credentials["model_mapping"] = mapping
	return added, nil
}

type modelSyncRouteUpstream struct{}

func (modelSyncRouteUpstream) Do(_ *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"upstream-new"},{"id":"existing"}]}`))}, nil
}

func (u modelSyncRouteUpstream) DoWithTLS(req *http.Request, proxyURL string, id int64, concurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return u.Do(req, proxyURL, id, concurrency)
}

func setupModelSyncRoute() (*gin.Engine, *modelSyncRouteRepo) {
	gin.SetMode(gin.TestMode)
	repo := &modelSyncRouteRepo{accounts: map[int64]*service.Account{
		1: {ID: 1, Name: "OAuth fixture", Platform: service.PlatformAnthropic, Type: service.AccountTypeOAuth, Credentials: map[string]any{"access_token": "fixture-oauth", "model_mapping": map[string]any{"existing": "custom-target"}}},
		2: {ID: 2, Name: "API Key fixture", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"api_key": "fixture-key", "model_mapping": map[string]any{"existing": "existing"}}},
		3: {ID: 3, Name: "Missing key", Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Credentials: map[string]any{"model_mapping": map[string]any{"original": "original"}}},
		4: {ID: 4, Name: "Unsupported fixture", Platform: service.PlatformAnthropic, Type: service.AccountTypeServiceAccount, Credentials: map[string]any{}},
	}}
	testService := service.NewAccountTestService(repo, nil, nil, nil, nil, modelSyncRouteUpstream{},
		&config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{Enabled: false}}}, nil)
	accountHandler := admin.NewAccountHandler(nil, nil, nil, nil, nil, nil, nil, nil, testService, nil, nil, nil, nil, nil)
	router := gin.New()
	registerAccountRoutes(router.Group("/api/v1/admin"), &handler.Handlers{Admin: &handler.AdminHandlers{Account: accountHandler}}, nil)
	return router, repo
}

func TestAccountModelsBatchSyncRoute(t *testing.T) {
	router, repo := setupModelSyncRoute()
	request := `{"account_ids":[1,2,3,4,1]}`
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/models/sync-upstream-batch", strings.NewReader(request)))
	fmt.Printf("MODEL_SYNC_HTTP status=%d body=%s\n", rec.Code, strings.TrimSpace(rec.Body.String()))
	require.Equal(t, http.StatusOK, rec.Code)
	var envelope struct {
		Data struct {
			Total   int `json:"total"`
			Success int `json:"success"`
			Failed  int `json:"failed"`
			Skipped int `json:"skipped"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &envelope))
	require.Equal(t, 4, envelope.Data.Total)
	require.Equal(t, 2, envelope.Data.Success)
	require.Equal(t, 1, envelope.Data.Failed)
	require.Equal(t, 1, envelope.Data.Skipped)
	require.Equal(t, "custom-target", repo.accounts[1].Credentials["model_mapping"].(map[string]any)["existing"])
	require.Equal(t, "upstream-new", repo.accounts[1].Credentials["model_mapping"].(map[string]any)["upstream-new"])
	require.Equal(t, "fixture-oauth", repo.accounts[1].Credentials["access_token"])
	require.Equal(t, "fixture-key", repo.accounts[2].Credentials["api_key"])
	require.Equal(t, map[string]any{"original": "original"}, repo.accounts[3].Credentials["model_mapping"])

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/models/sync-upstream-batch", strings.NewReader(request)))
	require.Equal(t, 200, rec.Code)
	require.Contains(t, rec.Body.String(), `"added_count":0`)
}

func TestAccountModelsBatchSyncRouteRejectsInvalidInput(t *testing.T) {
	router, _ := setupModelSyncRoute()
	for _, body := range []string{`{}`, `{"account_ids":[]}`, `{"account_ids":[0]}`, `{"account_ids":[-1]}`, `{"account_ids":["1"]}`, `not json`} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/models/sync-upstream-batch", strings.NewReader(body)))
		require.Equal(t, 400, rec.Code, body)
	}
}

//go:build unit

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type cpaSyncHandlerAccountRepo struct {
	service.AccountRepository
}

func (r *cpaSyncHandlerAccountRepo) FindByExtraField(_ context.Context, _ string, _ any) ([]service.Account, error) {
	return nil, nil
}

func newCPAHandlerForRemote(t *testing.T, responseStatus int, responseBody string) (*AccountHandler, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer handler-fixture-password", r.Header.Get("Authorization"))
		require.Equal(t, "/v0/management/auth-files", r.URL.Path)
		w.WriteHeader(responseStatus)
		_, _ = w.Write([]byte(responseBody))
	}))
	t.Cleanup(server.Close)

	cfg := &config.Config{}
	cfg.Security.URLAllowlist.AllowPrivateHosts = true
	cfg.Security.URLAllowlist.AllowInsecureHTTP = true
	repo := &cpaSyncHandlerAccountRepo{}
	adminService := newStubAdminService()
	syncService := service.NewCPASyncService(repo, adminService, cfg, nil, nil)
	return &AccountHandler{cpaSyncService: syncService}, server
}

func TestAccountHandlerCPAPreviewReturnsMetadataWithoutCredentials(t *testing.T) {
	const password = "handler-fixture-password"
	handler, server := newCPAHandlerForRemote(t, http.StatusOK, `{"files":[{"name":"one.json","label":"One","email":"one@example.test","provider":"codex","status":"active"}]}`)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/sync/cpa/preview", handler.PreviewFromCPA)
	body := bytes.NewBufferString(`{"base_url":"` + server.URL + `","management_password":"` + password + `","platform":"openai"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/sync/cpa/preview", body)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusOK, recorder.Code)
	var response struct {
		Data service.CPAPreviewResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Equal(t, 1, response.Data.Total)
	require.Len(t, response.Data.Accounts, 1)
	require.Equal(t, "one@example.test", response.Data.Accounts[0].Email)
	require.NotContains(t, recorder.Body.String(), password)
	require.NotContains(t, recorder.Body.String(), "access_token")
	require.NotContains(t, recorder.Body.String(), "refresh_token")
}

func TestAccountHandlerCPASyncValidatesSelectionBeforeRemoteWrite(t *testing.T) {
	handler, server := newCPAHandlerForRemote(t, http.StatusOK, `{"files":[]}`)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/sync/cpa", handler.SyncFromCPA)
	body := bytes.NewBufferString(`{"base_url":"` + server.URL + `","management_password":"handler-fixture-password","platform":"openai","selected_files":[]}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/sync/cpa", body)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadRequest, recorder.Code)
	require.Contains(t, recorder.Body.String(), "INVALID_CPA_SELECTION")
	require.NotContains(t, recorder.Body.String(), "handler-fixture-password")
	require.NotContains(t, recorder.Body.String(), "access_token")
}

func TestAccountHandlerCPARemoteErrorsDoNotExposeSecrets(t *testing.T) {
	const password = "handler-fixture-password"
	handler, server := newCPAHandlerForRemote(t, http.StatusUnauthorized, `fixture-password access_token refresh_token`)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/sync/cpa/preview", handler.PreviewFromCPA)
	body := bytes.NewBufferString(`{"base_url":"` + server.URL + `","management_password":"` + password + `","platform":"openai"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/sync/cpa/preview", body)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.Contains(t, recorder.Body.String(), "CPA_CONNECTION_FAILED")
	require.NotContains(t, recorder.Body.String(), password)
	require.NotContains(t, recorder.Body.String(), "access_token")
	require.NotContains(t, recorder.Body.String(), "refresh_token")
}

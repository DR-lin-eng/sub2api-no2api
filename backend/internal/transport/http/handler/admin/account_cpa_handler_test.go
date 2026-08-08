//go:build unit

package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestAccountHandlerCPAConnectionTestAndSync(t *testing.T) {
	var mu sync.Mutex
	authorizationHeaders := make([]string, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authorizationHeaders = append(authorizationHeaders, r.Header.Get("Authorization"))
		mu.Unlock()
		require.Equal(t, "/v0/management/auth-files", r.URL.Path)
		_, _ = w.Write([]byte(`{"files":[{"status":"active"},{"status":"error"}]}`))
	}))
	defer server.Close()

	adminService := newStubAdminService()
	adminService.getAccountResult = &service.Account{
		ID: 9, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey, Concurrency: 99,
		Credentials: map[string]any{
			"base_url":                                         server.URL + "/v1",
			service.CPAModeCredentialKey:                       true,
			service.CPAManagementKeyCredentialKey:              "stored-password",
			service.CPAConcurrencyPerCredentialCredentialKey:   10,
			service.CPAExcludeAbnormalCredentialsCredentialKey: true,
		},
	}
	concurrencyService := service.NewConcurrencyService(nil)
	handler := &AccountHandler{adminService: adminService, concurrencyService: concurrencyService}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/accounts/:id/cpa/test", handler.TestCPAConnection)
	router.POST("/accounts/:id/cpa/sync", handler.SyncCPACapacity)

	body := bytes.NewBufferString(`{"use_account_base_url":true,"base_url":"` + server.URL + `/v1","management_password":"unsaved-password","concurrency_per_credential":10,"exclude_abnormal_credentials":false}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/accounts/9/cpa/test", body)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	var testResponse struct {
		Data service.CPATestResult `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &testResponse))
	require.Equal(t, 2, testResponse.Data.EnabledCredentials)
	require.Equal(t, 1, testResponse.Data.AbnormalCredentials)
	require.Equal(t, 1, testResponse.Data.AvailableCredentials)
	require.Equal(t, 2, testResponse.Data.CapacityCredentials)
	require.False(t, testResponse.Data.ExcludeAbnormalCredentials)
	require.Equal(t, 20, testResponse.Data.EffectiveConcurrency)

	status, err := concurrencyService.GetCPACapacityStatus(context.Background(), adminService.getAccountResult)
	require.NoError(t, err)
	require.Equal(t, 1, status.AvailableCredentials)
	require.Equal(t, 1, status.CapacityCredentials)
	require.True(t, status.ExcludeAbnormalCredentials)
	require.Equal(t, 10, status.EffectiveConcurrency)

	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/accounts/9/cpa/sync", nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)

	mu.Lock()
	require.Equal(t, []string{
		"Bearer unsaved-password",
		"Bearer stored-password",
		"Bearer stored-password",
	}, authorizationHeaders)
	mu.Unlock()
}

func TestAccountHandlerCPAConnectionFailureRedactsPassword(t *testing.T) {
	const administratorPassword = "never-return-this-password"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	adminService := newStubAdminService()
	adminService.getAccountResult = &service.Account{
		ID: 9, Platform: service.PlatformOpenAI, Type: service.AccountTypeAPIKey,
		Credentials: map[string]any{"base_url": server.URL + "/v1"},
	}
	concurrencyService := service.NewConcurrencyService(nil)
	handler := &AccountHandler{adminService: adminService, concurrencyService: concurrencyService}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/accounts/:id/cpa/test", handler.TestCPAConnection)

	body := bytes.NewBufferString(`{"use_account_base_url":true,"management_password":"` + administratorPassword + `"}`)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/accounts/9/cpa/test", body)
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)

	require.Equal(t, http.StatusBadGateway, recorder.Code)
	require.NotContains(t, recorder.Body.String(), administratorPassword)
	require.True(t, strings.Contains(recorder.Body.String(), "CPA_CONNECTION_FAILED") || strings.Contains(recorder.Body.String(), "CPA connection failed"))
}

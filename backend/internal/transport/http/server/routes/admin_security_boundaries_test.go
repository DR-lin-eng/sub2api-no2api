package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/transport/http/handler/admin"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type securitySettings struct {
	service.SettingRepository
	values map[string]string
}

func (s *securitySettings) GetValue(_ context.Context, key string) (string, error) {
	return s.values[key], nil
}
func (s *securitySettings) Set(_ context.Context, key, value string) error {
	s.values[key] = value
	return nil
}

func (s *securitySettings) Delete(_ context.Context, key string) error {
	delete(s.values, key)
	return nil
}

type securityAdminUsers struct{ service.UserRepository }

func (*securityAdminUsers) GetFirstAdmin(context.Context) (*service.User, error) {
	return &service.User{ID: 1, Email: "fixture@example.test", Role: service.RoleAdmin, Status: service.StatusActive}, nil
}

func TestSecurityH1ScopedCredentialCannotMintBroaderCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{`{"name":"minted","scopes":["admin.read","admin.write"]}`, `{"name":"default-scope"}`} {
		settings := service.NewSettingService(&securitySettings{values: map[string]string{}}, nil)
		_, restricted, err := settings.CreateAdminAPIKey(t.Context(), service.AdminAPIKeyCreateInput{Name: "restricted", Scopes: []string{service.AdminAPIKeyScopeSettingsWrite}}, 1)
		require.NoError(t, err)
		users := service.NewUserService(&securityAdminUsers{}, nil, nil, nil)
		router := gin.New()
		router.Use(gin.HandlerFunc(middleware.NewAdminAuthMiddleware(nil, users, settings, nil)), middleware.AdminPermissionMiddleware(settings))
		admin := router.Group("/api/v1/admin")
		registerSettingsRoutes(admin, &handler.Handlers{Admin: &handler.AdminHandlers{Setting: adminhandler.NewSettingHandler(settings, nil, nil, nil, nil, nil, nil)}})
		admin.GET("/users", func(c *gin.Context) { c.Status(http.StatusNoContent) })
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/settings/admin-api-keys", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("x-api-key", restricted)
		router.ServeHTTP(response, request)
		t.Logf("H1 authenticated settings.write key mint status=%d", response.Code)
		if response.Code == http.StatusCreated {
			var result struct {
				Data struct {
					Key string `json:"key"`
				} `json:"data"`
			}
			require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
			probe := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
			probe.Header.Set("x-api-key", result.Data.Key)
			observed := httptest.NewRecorder()
			router.ServeHTTP(observed, probe)
			t.Logf("H1 minted key users authorization status=%d", observed.Code)
		}
		require.Equal(t, http.StatusForbidden, response.Code)
		keys, err := settings.ListAdminAPIKeys(t.Context())
		require.NoError(t, err)
		require.Len(t, keys, 1)
	}
}

func TestSecurityH1ScopedCredentialCannotGrantRolePermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	settings := service.NewSettingService(&securitySettings{values: map[string]string{}}, nil)
	_, err := settings.UpdatePermissionGroups(t.Context(), service.DefaultPermissionGroups())
	require.NoError(t, err)
	_, restricted, err := settings.CreateAdminAPIKey(t.Context(), service.AdminAPIKeyCreateInput{Name: "restricted", Scopes: []string{service.AdminAPIKeyScopeSettingsWrite}}, 1)
	require.NoError(t, err)
	router := gin.New()
	users := service.NewUserService(&securityAdminUsers{}, nil, nil, nil)
	router.Use(gin.HandlerFunc(middleware.NewAdminAuthMiddleware(nil, users, settings, nil)), middleware.AdminPermissionMiddleware(settings))
	registerSettingsRoutes(router.Group("/api/v1/admin"), &handler.Handlers{Admin: &handler.AdminHandlers{Setting: adminhandler.NewSettingHandler(settings, nil, nil, nil, nil, nil, nil)}})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/settings/permission-groups", strings.NewReader(`{"groups":[{"id":"support","name":"Support","permissions":["support.read","users.credentials","accounts.manage"]}]}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("x-api-key", restricted)
	router.ServeHTTP(response, request)
	granted, err := settings.HasPermission(t.Context(), "support", service.PermissionUsersCredentials)
	require.NoError(t, err)
	t.Logf("H1 settings.write permission-group update status=%d users.credentials granted=%t", response.Code, granted)
	require.Equal(t, http.StatusForbidden, response.Code)
	require.False(t, granted)
}

func TestSecurityH1AdminKeyManagementRequiresHumanAdministrator(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, identity := range []struct {
		name, role, method string
		denied             bool
	}{
		{"machine", service.RoleAdmin, service.AuditAuthMethodAdminAPIKey, true},
		{"delegated", "settings_staff", service.AuditAuthMethodJWT, true},
		{"administrator", service.RoleAdmin, service.AuditAuthMethodJWT, false},
	} {
		t.Run(identity.name, func(t *testing.T) {
			settings := service.NewSettingService(&securitySettings{values: map[string]string{}}, nil)
			key, _, err := settings.CreateAdminAPIKey(t.Context(), service.AdminAPIKeyCreateInput{Name: "existing", Scopes: []string{service.AdminAPIKeyScopeRead, service.AdminAPIKeyScopeWrite}}, 1)
			require.NoError(t, err)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set(string(middleware.ContextKeyUserRole), identity.role)
				c.Set("auth_method", identity.method)
				c.Next()
			})
			registerSettingsRoutes(router.Group("/api/v1/admin"), &handler.Handlers{Admin: &handler.AdminHandlers{Setting: adminhandler.NewSettingHandler(settings, nil, nil, nil, nil, nil, nil)}})
			for _, request := range []struct {
				method, path, body string
				want               int
			}{
				{http.MethodGet, "/admin-api-keys", "", http.StatusOK},
				{http.MethodPost, "/admin-api-keys", `{"name":"minted","scopes":["admin.read","admin.write"]}`, http.StatusCreated},
				{http.MethodPut, "/admin-api-keys/" + key.ID, `{"scopes":["admin.read","admin.write"]}`, http.StatusOK},
				{http.MethodPost, "/admin-api-keys/" + key.ID + "/rotate", "", http.StatusOK},
				{http.MethodDelete, "/admin-api-keys/" + key.ID, "", http.StatusOK},
				{http.MethodGet, "/admin-api-key", "", http.StatusOK},
				{http.MethodPost, "/admin-api-key/regenerate", "", http.StatusOK},
				{http.MethodDelete, "/admin-api-key", "", http.StatusOK},
			} {
				response := httptest.NewRecorder()
				req := httptest.NewRequest(request.method, "/api/v1/admin/settings"+request.path, strings.NewReader(request.body))
				req.Header.Set("Content-Type", "application/json")
				router.ServeHTTP(response, req)
				t.Logf("H1 %s %s %s status=%d", identity.name, request.method, request.path, response.Code)
				if identity.denied && request.method != http.MethodGet {
					require.Equal(t, http.StatusForbidden, response.Code)
				} else {
					require.Equal(t, request.want, response.Code)
				}
			}
		})
	}
}

func TestSecurityH2S3TestRoutesRequireStepUp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, path := range []string{"s3-config/test", "image-storage/test"} {
		t.Run(path, func(t *testing.T) {
			router := gin.New()
			gateReached := false
			gate := middleware.StepUpAuthMiddleware(func(c *gin.Context) { gateReached = true; c.AbortWithStatus(http.StatusForbidden) })
			registerBackupRoutes(router.Group("/api/v1/admin"), &handler.Handlers{Admin: &handler.AdminHandlers{Backup: &adminhandler.BackupHandler{}}}, gate)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/api/v1/admin/backups/"+path, strings.NewReader("{")))
			t.Logf("H2 %s step_up=%t status=%d", path, gateReached, response.Code)
			require.True(t, gateReached)
			require.Equal(t, http.StatusForbidden, response.Code)
		})
	}
}

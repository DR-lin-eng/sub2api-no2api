//go:build unit

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type embeddedCapabilitySettingRepo struct {
	service.SettingRepository
	values map[string]string
}

func (r *embeddedCapabilitySettingRepo) GetValue(_ context.Context, key string) (string, error) {
	value, ok := r.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}

func embeddedCapabilityTestHandler() *AuthHandler {
	cfg := &config.Config{JWT: config.JWTConfig{Secret: "handler-test-secret", ExpireHour: 1}}
	settingsRepo := &embeddedCapabilitySettingRepo{values: map[string]string{
		service.SettingKeyCustomMenuItems: `[{"id":"help","url":"https://help.example.test/embed","visibility":"user","forward_access_token":true}]`,
	}}
	settings := service.NewSettingService(settingsRepo, cfg)
	userRepo := &userHandlerRepoStub{user: &service.User{
		ID: 17, Email: "user@example.test", Role: service.RoleUser,
		Status: service.StatusActive, TokenVersion: 4,
	}}
	users := service.NewUserService(userRepo, settingsRepo, nil, nil)
	auth := service.NewAuthService(
		nil, userRepo, nil, nil, cfg, settings, nil, nil, nil, nil, nil, nil, nil,
	)
	return &AuthHandler{cfg: cfg, authService: auth, userService: users, settingSvc: settings}
}

func TestEmbeddedCapabilityHandlersIssueAndIntrospectWithoutSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := embeddedCapabilityTestHandler()

	issueRecorder := httptest.NewRecorder()
	issueContext, _ := gin.CreateTestContext(issueRecorder)
	issueContext.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/embedded-capability",
		bytes.NewBufferString(`{"menu_id":"help","target_origin":"https://help.example.test"}`),
	)
	issueContext.Request.Header.Set("Content-Type", "application/json")
	issueContext.Set(string(middleware2.ContextKeyUser), middleware2.AuthSubject{UserID: 17})
	handler.IssueEmbeddedCapability(issueContext)
	require.Equal(t, http.StatusOK, issueRecorder.Code)

	var issued struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(issueRecorder.Body.Bytes(), &issued))
	require.NotEmpty(t, issued.Data.Token)
	require.Equal(t, "no-store", issueRecorder.Header().Get("Cache-Control"))
	require.Empty(t, issueRecorder.Header().Values("Set-Cookie"))

	verifyBody, err := json.Marshal(map[string]string{
		"token": issued.Data.Token, "audience": "https://help.example.test",
	})
	require.NoError(t, err)
	verifyRecorder := httptest.NewRecorder()
	verifyContext, _ := gin.CreateTestContext(verifyRecorder)
	verifyContext.Request = httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/embedded-capability/verify",
		bytes.NewReader(verifyBody),
	)
	verifyContext.Request.Header.Set("Content-Type", "application/json")
	verifyContext.Request.Header.Set("Origin", "https://help.example.test")
	handler.VerifyEmbeddedCapability(verifyContext)
	require.Equal(t, http.StatusOK, verifyRecorder.Code)
	require.Equal(t, "no-store", verifyRecorder.Header().Get("Cache-Control"))
	require.Empty(t, verifyRecorder.Header().Values("Set-Cookie"))
	require.Contains(t, verifyRecorder.Body.String(), `"valid":true`)
	require.Contains(t, verifyRecorder.Body.String(), `"custom_menu:access"`)
}

func TestEmbeddedCapabilityVerifyRejectsMismatchedBrowserOrigin(t *testing.T) {
	handler := embeddedCapabilityTestHandler()
	user, err := handler.userService.GetByID(context.Background(), 17)
	require.NoError(t, err)
	target, err := handler.settingSvc.ResolveEmbeddedCapabilityTarget(
		context.Background(), "help", "https://help.example.test", service.RoleUser,
	)
	require.NoError(t, err)
	issued, err := handler.authService.IssueEmbeddedCapability(user, target)
	require.NoError(t, err)
	body, err := json.Marshal(map[string]string{
		"token": issued.Token, "audience": "https://help.example.test",
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/verify", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Header.Set("Origin", "https://attacker.example.test")
	handler.VerifyEmbeddedCapability(c)
	require.Equal(t, http.StatusForbidden, recorder.Code)
}

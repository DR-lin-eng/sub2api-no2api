//go:build unit

package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type oauthCaptchaSettingRepo struct {
	service.SettingRepository
	values map[string]string
}

func (r *oauthCaptchaSettingRepo) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	values := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := r.values[key]; ok {
			values[key] = value
		}
	}
	return values, nil
}

type oauthCaptchaVerifier struct {
	calls int
	proof service.TencentCaptchaProof
}

func (v *oauthCaptchaVerifier) VerifyTicket(
	_ context.Context,
	_ service.TencentCaptchaCredentials,
	proof service.TencentCaptchaProof,
	_ string,
) (*service.TencentCaptchaVerifyResponse, error) {
	v.calls++
	v.proof = proof
	return &service.TencentCaptchaVerifyResponse{CaptchaCode: 1}, nil
}

func newOAuthCaptchaTestHandler(enabled bool) (*AuthHandler, *oauthCaptchaVerifier) {
	values := map[string]string{}
	if enabled {
		values = map[string]string{
			service.SettingKeyTencentCaptchaEnabled:        "true",
			service.SettingKeyTencentCaptchaAppID:          "123456789",
			service.SettingKeyTencentCaptchaAppSecretKey:   "app-secret",
			service.SettingKeyTencentCaptchaCloudSecretID:  "cloud-secret-id",
			service.SettingKeyTencentCaptchaCloudSecretKey: "cloud-secret-key",
		}
	}
	cfg := &config.Config{}
	settings := service.NewSettingService(&oauthCaptchaSettingRepo{values: values}, cfg)
	verifier := &oauthCaptchaVerifier{}
	humanVerification := service.NewHumanVerificationService(settings, nil, nil, nil, verifier)
	authService := service.NewAuthService(
		nil, nil, nil, nil, cfg, settings, nil, humanVerification, nil, nil, nil, nil, nil,
	)
	return &AuthHandler{authService: authService, settingSvc: settings, cfg: cfg}, verifier
}

func oauthStartHandlers() map[string]func(*AuthHandler, *gin.Context) {
	return map[string]func(*AuthHandler, *gin.Context){
		"github":   func(h *AuthHandler, c *gin.Context) { h.GitHubOAuthStart(c) },
		"google":   func(h *AuthHandler, c *gin.Context) { h.GoogleOAuthStart(c) },
		"linuxdo":  func(h *AuthHandler, c *gin.Context) { h.LinuxDoOAuthStart(c) },
		"dingtalk": func(h *AuthHandler, c *gin.Context) { h.DingTalkOAuthStart(c) },
		"wechat":   func(h *AuthHandler, c *gin.Context) { h.WeChatOAuthStart(c) },
		"oidc":     func(h *AuthHandler, c *gin.Context) { h.OIDCOAuthStart(c) },
	}
}

func TestOAuthStartGetRejectsTencentProtectedLoginBeforeSideEffects(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for provider, start := range oauthStartHandlers() {
		t.Run(provider, func(t *testing.T) {
			handler, verifier := newOAuthCaptchaTestHandler(true)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/"+provider+"/start?intent=bind_current_user", nil)

			start(handler, c)

			require.Equal(t, http.StatusBadRequest, recorder.Code)
			require.Contains(t, recorder.Body.String(), "TENCENT_CAPTCHA_VERIFICATION_FAILED")
			require.Empty(t, recorder.Header().Get("Location"))
			require.Empty(t, recorder.Header().Values("Set-Cookie"))
			require.Zero(t, verifier.calls)
		})
	}
}

func TestOAuthStartPostReturnsAuthorizeURLAfterTencentVerification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for provider := range oauthStartHandlers() {
		t.Run(provider, func(t *testing.T) {
			handler, verifier := newOAuthCaptchaTestHandler(true)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(
				http.MethodPost,
				"/api/v1/auth/oauth/"+provider+"/start",
				strings.NewReader(`{"tencent_captcha_ticket":"ticket-value","tencent_captcha_randstr":"@rand-value"}`),
			)
			c.Request.Header.Set("Content-Type", "application/json")

			require.True(t, handler.requireActionCaptchaForOAuthLoginStart(c))
			respondOAuthStart(c, "https://provider.example/authorize")

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Contains(t, recorder.Body.String(), `"authorize_url":"https://provider.example/authorize"`)
			require.Equal(t, 1, verifier.calls)
			require.Equal(t, service.TencentCaptchaProof{Ticket: "ticket-value", Randstr: "@rand-value"}, verifier.proof)
		})
	}
}

func TestOAuthBindingStartRemainsOutsideTencentGate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &AuthHandler{}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/oidc/bind/start", nil)

	require.True(t, handler.requireActionCaptchaForOAuthLoginStart(c))
	require.Equal(t, http.StatusOK, recorder.Code)
}

func TestOAuthStartGetRemainsCompatibleWhenTencentDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, verifier := newOAuthCaptchaTestHandler(false)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/auth/oauth/github/start", nil)

	require.True(t, handler.requireActionCaptchaForOAuthLoginStart(c))
	respondOAuthStart(c, "https://provider.example/authorize")

	require.Equal(t, http.StatusFound, recorder.Code)
	require.Equal(t, "https://provider.example/authorize", recorder.Header().Get("Location"))
	require.Zero(t, verifier.calls)
}

func TestAuthRequestsBindTencentCaptchaProof(t *testing.T) {
	const payload = `{"email":"user@example.com","password":"secret-123","tencent_captcha_ticket":"ticket-value","tencent_captcha_randstr":"@rand-value"}`

	var login LoginRequest
	require.NoError(t, json.Unmarshal([]byte(payload), &login))
	proof := humanVerificationProof(login.CaptchaToken, login.TurnstileToken, login.TencentCaptchaTicket, login.TencentCaptchaRandstr)
	require.Equal(t, "ticket-value", proof.TencentTicket)
	require.Equal(t, "@rand-value", proof.TencentRandstr)

	var pending createPendingOAuthAccountRequest
	require.NoError(t, json.Unmarshal([]byte(payload), &pending))
	proof = humanVerificationProof(pending.CaptchaToken, pending.TurnstileToken, pending.TencentCaptchaTicket, pending.TencentCaptchaRandstr)
	require.Equal(t, "ticket-value", proof.TencentTicket)
	require.Equal(t, "@rand-value", proof.TencentRandstr)
}

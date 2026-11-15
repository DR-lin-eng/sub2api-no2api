package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	dbent "github.com/DR-lin-eng/sub2api-no2api/ent"
	dbuser "github.com/DR-lin-eng/sub2api-no2api/ent/user"
	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type pendingOAuthTencentVerifierStub struct {
	calls int
}

func (s *pendingOAuthTencentVerifierStub) VerifyTicket(
	context.Context,
	service.TencentCaptchaCredentials,
	service.TencentCaptchaProof,
	string,
) (*service.TencentCaptchaVerifyResponse, error) {
	s.calls++
	return &service.TencentCaptchaVerifyResponse{CaptchaCode: 1}, nil
}

func pendingOAuthTencentSettingValues() map[string]string {
	return map[string]string{
		service.SettingKeyTencentCaptchaEnabled:        "true",
		service.SettingKeyTencentCaptchaAppID:          "123456789",
		service.SettingKeyTencentCaptchaAppSecretKey:   "app-secret",
		service.SettingKeyTencentCaptchaCloudSecretID:  "cloud-secret-id",
		service.SettingKeyTencentCaptchaCloudSecretKey: "cloud-secret-key",
	}
}

func createTencentProtectedPendingOAuthSession(
	t *testing.T,
	client *dbent.Client,
	suffix string,
) (string, string) {
	t.Helper()
	sessionToken := "tencent-captcha-" + suffix + "-session-token"
	browserSessionKey := "tencent-captcha-" + suffix + "-browser-key"
	_, err := client.PendingAuthSession.Create().
		SetSessionToken(sessionToken).
		SetIntent("login").
		SetProviderType("oidc").
		SetProviderKey("https://issuer.example").
		SetProviderSubject("oidc-tencent-captcha-" + suffix).
		SetBrowserSessionKey(browserSessionKey).
		SetUpstreamIdentityClaims(map[string]any{"username": "oidc_user"}).
		SetExpiresAt(time.Now().UTC().Add(10 * time.Minute)).
		Save(context.Background())
	require.NoError(t, err)
	return sessionToken, browserSessionKey
}

func postPendingOAuthCreateAccount(
	t *testing.T,
	handler *AuthHandler,
	sessionToken string,
	browserSessionKey string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	ginContext, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/auth/oauth/oidc/create-account",
		bytes.NewBufferString(body),
	)
	request.Header.Set("Content-Type", "application/json")
	request.AddCookie(&http.Cookie{Name: oauthPendingSessionCookieName, Value: encodeCookieValue(sessionToken)})
	request.AddCookie(&http.Cookie{Name: oauthPendingBrowserCookieName, Value: encodeCookieValue(browserSessionKey)})
	ginContext.Request = request

	handler.CreateOIDCOAuthAccount(ginContext)
	return recorder
}

func TestPendingOAuthCreateAccountTencentCaptchaPolicy(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("email verification proof skips duplicate captcha", func(t *testing.T) {
		verifier := &pendingOAuthTencentVerifierStub{}
		handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
			emailVerifyEnabled: true,
			emailCache: &oauthPendingFlowEmailCacheStub{
				verificationCodes: map[string]*service.VerificationCodeData{
					"verified@example.com": {
						Code:      "246810",
						CreatedAt: time.Now().UTC(),
						ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
					},
				},
			},
			tencentVerifier: verifier,
			settingValues:   pendingOAuthTencentSettingValues(),
		})
		sessionToken, browserSessionKey := createTencentProtectedPendingOAuthSession(t, client, "email-verified")

		recorder := postPendingOAuthCreateAccount(
			t,
			handler,
			sessionToken,
			browserSessionKey,
			`{"email":"verified@example.com","verify_code":"246810","password":"secret-123"}`,
		)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		require.Zero(t, verifier.calls)
		count, err := client.User.Query().Where(dbuser.EmailEQ("verified@example.com")).Count(context.Background())
		require.NoError(t, err)
		require.Equal(t, 1, count)
	})

	t.Run("captcha proof is required without email verification", func(t *testing.T) {
		verifier := &pendingOAuthTencentVerifierStub{}
		handler, client := newOAuthPendingFlowTestHandlerWithDependencies(t, oauthPendingFlowTestHandlerOptions{
			tencentVerifier: verifier,
			settingValues:   pendingOAuthTencentSettingValues(),
		})
		sessionToken, browserSessionKey := createTencentProtectedPendingOAuthSession(t, client, "email-disabled")

		recorder := postPendingOAuthCreateAccount(
			t,
			handler,
			sessionToken,
			browserSessionKey,
			`{"email":"unverified@example.com","password":"secret-123"}`,
		)

		require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
		payload := decodeJSONBody(t, recorder)
		require.Equal(t, "TENCENT_CAPTCHA_VERIFICATION_FAILED", payload["reason"])
		require.Zero(t, verifier.calls)
		count, err := client.User.Query().Where(dbuser.EmailEQ("unverified@example.com")).Count(context.Background())
		require.NoError(t, err)
		require.Zero(t, count)
	})
}

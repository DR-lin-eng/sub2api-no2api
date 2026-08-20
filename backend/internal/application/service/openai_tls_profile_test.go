package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain/model"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestOpenAIOAuthTLSProfileIsEnabledAndStable(t *testing.T) {
	profileService := &TLSFingerprintProfileService{localCache: make(map[int64]*model.TLSFingerprintProfile)}
	profileService.setLocalCache([]*model.TLSFingerprintProfile{
		{ID: 101, Name: "codex-a"},
		{ID: 202, Name: "codex-b"},
	})
	service := &OpenAIGatewayService{tlsFPProfileService: profileService}
	account := &Account{
		ID:       73,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"enable_tls_fingerprint": true, "tls_fingerprint_profile_id": -1},
	}

	first := service.resolveTLSProfile(account)
	second := service.resolveTLSProfile(account)
	require.NotNil(t, first)
	require.Equal(t, tlsfingerprint.FingerprintKey(first), tlsfingerprint.FingerprintKey(second))
	require.True(t, account.IsTLSFingerprintEnabled())
	require.False(t, (&Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Extra: map[string]any{"enable_tls_fingerprint": true}}).IsTLSFingerprintEnabled())
}

func TestTLSProfileKeyIgnoresAdministrativeProfileName(t *testing.T) {
	first := tlsfingerprint.FingerprintKey(&tlsfingerprint.Profile{Name: "first"})
	second := tlsfingerprint.FingerprintKey(&tlsfingerprint.Profile{Name: "second"})
	require.Equal(t, first, second)
}

func TestOpenAIRequestCarriesResolvedTLSProfileToTransportBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)

	profileService := &TLSFingerprintProfileService{localCache: make(map[int64]*model.TLSFingerprintProfile)}
	profileService.setLocalCache([]*model.TLSFingerprintProfile{{ID: 1, Name: "codex"}})
	service := &OpenAIGatewayService{tlsFPProfileService: profileService}
	account := &Account{
		ID:       91,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra:    map[string]any{"enable_tls_fingerprint": true, "tls_fingerprint_profile_id": int64(1)},
	}

	req, err := service.buildUpstreamRequestWithFingerprint(
		context.Background(), c, account, []byte(`{"model":"gpt-5"}`), "token", false, "", true, nil,
	)
	require.NoError(t, err)
	profile := HTTPUpstreamTLSProfileFromContext(req.Context())
	require.NotNil(t, profile)
	require.Equal(t, "codex", profile.Name)
}

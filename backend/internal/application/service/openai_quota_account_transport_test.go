package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain/model"
	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/stretchr/testify/require"
)

type quotaAccountTransportCapture struct {
	request      *http.Request
	profile      *tlsfingerprint.Profile
	requestBody  string
	responseBody string
	responseCode int
}

func (c *quotaAccountTransportCapture) Do(req *http.Request, proxyURL string, accountID int64, accountConcurrency int) (*http.Response, error) {
	return c.DoWithTLS(req, proxyURL, accountID, accountConcurrency, nil)
}

func (c *quotaAccountTransportCapture) DoWithTLS(req *http.Request, _ string, _ int64, _ int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	c.request = req
	c.profile = profile
	if req != nil && req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		c.requestBody = string(body)
	}
	status := c.responseCode
	if status == 0 {
		status = http.StatusOK
	}
	body := c.responseBody
	if body == "" {
		body = `{"ok":true}`
	}
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}, nil
}

func TestCodexQuotaAccountTransportCarriesTLSAndHTTPProfile(t *testing.T) {
	profileService := &TLSFingerprintProfileService{localCache: make(map[int64]*model.TLSFingerprintProfile)}
	account := &Account{
		ID:       7001,
		Platform: PlatformOpenAI,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"enable_tls_fingerprint": true,
		},
	}
	capture := &quotaAccountTransportCapture{responseBody: `{"ok":true}`}
	service := NewOpenAIQuotaService(nil, nil, nil, nil)
	service.SetHTTPUpstream(capture, profileService)

	status, _, body, err := service.doCodexQuotaHTTP(
		context.Background(),
		account,
		http.MethodGet,
		"https://chatgpt.com/backend-api/wham/usage",
		map[string]string{"Authorization": "Bearer test"},
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, status)
	require.JSONEq(t, `{"ok":true}`, string(body))
	require.NotNil(t, capture.request)
	require.NotNil(t, capture.profile)
	require.Equal(t, "openai", string(HTTPUpstreamProfileFromContext(capture.request.Context())))
	require.Equal(t, "Bearer test", capture.request.Header.Get("Authorization"))
}

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

type codexModelsTransportCapture struct {
	calls   int
	profile *tlsfingerprint.Profile
}

func (c *codexModelsTransportCapture) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	return c.DoWithTLS(req, "", 0, 0, nil)
}

func (c *codexModelsTransportCapture) DoWithTLS(_ *http.Request, _ string, _ int64, _ int, profile *tlsfingerprint.Profile) (*http.Response, error) {
	c.calls++
	c.profile = profile
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"models":[{"slug":"gpt-5.5"}]}`)),
	}, nil
}

func TestFetchCodexModelsManifestOAuthUsesAccountScopedTransportOnce(t *testing.T) {
	account := newCodexModelsTestAccount()
	account.Extra = map[string]any{"enable_tls_fingerprint": true}
	capture := &codexModelsTransportCapture{}
	profileService := &TLSFingerprintProfileService{localCache: make(map[int64]*model.TLSFingerprintProfile)}
	service := &OpenAIGatewayService{httpUpstream: capture, tlsFPProfileService: profileService}

	manifest, err := service.FetchCodexModelsManifest(context.Background(), account, "0.149.0", "")
	require.NoError(t, err)
	require.NotNil(t, manifest)
	require.Equal(t, 1, capture.calls, "OAuth manifest must not fall through to a second generic client request")
	require.NotNil(t, capture.profile)
	require.Equal(t, "Built-in Codex Rustls (aws-lc-rs)", capture.profile.Name)
}

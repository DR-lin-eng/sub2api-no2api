//go:build unit

package handler

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type wechatFailingTransport struct{}

func (wechatFailingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("fixture TLS failure")
}

func TestWeChatOAuthTransportErrorDoesNotDiscloseSecrets(t *testing.T) {
	previous := http.DefaultTransport
	http.DefaultTransport = wechatFailingTransport{}
	t.Cleanup(func() { http.DefaultTransport = previous })
	_, err := exchangeWeChatOAuthCode(t.Context(), wechatOAuthConfig{appID: "fixture-app", appSecret: "fixture-private-app-secret"}, "fixture-authorization-code")
	require.Error(t, err)
	require.NotContains(t, err.Error(), "fixture-private-app-secret")
	require.NotContains(t, err.Error(), "fixture-authorization-code")
	require.NotContains(t, err.Error(), "secret=")
}

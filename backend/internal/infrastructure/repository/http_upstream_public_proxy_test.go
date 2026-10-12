package repository

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublicImageHTTPSProxyVerifiesProxyIdentityAndAuthentication(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		require.Equal(t, http.MethodConnect, r.Method)
		require.Equal(t, "8.8.8.8:443", r.Host)
		require.Equal(t, "Basic "+base64.StdEncoding.EncodeToString([]byte("fixture:fixture-password")), r.Header.Get("Proxy-Authorization"))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	proxyURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	proxyURL.User = url.UserPassword("fixture", "fixture-password")
	_, err = dialPublicImageProxy(t.Context(), "tcp", "8.8.8.8:443", proxyURL, nil)
	require.ErrorContains(t, err, "certificate")
	require.Zero(t, requests.Load(), "untrusted proxy certificate must not receive credentials")
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	conn, err := dialPublicImageProxy(t.Context(), "tcp", "8.8.8.8:443", proxyURL, &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12})
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	require.EqualValues(t, 1, requests.Load())
}

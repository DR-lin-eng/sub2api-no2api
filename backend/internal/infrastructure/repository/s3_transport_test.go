package repository

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestS3TransportPinsDNSAndRejectsRebinding(t *testing.T) {
	answers := []netip.Addr{netip.MustParseAddr("8.8.8.8")}
	resolver := publicDestinationResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) { return answers, nil })
	var destinations []string
	dial := s3DialContext(resolver, func(ctx context.Context, network, address string) (net.Conn, error) {
		destinations = append(destinations, address)
		client, server := net.Pipe()
		_ = server.Close()
		return client, nil
	})
	conn, err := dial(t.Context(), "tcp", "s3.example.com:443")
	require.NoError(t, err)
	_ = conn.Close()
	require.Equal(t, []string{"8.8.8.8:443"}, destinations)
	for _, unsafe := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "::ffff:127.0.0.1", "fc00::1"} {
		answers = []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr(unsafe)}
		_, err = dial(t.Context(), "tcp", "s3.example.com:443")
		require.Error(t, err)
		require.Len(t, destinations, 1, "no IP may be dialed when any DNS answer is unsafe")
	}
}

func TestS3TransportDoesNotFollowRedirectOrUseEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		require.Equal(t, "s3.example.com", r.Host)
		w.Header().Set("Location", "http://127.0.0.1/private")
		w.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	client := newS3HTTPClient()
	transport, ok := client.Transport.(*http.Transport)
	require.True(t, ok)
	defer transport.CloseIdleConnections()
	require.Nil(t, transport.Proxy)
	resolver := publicDestinationResolverFunc(func(context.Context, string, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
	})
	transport.DialContext = s3DialContext(resolver, func(ctx context.Context, network, address string) (net.Conn, error) {
		require.Equal(t, "8.8.8.8:80", address)
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	})
	response, err := client.Get("http://s3.example.com/test-bucket")
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	require.Equal(t, http.StatusTemporaryRedirect, response.StatusCode)
	require.Equal(t, 1, requests)
}
